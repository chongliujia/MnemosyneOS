package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"mnemosyneos/internal/airuntime"
	"mnemosyneos/internal/approval"
	"mnemosyneos/internal/execution"
	"mnemosyneos/internal/memory"
	"mnemosyneos/internal/model"
	"mnemosyneos/internal/skills"
)

type initOptions struct {
	RuntimeRoot            string
	APIBase                string
	EnvFile                string
	Provider               string
	Preset                 string
	BaseURL                string
	APIKey                 string
	ConversationModel      string
	RoutingModel           string
	SkillsModel            string
	WorkspaceRoot          string // if set, written as MNEMOSYNE_WORKSPACE_ROOT in EnvFile
	FilesystemUnrestricted string // if non-empty, written as MNEMOSYNE_FILESYSTEM_UNRESTRICTED ("true"/"false")
}

type initResult struct {
	RuntimeRoot      string       `json:"runtime_root"`
	APIBase          string       `json:"api_base"`
	EnvFile          string       `json:"env_file"`
	RuntimeStatePath string       `json:"runtime_state_path"`
	ModelConfigPath  string       `json:"model_config_path"`
	SkillsDir        string       `json:"skills_dir"`
	SkillStatePath   string       `json:"skill_state_path"`
	ModelConfig      model.Config `json:"model_config"`
	CreatedEnvFile   bool         `json:"created_env_file"`
	UpdatedEnvKeys   []string     `json:"updated_env_keys,omitempty"`
}

func redactedInitResult(result initResult) initResult {
	if strings.TrimSpace(result.ModelConfig.APIKey) != "" {
		result.ModelConfig.APIKey = "<redacted>"
	}
	return result
}

type doctorOptions struct {
	RuntimeRoot string
	APIBase     string
	EnvFile     string
	TestModel   bool
}

type doctorCheck struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Details string `json:"details,omitempty"`
}

type doctorReport struct {
	RuntimeRoot string        `json:"runtime_root"`
	APIBase     string        `json:"api_base"`
	EnvFile     string        `json:"env_file"`
	Status      string        `json:"status"`
	Checks      []doctorCheck `json:"checks"`
}

type doctorClient interface {
	Health() (map[string]string, error)
	RuntimeState() (airuntime.RuntimeState, error)
}

var newDoctorClient = func(baseURL string) doctorClient {
	return consoleClient{baseURL: resolveAPIBase(baseURL), httpClient: &http.Client{Timeout: 5 * time.Second}}
}

func runInit(opts initOptions) (initResult, error) {
	envFile := firstNonEmpty(strings.TrimSpace(opts.EnvFile), ".env.local")
	runtimeRoot := resolveRuntimeRoot(strings.TrimSpace(opts.RuntimeRoot))
	apiBase := resolveAPIBase(strings.TrimSpace(opts.APIBase))

	if err := ensureRuntimeLayout(runtimeRoot); err != nil {
		return initResult{}, err
	}
	runtimeStatePath, err := ensureRuntimeState(runtimeRoot)
	if err != nil {
		return initResult{}, err
	}
	skillStatePath, err := ensureSkillState(runtimeRoot)
	if err != nil {
		return initResult{}, err
	}
	privateModelConfig := initUsesPrivateModelConfig(opts)
	modelConfigPath := resolveInitModelConfigPath(runtimeRoot, opts)
	modelStore, err := model.NewConfigStoreAtPath(runtimeRoot, modelConfigPath)
	if err != nil {
		return initResult{}, err
	}
	cfg := mergeInitModelConfig(modelStore.Get(), opts)
	if err := modelStore.Save(cfg); err != nil {
		return initResult{}, err
	}

	envUpdates := map[string]string{
		"MNEMOSYNE_RUNTIME_ROOT": runtimeRoot,
		"MNEMOSYNE_API_BASE":     apiBase,
	}
	if w := strings.TrimSpace(opts.WorkspaceRoot); w != "" {
		aw, err := filepath.Abs(w)
		if err != nil {
			return initResult{}, fmt.Errorf("workspace root: %w", err)
		}
		envUpdates["MNEMOSYNE_WORKSPACE_ROOT"] = aw
	}
	if v := strings.TrimSpace(opts.FilesystemUnrestricted); v != "" {
		normalized, err := normalizeTruthyEnv(v)
		if err != nil {
			return initResult{}, err
		}
		envUpdates["MNEMOSYNE_FILESYSTEM_UNRESTRICTED"] = normalized
	}
	if privateModelConfig {
		envUpdates["MNEMOSYNE_MODEL_CONFIG_PATH"] = modelConfigPath
	}
	created, updated, err := upsertEnvFile(envFile, envUpdates)
	if err != nil {
		return initResult{}, err
	}

	return initResult{
		RuntimeRoot:      runtimeRoot,
		APIBase:          apiBase,
		EnvFile:          envFile,
		RuntimeStatePath: runtimeStatePath,
		ModelConfigPath:  firstNonEmpty(modelConfigPath, filepath.Join(runtimeRoot, "model", "config.json")),
		SkillsDir:        filepath.Join(runtimeRoot, "skills"),
		SkillStatePath:   skillStatePath,
		ModelConfig:      cfg,
		CreatedEnvFile:   created,
		UpdatedEnvKeys:   updated,
	}, nil
}

