package runtimeapp

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"mnemosyneos/internal/airuntime"
	"mnemosyneos/internal/api"
	"mnemosyneos/internal/approval"
	"mnemosyneos/internal/chat"
	"mnemosyneos/internal/connectors"
	"mnemosyneos/internal/emailconnector"
	"mnemosyneos/internal/execution"
	"mnemosyneos/internal/githubconnector"
	"mnemosyneos/internal/memory"
	"mnemosyneos/internal/model"
	"mnemosyneos/internal/recall"
	"mnemosyneos/internal/skills"
	"mnemosyneos/internal/websearch"
)

type Options struct {
	Addr          string
	RuntimeRoot   string
	WorkspaceRoot string
	EnableWeb     bool
}

type App struct {
	Addr         string
	RuntimeRoot  string
	Workspace    string
	Handler      http.Handler
	RuntimeStore *airuntime.Store
	MemoryStore  *memory.Store
	SkillRunner  *skills.Runner
	ModelConfig  *model.ConfigStore
	govScheduler *memory.GovernanceScheduler
}

// Shutdown releases persistent resources (memory store, scheduler).
func (a *App) Shutdown() {
	if a.govScheduler != nil {
		a.govScheduler.Stop()
	}
	if a.MemoryStore != nil {
		_ = a.MemoryStore.Close()
	}
}

func Build(opts Options) (*App, error) {
	addr := strings.TrimSpace(opts.Addr)
	if addr == "" {
		addr = strings.TrimSpace(os.Getenv("MNEMOSYNE_ADDR"))
	}
	if addr == "" {
		addr = ":8080"
	}

	runtimeRoot := strings.TrimSpace(opts.RuntimeRoot)
	if runtimeRoot == "" {
		runtimeRoot = strings.TrimSpace(os.Getenv("MNEMOSYNE_RUNTIME_ROOT"))
	}
	if runtimeRoot == "" {
		runtimeRoot = "runtime"
	}

	workspaceRoot := strings.TrimSpace(opts.WorkspaceRoot)
	if workspaceRoot == "" {
		workspaceRoot = strings.TrimSpace(os.Getenv("MNEMOSYNE_WORKSPACE_ROOT"))
	}
	if workspaceRoot == "" {
		workspaceRoot = "."
	}
	if absWS, err := filepath.Abs(workspaceRoot); err == nil {
		workspaceRoot = absWS
	}

	memoryRoot := filepath.Join(runtimeRoot, "memory")
	store, err := memory.NewPersistentStore(memoryRoot)
	if err != nil {
		return nil, fmt.Errorf("create persistent memory store: %w", err)
	}
	runtimeStore := airuntime.NewStore(runtimeRoot)
	if state, err := runtimeStore.LoadState(); err != nil {
		// Auto-bootstrap: create a fresh runtime state if none exists.
		state = airuntime.RuntimeState{
			RuntimeID:        fmt.Sprintf("rt-%d", time.Now().UnixNano()),
			Status:           "idle",
			ExecutionProfile: "user",
		}
		if saveErr := runtimeStore.SaveState(state); saveErr != nil {
			return nil, fmt.Errorf("bootstrap runtime state: %w", saveErr)
		}
	} else if err := runtimeStore.SaveState(state); err != nil {
		return nil, fmt.Errorf("save runtime state: %w", err)
	}

	orchestrator := airuntime.NewOrchestrator(runtimeStore)
	if err := orchestrator.Recover(); err != nil {
		return nil, fmt.Errorf("recover runtime state: %w", err)
	}

	approvalTTL := 10 * time.Minute
	if value := strings.TrimSpace(os.Getenv("MNEMOSYNE_ROOT_APPROVAL_TTL")); value != "" {
		parsed, err := time.ParseDuration(value)
		if err != nil {
			return nil, fmt.Errorf("parse MNEMOSYNE_ROOT_APPROVAL_TTL: %w", err)
		}
		approvalTTL = parsed
	}
	approvalStore := approval.NewStore(runtimeRoot, approvalTTL)

	actionStore := execution.NewStore(runtimeRoot)
	executor, err := execution.NewExecutorWithApprovals(actionStore, workspaceRoot, os.Getenv("MNEMOSYNE_ROOT_APPROVAL_TOKEN"), approvalStore)
	if err != nil {
		return nil, fmt.Errorf("create executor: %w", err)
	}

	searchClient, err := websearch.NewClientFromEnv()
	if err != nil {
		return nil, fmt.Errorf("create web search client: %w", err)
	}
	githubClient, err := githubconnector.NewClientFromEnv()
	if err != nil {
		return nil, fmt.Errorf("create github connector: %w", err)
	}
	emailClient, err := emailconnector.NewClientFromEnv()
	if err != nil {
		return nil, fmt.Errorf("create email connector: %w", err)
	}
	connectorRuntime := connectors.NewRuntime(
		connectors.NewWebSearchAdapter(searchClient),
		githubClient,
		emailClient,
	)

	modelConfig, err := model.NewConfigStore(runtimeRoot)
	if err != nil {
		return nil, fmt.Errorf("create model config store: %w", err)
	}
	modelProfiles, err := model.NewProfileStore(runtimeRoot)
	if err != nil {
		return nil, fmt.Errorf("create model profile store: %w", err)
	}
	textModel := model.NewRuntimeGateway(modelConfig)
	skillRunner := skills.NewRunner(runtimeStore, store, executor, connectorRuntime, approvalStore, textModel)
	recallService := recall.NewService(store)
	chatService := chat.NewServiceWithSystemInfo(
		chat.NewStore(runtimeRoot), orchestrator, runtimeStore, recallService,
		skillRunner, textModel, store,
		chat.SystemInfo{
			RuntimeRoot:   runtimeRoot,
			WorkspaceRoot: workspaceRoot,
			Addr:          addr,
		},
	)

	// Set up the agent skills registry and inject the agent loop into the
	// chat service so the LLM can autonomously call tools.
	agentSkillsReg := skills.NewAgentSkillRegistry()
	skills.RegisterBuiltinAgentSkills(agentSkillsReg, skills.BuiltinSkillOpts{
		WorkspaceRoot: workspaceRoot,
		RuntimeRoot:   runtimeRoot,
		Version:       "0.1.0",
		Addr:          addr,
		Connectors:    connectorRuntime,
		RecallService: recallService,
		RuntimeStore:  runtimeStore,
	})
	chatService.SetAgentLoop(chat.NewAgentLoop(textModel, agentSkillsReg))

	handler := api.NewServerWithOptions(store, runtimeStore, approvalStore, chatService, recallService, orchestrator, executor, skillRunner, modelConfig, api.ServerOptions{
		EnableWeb:     opts.EnableWeb,
		ModelProfiles: modelProfiles,
	}).Routes()

	govScheduler := memory.NewGovernanceScheduler(store, nil)
	govScheduler.Start()

	return &App{
		Addr:         addr,
		RuntimeRoot:  runtimeRoot,
		Workspace:    workspaceRoot,
		Handler:      handler,
		RuntimeStore: runtimeStore,
		MemoryStore:  store,
		SkillRunner:  skillRunner,
		ModelConfig:  modelConfig,
		govScheduler: govScheduler,
	}, nil
}