func runDoctor(opts doctorOptions) doctorReport {
	envFile := firstNonEmpty(strings.TrimSpace(opts.EnvFile), ".env.local")
	runtimeRoot := resolveRuntimeRoot(strings.TrimSpace(opts.RuntimeRoot))
	apiBase := resolveAPIBase(strings.TrimSpace(opts.APIBase))

	report := doctorReport{
		RuntimeRoot: runtimeRoot,
		APIBase:     apiBase,
		EnvFile:     envFile,
		Checks:      make([]doctorCheck, 0, 8),
		Status:      "ok",
	}
	add := func(name, status, details string) {
		report.Checks = append(report.Checks, doctorCheck{Name: name, Status: status, Details: details})
		switch status {
		case "fail":
			report.Status = "fail"
		case "warn":
			if report.Status == "ok" {
				report.Status = "warn"
			}
		}
	}

	if _, err := os.Stat(envFile); err == nil {
		add("env_file", "ok", envFile)
	} else if os.IsNotExist(err) {
		add("env_file", "warn", "missing .env file; run mnemosynectl init")
	} else {
		add("env_file", "fail", err.Error())
	}

	if info, err := os.Stat(runtimeRoot); err == nil && info.IsDir() {
		add("runtime_root", "ok", runtimeRoot)
	} else if os.IsNotExist(err) {
		add("runtime_root", "fail", "runtime root does not exist")
	} else if err != nil {
		add("runtime_root", "fail", err.Error())
	} else {
		add("runtime_root", "fail", "runtime root is not a directory")
	}

	runtimeStore := airuntime.NewStore(runtimeRoot)
	if state, err := runtimeStore.LoadState(); err == nil {
		add("runtime_state", "ok", fmt.Sprintf("status=%s execution_profile=%s", state.Status, state.ExecutionProfile))
	} else {
		add("runtime_state", "fail", err.Error())
	}

	modelStore, err := model.NewConfigStore(runtimeRoot)
	if err != nil {
		add("model_config", "fail", err.Error())
	} else {
		cfg := modelStore.Get()
		if details, status := validateDoctorModelConfig(cfg); status == "ok" {
			add("model_config", "ok", details)
		} else {
			add("model_config", status, details)
		}
		if opts.TestModel {
			switch status, details := testDoctorModelConfig(modelStore); status {
			case "ok":
				add("model_connection", "ok", details)
			case "warn":
				add("model_connection", "warn", details)
			default:
				add("model_connection", "fail", details)
			}
			switch status, details := testDoctorToolCalling(modelStore); status {
			case "ok":
				add("model_tool_calling", "ok", details)
			case "warn":
				add("model_tool_calling", "warn", details)
			default:
				add("model_tool_calling", "fail", details)
			}
		}
	}

	skillsDir := filepath.Join(runtimeRoot, "skills")
	if info, err := os.Stat(skillsDir); err == nil && info.IsDir() {
		memoryStore := memory.NewStore()
		runner := skills.NewRunner(runtimeStore, memoryStore, nil, nil, nil, nil)
		statuses := runner.ListManifestStatuses()
		failures := 0
		for _, status := range statuses {
			if !status.Loaded && strings.TrimSpace(status.Error) != "" {
				failures++
			}
		}
		if failures > 0 {
			add("skills", "warn", fmt.Sprintf("loaded=%d manifest_errors=%d", len(runner.ListSkills()), failures))
		} else {
			add("skills", "ok", fmt.Sprintf("loaded=%d manifests=%d", len(runner.ListSkills()), len(statuses)))
		}
	} else if os.IsNotExist(err) {
		add("skills", "warn", "skills directory missing")
	} else if err != nil {
		add("skills", "fail", err.Error())
	} else {
		add("skills", "fail", "skills path is not a directory")
	}

	client := newDoctorClient(apiBase)
	if health, err := client.Health(); err == nil {
		add("api_health", "ok", firstNonEmpty(health["status"], "ok"))
		if state, err := client.RuntimeState(); err == nil {
			add("api_runtime_state", "ok", fmt.Sprintf("status=%s", state.Status))
		} else {
			add("api_runtime_state", "warn", err.Error())
		}
	} else {
		add("api_health", "warn", err.Error())
	}

	return report
}

func ensureRuntimeLayout(runtimeRoot string) error {
	dirs := []string{
		filepath.Join(runtimeRoot, "state"),
		filepath.Join(runtimeRoot, "model"),
		filepath.Join(runtimeRoot, "skills"),
		filepath.Join(runtimeRoot, "tasks", airuntime.TaskStateInbox),
		filepath.Join(runtimeRoot, "tasks", airuntime.TaskStatePlanned),
		filepath.Join(runtimeRoot, "tasks", airuntime.TaskStateActive),
		filepath.Join(runtimeRoot, "tasks", airuntime.TaskStateBlocked),
		filepath.Join(runtimeRoot, "tasks", airuntime.TaskStateAwaitingApproval),
		filepath.Join(runtimeRoot, "tasks", airuntime.TaskStateDone),
		filepath.Join(runtimeRoot, "tasks", airuntime.TaskStateFailed),
		filepath.Join(runtimeRoot, "tasks", airuntime.TaskStateArchived),
		filepath.Join(runtimeRoot, "approvals", approval.StatusPending),
		filepath.Join(runtimeRoot, "approvals", approval.StatusApproved),
		filepath.Join(runtimeRoot, "approvals", approval.StatusDenied),
		filepath.Join(runtimeRoot, "approvals", approval.StatusConsumed),
		filepath.Join(runtimeRoot, "actions", execution.ActionStatusPending),
		filepath.Join(runtimeRoot, "actions", execution.ActionStatusRunning),
		filepath.Join(runtimeRoot, "actions", execution.ActionStatusCompleted),
		filepath.Join(runtimeRoot, "actions", execution.ActionStatusFailed),
		filepath.Join(runtimeRoot, "artifacts", "reports"),
		filepath.Join(runtimeRoot, "observations", "filesystem"),
		filepath.Join(runtimeRoot, "observations", "os"),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return nil
}

func ensureRuntimeState(runtimeRoot string) (string, error) {
	path := filepath.Join(runtimeRoot, "state", "runtime.json")
	if _, err := os.Stat(path); err == nil {
		return path, nil
	} else if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	state := airuntime.RuntimeState{
		RuntimeID:        fmt.Sprintf("runtime-%d", time.Now().UTC().UnixNano()),
		ActiveUserID:     "default-user",
		Status:           "idle",
		ExecutionProfile: "user",
		UpdatedAt:        time.Now().UTC(),
	}
	raw, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return "", err
	}
	raw = append(raw, '\n')
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func ensureSkillState(runtimeRoot string) (string, error) {
	path := filepath.Join(runtimeRoot, "state", "skills.json")
	if _, err := os.Stat(path); err == nil {
		return path, nil
	} else if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	raw := []byte("{\n  \"enabled\": {}\n}\n")
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func initUsesPrivateModelConfig(opts initOptions) bool {
	return strings.TrimSpace(opts.Provider) != "" ||
		strings.TrimSpace(opts.Preset) != "" ||
		strings.TrimSpace(opts.BaseURL) != "" ||
		strings.TrimSpace(opts.APIKey) != "" ||
		strings.TrimSpace(opts.ConversationModel) != "" ||
		strings.TrimSpace(opts.RoutingModel) != "" ||
		strings.TrimSpace(opts.SkillsModel) != ""
}

func resolveInitModelConfigPath(runtimeRoot string, opts initOptions) string {
	if !initUsesPrivateModelConfig(opts) {
		return ""
	}
	if value := strings.TrimSpace(os.Getenv("MNEMOSYNE_MODEL_CONFIG_PATH")); value != "" {
		if filepath.IsAbs(value) {
			return value
		}
		if abs, err := filepath.Abs(value); err == nil {
			return abs
		}
		return value
	}
	absRuntime, err := filepath.Abs(runtimeRoot)
	if err != nil {
		absRuntime = runtimeRoot
	}
	return filepath.Join(absRuntime, "model", "local.config.json")
}

func mergeInitModelConfig(current model.Config, opts initOptions) model.Config {
	cfg := current
	if strings.TrimSpace(opts.Provider) != "" {
		cfg.Provider = strings.TrimSpace(opts.Provider)
	}
	if strings.TrimSpace(opts.Preset) != "" {
		cfg.Preset = strings.TrimSpace(opts.Preset)
	}
	if strings.TrimSpace(opts.BaseURL) != "" {
		cfg.BaseURL = strings.TrimSpace(opts.BaseURL)
	}
	if strings.TrimSpace(opts.APIKey) != "" {
		cfg.APIKey = strings.TrimSpace(opts.APIKey)
	}
	if strings.TrimSpace(opts.ConversationModel) != "" {
		cfg.Conversation.Model = strings.TrimSpace(opts.ConversationModel)
	}
	if strings.TrimSpace(opts.RoutingModel) != "" {
		cfg.Routing.Model = strings.TrimSpace(opts.RoutingModel)
	}
	if strings.TrimSpace(opts.SkillsModel) != "" {
		cfg.Skills.Model = strings.TrimSpace(opts.SkillsModel)
	}
	if cfg.Provider == "none" {
		if strings.TrimSpace(opts.Provider) == "" && (strings.TrimSpace(opts.BaseURL) != "" || strings.TrimSpace(opts.APIKey) != "" || strings.TrimSpace(opts.ConversationModel) != "") {
			cfg.Provider = "openai-compatible"
		}
	}
	return cfg
}

func validateDoctorModelConfig(cfg model.Config) (string, string) {
	if strings.TrimSpace(cfg.Provider) == "" || strings.TrimSpace(cfg.Provider) == "none" {
		return "model provider is not configured", "warn"
	}
	missing := make([]string, 0, 4)
	if strings.TrimSpace(cfg.APIKey) == "" {
		missing = append(missing, "api_key")
	}
	if strings.TrimSpace(cfg.BaseURL) == "" {
		missing = append(missing, "base_url")
	}
	if strings.TrimSpace(cfg.Conversation.Model) == "" {
		missing = append(missing, "conversation_model")
	}
	if strings.TrimSpace(cfg.Routing.Model) == "" {
		missing = append(missing, "routing_model")
	}
	if strings.TrimSpace(cfg.Skills.Model) == "" {
		missing = append(missing, "skills_model")
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return "missing " + strings.Join(missing, ", "), "fail"
	}
	return fmt.Sprintf("provider=%s base_url=%s", cfg.Provider, cfg.BaseURL), "ok"
}

func testDoctorModelConfig(store *model.ConfigStore) (string, string) {
	if store == nil {
		return "model config store is not available", "fail"
	}
	gateway, ok := model.NewRuntimeGateway(store).(*model.RuntimeGateway)
	if !ok {
		return "runtime gateway is not available", "fail"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	resp, err := gateway.TestConfig(ctx, store.Get())
	if err != nil {
		return err.Error(), "fail"
	}
	return fmt.Sprintf("provider=%s model=%s", resp.Provider, resp.Model), "ok"
}

func testDoctorToolCalling(store *model.ConfigStore) (string, string) {
	if store == nil {
		return "model config store is not available", "fail"
	}
	gateway, ok := model.NewRuntimeGateway(store).(*model.RuntimeGateway)
	if !ok {
		return "runtime gateway is not available", "fail"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	resp, err := gateway.GenerateText(ctx, model.TextRequest{
		SystemPrompt: "You are testing tool-calling support. Call the provided function exactly once. Do not answer in plain text.",
		UserPrompt:   "Call the get_workspace_info function now.",
		MaxTokens:    80,
		Temperature:  0,
		Profile:      model.ProfileConversation,
		Tools: []model.ToolDefinition{{
			Type: "function",
			Function: model.ToolFunction{
				Name:        "get_workspace_info",
				Description: "Return local workspace information for a capability probe.",
				Parameters: map[string]any{
					"type":       "object",
					"properties": map[string]any{},
				},
			},
		}},
	})
	if err != nil {
		return err.Error(), "fail"
	}
	if len(resp.ToolCalls) == 0 {
		return "model responded without tool_calls; choose a model/provider that supports OpenAI-compatible function calling for file/code operations", "fail"
	}
	return fmt.Sprintf("provider=%s model=%s tool=%s", resp.Provider, resp.Model, resp.ToolCalls[0].Function.Name), "ok"
}

func resolveRuntimeRoot(explicit string) string {
	if strings.TrimSpace(explicit) != "" {
		return strings.TrimSpace(explicit)
	}
	if value := strings.TrimSpace(os.Getenv("MNEMOSYNE_RUNTIME_ROOT")); value != "" {
		return value
	}
	return "runtime"
}

func resolveAPIBase(explicit string) string {
	if strings.TrimSpace(explicit) != "" {
		return strings.TrimRight(strings.TrimSpace(explicit), "/")
	}
	if value := strings.TrimSpace(os.Getenv("MNEMOSYNE_API_BASE")); value != "" {
		return strings.TrimRight(value, "/")
	}
	return "http://127.0.0.1:8080"
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func normalizeTruthyEnv(s string) (string, error) {
	v := strings.ToLower(strings.TrimSpace(s))
	switch v {
	case "1", "true", "yes", "on", "y":
		return "true", nil
	case "0", "false", "no", "off", "n":
		return "false", nil
	default:
		return "", fmt.Errorf("invalid boolean-like value %q (use true/false)", s)
	}
}

func upsertEnvFile(path string, updates map[string]string) (bool, []string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return false, nil, fmt.Errorf("env file path is required")
	}
	created := false
	lines := make([]string, 0)
	if raw, err := os.ReadFile(path); err == nil {
		lines = strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n")
	} else if os.IsNotExist(err) {
		created = true
	} else {
		return false, nil, err
	}
	keys := make([]string, 0, len(updates))
	for key := range updates {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	updated := make([]string, 0, len(keys))
	seen := make(map[string]bool, len(keys))
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		key, _, ok := strings.Cut(trimmed, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value, exists := updates[key]
		if !exists {
			continue
		}
		lines[i] = fmt.Sprintf("%s=%s", key, value)
		seen[key] = true
		updated = append(updated, key)
	}
	for _, key := range keys {
		if seen[key] {
			continue
		}
		lines = append(lines, fmt.Sprintf("%s=%s", key, updates[key]))
		updated = append(updated, key)
	}
	content := strings.Join(lines, "\n")
	content = strings.TrimRight(content, "\n") + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return created, updated, err
	}
	return created, updated, nil
}

type consoleClient struct {
	baseURL    string
	httpClient *http.Client
}

func (c consoleClient) Health() (map[string]string, error) {
	var out map[string]string
	err := c.do(http.MethodGet, "/health", &out)
	return out, err
}

func (c consoleClient) RuntimeState() (airuntime.RuntimeState, error) {
	var out airuntime.RuntimeState
	err := c.do(http.MethodGet, "/runtime/state", &out)
	return out, err
}

func (c consoleClient) do(method, path string, target any) error {
	req, err := http.NewRequest(method, c.baseURL+path, nil)
	if err != nil {
		return err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("request failed with status %d", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(target)
}
