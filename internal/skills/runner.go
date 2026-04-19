package skills

import (
	"context"
	"crypto/sha1"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"mnemosyneos/internal/airuntime"
	"mnemosyneos/internal/approval"
	"mnemosyneos/internal/connectors"
	"mnemosyneos/internal/execution"
	"mnemosyneos/internal/memory"
	"mnemosyneos/internal/memoryscheduler"
	"mnemosyneos/internal/model"
)

const (
	memoryScopeProject = "project"
	memoryScopeUser    = "user"
)

type RunResult struct {
	Task             airuntime.Task          `json:"task"`
	Action           *execution.ActionRecord `json:"action,omitempty"`
	ArtifactPaths    []string                `json:"artifact_paths,omitempty"`
	ObservationPaths []string                `json:"observation_paths,omitempty"`
}

type ProgressEvent struct {
	Stage   string `json:"stage"`
	Message string `json:"message"`
}

type Runner struct {
	runtimeStore *airuntime.Store
	memoryStore  *memory.Store
	consolidator *memory.Consolidator
	registry     *Registry
	manifests    []ManifestStatus
	schedulerCfg memoryscheduler.Config
	executor     *execution.Executor
	connectors   *connectors.Runtime
	approvals    *approval.Store
	model        model.TextGateway
}

func NewRunner(runtimeStore *airuntime.Store, memoryStore *memory.Store, executor *execution.Executor, connectorRuntime *connectors.Runtime, approvalStore *approval.Store, textModel model.TextGateway) *Runner {
	runner := &Runner{
		runtimeStore: runtimeStore,
		memoryStore:  memoryStore,
		consolidator: memory.NewConsolidator(memoryStore),
		registry:     NewRegistry(),
		schedulerCfg: memoryscheduler.Config{
			Scope:             memory.ScopeProject,
			Cooldown:          5 * time.Minute,
			MinCandidates:     1,
			AllowedCardTypes:  []string{"web_result", "email_thread", "email_message", "github_issue", "procedure"},
			ExtractProcedures: true,
			ProcedureMinRuns:  2,
		},
		executor:   executor,
		connectors: connectorRuntime,
		approvals:  approvalStore,
		model:      textModel,
	}
	_ = runner.ReloadSkills()
	return runner
}

func (r *Runner) RegisterSkill(def Definition) error {
	if r == nil {
		return fmt.Errorf("runner is nil")
	}
	if r.registry == nil {
		r.registry = NewRegistry()
	}
	return r.registry.Register(def)
}

func (r *Runner) ListSkills() []Definition {
	if r == nil || r.registry == nil {
		return nil
	}
	return r.registry.List()
}

func (r *Runner) ListManifestStatuses() []ManifestStatus {
	if r == nil || len(r.manifests) == 0 {
		return nil
	}
	out := make([]ManifestStatus, len(r.manifests))
	copy(out, r.manifests)
	return out
}

func (r *Runner) registerBuiltins() {
	must := func(def Definition) {
		if err := r.RegisterSkill(def); err != nil {
			panic(err)
		}
	}
	must(Definition{
		Name:        "task-plan",
		Description: "Generate a task plan and supporting procedure evidence.",
		Source:      "builtin",
		Enabled:     true,
		Handler:     (*Runner).runTaskPlan,
		MaintenancePolicy: &MaintenancePolicy{
			Enabled:          true,
			Scope:            memory.ScopeProject,
			AllowedCardTypes: []string{"procedure"},
			MinCandidates:    1,
		},
	})
	must(Definition{Name: "file-edit", Description: "Write files through the execution layer.", Source: "builtin", Enabled: true, Handler: (*Runner).runFileEdit})
	must(Definition{Name: "file-read", Description: "Read files through the execution layer.", Source: "builtin", Enabled: true, Handler: (*Runner).runFileRead})
	must(Definition{Name: "shell-command", Description: "Run shell commands through the execution layer.", Source: "builtin", Enabled: true, Handler: (*Runner).runShellCommand})
	must(Definition{
		Name:        "web-search",
		Description: "Search the web and persist candidate memory.",
		Source:      "builtin",
		Enabled:     true,
		Handler:     (*Runner).runWebSearch,
		MaintenancePolicy: &MaintenancePolicy{
			Enabled:          true,
			Scope:            memory.ScopeProject,
			AllowedCardTypes: []string{"web_result", "procedure"},
			MinCandidates:    1,
		},
	})
	must(Definition{
		Name:        "github-issue-search",
		Description: "Search GitHub issues and persist candidate memory.",
		Source:      "builtin",
		Enabled:     true,
		Handler:     (*Runner).runGitHubIssueSearch,
		MaintenancePolicy: &MaintenancePolicy{
			Enabled:          true,
			Scope:            memory.ScopeProject,
			AllowedCardTypes: []string{"github_issue", "procedure"},
			MinCandidates:    1,
		},
	})
	must(Definition{
		Name:        "email-inbox",
		Description: "Read inbox messages and persist candidate memory.",
		Source:      "builtin",
		Enabled:     true,
		Handler:     (*Runner).runEmailInbox,
		MaintenancePolicy: &MaintenancePolicy{
			Enabled:          true,
			Scope:            memory.ScopeUser,
			AllowedCardTypes: []string{"email_thread", "email_message", "procedure"},
			MinCandidates:    1,
		},
	})
	must(Definition{Name: "memory-consolidate", Description: "Promote candidate memory into active durable memory.", Source: "builtin", Enabled: true, Handler: (*Runner).runMemoryConsolidate})
}

func (r *Runner) ReloadSkills() error {
	if r == nil {
		return fmt.Errorf("runner is nil")
	}
	r.registry = NewRegistry()
	r.registerBuiltins()
	if r.runtimeStore == nil {
		return r.applySkillStateOverrides()
	}
	if err := r.LoadSkillManifests(filepath.Join(r.runtimeStore.RootDir(), "skills")); err != nil {
		return err
	}
	return r.applySkillStateOverrides()
}

func (r *Runner) SetSkillEnabled(name string, enabled bool) error {
	if r == nil {
		return fmt.Errorf("runner is nil")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("skill name is required")
	}
	if _, ok := r.registry.Resolve(name); !ok {
		return fmt.Errorf("skill %s not registered", name)
	}
	state, err := r.loadSkillState()
	if err != nil {
		return err
	}
	if state.Enabled == nil {
		state.Enabled = map[string]bool{}
	}
	state.Enabled[name] = enabled
	if err := r.saveSkillState(state); err != nil {
		return err
	}
	return r.ReloadSkills()
}

func (r *Runner) RunTask(taskID string) (RunResult, error) {
	return r.RunTaskWithProgress(taskID, nil)
}

func (r *Runner) RunTaskWithProgress(taskID string, onProgress func(ProgressEvent)) (RunResult, error) {
	task, err := r.runtimeStore.GetTask(taskID)
	if err != nil {
		return RunResult{}, err
	}

	switch task.State {
	case airuntime.TaskStatePlanned, airuntime.TaskStateActive:
	case airuntime.TaskStateAwaitingApproval:
		return RunResult{}, fmt.Errorf("task %s is awaiting approval", taskID)
	default:
		return RunResult{}, fmt.Errorf("task %s is not runnable from state %s", taskID, task.State)
	}

	skillName := strings.TrimSpace(task.SelectedSkill)
	if skillName == "" {
		skillName = "task-plan"
	}
	def, ok := r.registry.Resolve(skillName)
	if !ok {
		def, _ = r.registry.Resolve("task-plan")
	}
	if !def.Enabled {
		return RunResult{}, fmt.Errorf("skill %s is disabled", skillName)
	}
	task = r.applySkillDefaults(task, def)
	return def.Handler(r, task, onProgress)
}

func (r *Runner) applySkillDefaults(task airuntime.Task, def Definition) airuntime.Task {
	if task.Metadata == nil {
		task.Metadata = map[string]string{}
	}
	for key, value := range def.DefaultMetadata {
		if strings.TrimSpace(task.Metadata[key]) == "" {
			task.Metadata[key] = value
		}
	}
	if strings.TrimSpace(task.ExecutionProfile) == "" && strings.TrimSpace(def.ExecutionProfile) != "" {
		task.ExecutionProfile = strings.TrimSpace(def.ExecutionProfile)
	}
	return task
}

func (r *Runner) runTaskPlan(task airuntime.Task, onProgress func(ProgressEvent)) (RunResult, error) {
	emitProgress(onProgress, "planning.generate", "Generating the task plan...")
	now := time.Now().UTC()
	body, modelResp, modelErr := r.generateTaskPlan(task, now)
	body = attachProcedureHintsToPlan(body, task.Metadata)
	emitProgress(onProgress, "planning.persist", "Writing the plan artifact and observation...")
	artifactPath, err := r.writeArtifact("reports", task.TaskID+"-plan.md", body)
	if err != nil {
		return RunResult{}, err
	}
	payload := map[string]any{
		"type":           "task-plan",
		"task_id":        task.TaskID,
		"selected_skill": firstNonEmpty(task.SelectedSkill, "task-plan"),
		"generated_at":   now.Format(time.RFC3339),
		"summary":        "plan artifact generated",
		"model_provider": modelResp.Provider,
		"model_name":     modelResp.Model,
		"model_error":    errorString(modelErr),
		"artifact_path":  artifactPath,
	}
	if hints := procedureHintsFromMetadata(task.Metadata); hints.hasEvidence() {
		payload["procedure_task_class"] = firstNonEmpty(strings.TrimSpace(task.Metadata["procedure_task_class"]), strings.TrimSpace(task.Metadata["task_class"]))
		payload["procedure_steps"] = hints.Steps
		payload["procedure_guardrails"] = hints.Guardrails
		payload["procedure_summary"] = hints.Summary
		payload["procedure_success_signal"] = hints.SuccessSignal
	}
	obsPath, err := r.writeObservation("os", task.TaskID+"-plan.json", payload)
	if err != nil {
		return RunResult{}, err
	}

	updated, err := r.runtimeStore.MoveTask(task.TaskID, airuntime.TaskStateDone, func(t *airuntime.Task) {
		ensureMetadata(t)
		t.NextAction = "plan generated"
		t.Metadata["plan_artifact"] = artifactPath
		t.Metadata["plan_observation"] = obsPath
	})
	if err != nil {
		return RunResult{}, err
	}
	if err := r.finalizeSuccessfulTask(updated, onProgress); err != nil {
		return RunResult{}, err
	}
	return RunResult{Task: updated, ArtifactPaths: []string{artifactPath}, ObservationPaths: []string{obsPath}}, nil
}

func (r *Runner) runFileEdit(task airuntime.Task, onProgress func(ProgressEvent)) (RunResult, error) {
	path := task.Metadata["path"]
	content := task.Metadata["content"]
	if strings.TrimSpace(path) == "" {
		return r.blockTask(task, "missing metadata.path for file-edit")
	}
	approvalToken, approvalResult, err := r.resolveRootApproval(task, execution.ActionKindFileWrite, fmt.Sprintf("root file write to %s", path), map[string]string{
		"path":  path,
		"skill": "file-edit",
	})
	if approvalResult != nil || err != nil {
		if err != nil {
			return RunResult{}, err
		}
		return *approvalResult, nil
	}

	emitProgress(onProgress, "execution.dispatch", fmt.Sprintf("Writing file %s...", path))
	action, err := r.executor.ExecuteFileWrite(execution.FileWriteActionRequest{
		TaskID:           task.TaskID,
		Path:             path,
		Content:          content,
		CreateParents:    true,
		ExecutionProfile: task.ExecutionProfile,
		ApprovalToken:    approvalToken,
		Metadata: map[string]string{
			"skill": "file-edit",
		},
	})
	if err != nil {
		return RunResult{}, err
	}

	emitProgress(onProgress, "execution.persist", "Recording file edit results...")
	obsPath, obsErr := r.writeObservation("filesystem", task.TaskID+"-file-edit.json", map[string]any{
		"type":          "file-edit",
		"task_id":       task.TaskID,
		"action_id":     action.ActionID,
		"path":          path,
		"status":        action.Status,
		"changed_files": action.ChangedFiles,
	})
	if obsErr != nil {
		return RunResult{}, obsErr
	}

	if action.Status == execution.ActionStatusCompleted {
		updated, err := r.runtimeStore.MoveTask(task.TaskID, airuntime.TaskStateDone, func(t *airuntime.Task) {
			ensureMetadata(t)
			t.NextAction = "file edit completed"
			t.Metadata["last_action_id"] = action.ActionID
			delete(t.Metadata, "approval_token")
		})
		if err != nil {
			return RunResult{}, err
		}
		if err := r.finalizeSuccessfulTask(updated, onProgress); err != nil {
			return RunResult{}, err
		}
		return RunResult{Task: updated, Action: &action, ObservationPaths: []string{obsPath}}, nil
	}

	updated, err := r.runtimeStore.MoveTask(task.TaskID, airuntime.TaskStateFailed, func(t *airuntime.Task) {
		ensureMetadata(t)
		t.FailureReason = firstNonEmpty(action.Error, "file edit failed")
		t.NextAction = "investigate failed file edit"
		t.Metadata["last_action_id"] = action.ActionID
		delete(t.Metadata, "approval_token")
	})
	if err != nil {
		return RunResult{}, err
	}
	if err := r.clearActiveTask(updated.TaskID); err != nil {
		return RunResult{}, err
	}
	return RunResult{Task: updated, Action: &action, ObservationPaths: []string{obsPath}}, nil
}

func (r *Runner) runFileRead(task airuntime.Task, onProgress func(ProgressEvent)) (RunResult, error) {
	path := task.Metadata["path"]
	if strings.TrimSpace(path) == "" {
		return r.blockTask(task, "missing metadata.path for file-read")
	}
	approvalToken, approvalResult, err := r.resolveRootApproval(task, execution.ActionKindFileRead, fmt.Sprintf("root file read from %s", path), map[string]string{
		"path":  path,
		"skill": "file-read",
	})
	if approvalResult != nil || err != nil {
		if err != nil {
			return RunResult{}, err
		}
		return *approvalResult, nil
	}

	emitProgress(onProgress, "execution.dispatch", fmt.Sprintf("Reading file %s...", path))
	action, err := r.executor.ExecuteFileRead(execution.FileReadActionRequest{
		TaskID:           task.TaskID,
		Path:             path,
		ExecutionProfile: task.ExecutionProfile,
		ApprovalToken:    approvalToken,
		Metadata: map[string]string{
			"skill": "file-read",
		},
	})
	if err != nil {
		return RunResult{}, err
	}

	emitProgress(onProgress, "execution.persist", "Persisting the file read artifact...")
	artifactPath, err := r.writeArtifact("reports", task.TaskID+"-file-read.txt", action.Stdout)
	if err != nil {
		return RunResult{}, err
	}
	obsPath, err := r.writeObservation("filesystem", task.TaskID+"-file-read.json", map[string]any{
		"type":          "file-read",
		"task_id":       task.TaskID,
		"action_id":     action.ActionID,
		"path":          path,
		"status":        action.Status,
		"artifact_path": artifactPath,
	})
	if err != nil {
		return RunResult{}, err
	}

	if action.Status == execution.ActionStatusCompleted {
		updated, err := r.runtimeStore.MoveTask(task.TaskID, airuntime.TaskStateDone, func(t *airuntime.Task) {
			ensureMetadata(t)
			t.NextAction = "file read completed"
			t.Metadata["last_action_id"] = action.ActionID
			t.Metadata["file_read_artifact"] = artifactPath
			delete(t.Metadata, "approval_token")
		})
		if err != nil {
			return RunResult{}, err
		}
		if err := r.finalizeSuccessfulTask(updated, onProgress); err != nil {
			return RunResult{}, err
		}
		return RunResult{Task: updated, Action: &action, ArtifactPaths: []string{artifactPath}, ObservationPaths: []string{obsPath}}, nil
	}

	updated, err := r.runtimeStore.MoveTask(task.TaskID, airuntime.TaskStateFailed, func(t *airuntime.Task) {
		ensureMetadata(t)
		t.FailureReason = firstNonEmpty(action.Error, "file read failed")
		t.NextAction = "investigate failed file read"
		t.Metadata["last_action_id"] = action.ActionID
		delete(t.Metadata, "approval_token")
	})
	if err != nil {
		return RunResult{}, err
	}
	if err := r.clearActiveTask(updated.TaskID); err != nil {
		return RunResult{}, err
	}
	return RunResult{Task: updated, Action: &action, ArtifactPaths: []string{artifactPath}, ObservationPaths: []string{obsPath}}, nil
}

func (r *Runner) runShellCommand(task airuntime.Task, onProgress func(ProgressEvent)) (RunResult, error) {
	command := strings.TrimSpace(task.Metadata["command"])
	if command == "" {
		return r.blockTask(task, "missing metadata.command for shell-command")
	}
	args := splitArgs(task.Metadata["args"])
	workdir := task.Metadata["workdir"]
	timeoutMS := parseInt(task.Metadata["timeout_ms"], 0)
	maxAttempts := parseInt(task.Metadata["max_attempts"], 0)
	idempotencyKey := strings.TrimSpace(task.Metadata["idempotency_key"])
	approvalToken, approvalResult, err := r.resolveRootApproval(task, execution.ActionKindShell, fmt.Sprintf("root shell command %s", command), map[string]string{
		"command": command,
		"args":    task.Metadata["args"],
		"workdir": workdir,
		"skill":   "shell-command",
	})
	if approvalResult != nil || err != nil {
		if err != nil {
			return RunResult{}, err
		}
		return *approvalResult, nil
	}

	emitProgress(onProgress, "execution.dispatch", fmt.Sprintf("Running command %s...", command))
	action, err := r.executor.ExecuteShell(execution.ShellActionRequest{
		TaskID:           task.TaskID,
		Command:          command,
		Args:             args,
		Workdir:          workdir,
		TimeoutMS:        timeoutMS,
		MaxAttempts:      maxAttempts,
		IdempotencyKey:   idempotencyKey,
		ExecutionProfile: task.ExecutionProfile,
		ApprovalToken:    approvalToken,
		Metadata: map[string]string{
			"skill": "shell-command",
		},
	})
	if err != nil {
		return RunResult{}, err
	}

	emitProgress(onProgress, "execution.persist", "Persisting command output...")
	artifactPath, err := r.writeArtifact("reports", task.TaskID+"-shell.txt", strings.TrimSpace(action.Stdout+"\n"+action.Stderr))
	if err != nil {
		return RunResult{}, err
	}
	obsPath, err := r.writeObservation("os", task.TaskID+"-shell.json", map[string]any{
		"type":          "shell-command",
		"task_id":       task.TaskID,
		"action_id":     action.ActionID,
		"command":       command,
		"args":          args,
		"workdir":       workdir,
		"status":        action.Status,
		"attempts":      len(action.AttemptHistory),
		"retryable":     action.Retryable,
		"failure_type":  action.FailureCategory,
		"replayed":      action.Replayed,
		"replay_of":     action.ReplayOfActionID,
		"artifact_path": artifactPath,
	})
	if err != nil {
		return RunResult{}, err
	}

	if action.Status == execution.ActionStatusCompleted {
		updated, err := r.runtimeStore.MoveTask(task.TaskID, airuntime.TaskStateDone, func(t *airuntime.Task) {
			ensureMetadata(t)
			t.NextAction = "shell command completed"
			t.Metadata["last_action_id"] = action.ActionID
			t.Metadata["shell_artifact"] = artifactPath
			delete(t.Metadata, "approval_token")
		})
		if err != nil {
			return RunResult{}, err
		}
		if err := r.finalizeSuccessfulTask(updated, onProgress); err != nil {
			return RunResult{}, err
		}
		return RunResult{Task: updated, Action: &action, ArtifactPaths: []string{artifactPath}, ObservationPaths: []string{obsPath}}, nil
	}

	updated, err := r.runtimeStore.MoveTask(task.TaskID, airuntime.TaskStateFailed, func(t *airuntime.Task) {
		ensureMetadata(t)
		t.FailureReason = firstNonEmpty(action.Error, "shell command failed")
		t.NextAction = "investigate failed shell command"
		t.Metadata["last_action_id"] = action.ActionID
		delete(t.Metadata, "approval_token")
	})
	if err != nil {
		return RunResult{}, err
	}
	if err := r.finalizeSuccessfulTask(updated, onProgress); err != nil {
		return RunResult{}, err
	}
	return RunResult{Task: updated, Action: &action, ArtifactPaths: []string{artifactPath}, ObservationPaths: []string{obsPath}}, nil
}

func (r *Runner) runWebSearch(task airuntime.Task, onProgress func(ProgressEvent)) (RunResult, error) {
	if r.connectors == nil {
		return r.blockSearchTask(task, "web search API is not configured")
	}

	query := task.Goal
	if task.Metadata != nil {
		query = firstNonEmpty(task.Metadata["query"], query)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	if strings.TrimSpace(query) == "" {
		return r.blockSearchTask(task, "missing search query")
	}
	emitProgress(onProgress, "connector.request", "Calling the web search connector...")
	resp, err := r.connectors.Search(ctx, connectors.SearchRequest{
		Query: query,
		Limit: 5,
	})
	if err != nil {
		return r.blockSearchTask(task, err.Error())
	}

	emitProgress(onProgress, "persist.artifacts", "Writing search artifact and observation...")
	artifactPath, err := r.writeArtifact("reports", task.TaskID+"-web-search.md", renderSearchArtifact(task, resp))
	if err != nil {
		return RunResult{}, err
	}
	obsPath, err := r.writeObservation("os", task.TaskID+"-web-search.json", map[string]any{
		"type":          "web-search",
		"task_id":       task.TaskID,
		"status":        "completed",
		"goal":          task.Goal,
		"query":         resp.Query,
		"provider":      resp.Provider,
		"result_count":  len(resp.Results),
		"artifact_path": artifactPath,
	})
	if err != nil {
		return RunResult{}, err
	}
	emitProgress(onProgress, "persist.memory", "Writing search results into memory...")
	if err := r.persistWebSearchMemory(task, resp, artifactPath, obsPath); err != nil {
		return RunResult{}, err
	}
	updated, err := r.runtimeStore.MoveTask(task.TaskID, airuntime.TaskStateDone, func(t *airuntime.Task) {
		ensureMetadata(t)
		t.NextAction = "web search completed"
		t.Metadata["web_search_artifact"] = artifactPath
		t.Metadata["web_search_memory_card_id"] = searchMemoryCardID(task.TaskID)
		t.Metadata["web_search_summary_card_id"] = searchSummaryCardID(task.TaskID)
	})
	if err != nil {
		return RunResult{}, err
	}
	if err := r.finalizeSuccessfulTask(updated, onProgress); err != nil {
		return RunResult{}, err
	}
	return RunResult{Task: updated, ArtifactPaths: []string{artifactPath}, ObservationPaths: []string{obsPath}}, nil
}

func (r *Runner) runGitHubIssueSearch(task airuntime.Task, onProgress func(ProgressEvent)) (RunResult, error) {
	if r.connectors == nil {
		return r.blockTask(task, "connector runtime is not configured")
	}
	query := task.Goal
	if task.Metadata != nil {
		query = firstNonEmpty(task.Metadata["query"], query)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	emitProgress(onProgress, "connector.request", "Calling the GitHub connector...")
	resp, err := r.connectors.SearchGitHubIssues(ctx, connectors.GitHubIssueRequest{
		Query: query,
		Limit: 5,
	})
	if err != nil {
		return r.blockTask(task, err.Error())
	}
	emitProgress(onProgress, "persist.artifacts", "Writing GitHub search artifact and observation...")
	artifactPath, err := r.writeArtifact("reports", task.TaskID+"-github-issues.md", renderGitHubIssueArtifact(task, resp))
	if err != nil {
		return RunResult{}, err
	}
	obsPath, err := r.writeObservation("os", task.TaskID+"-github-issues.json", map[string]any{
		"type":          "github-issue-search",
		"task_id":       task.TaskID,
		"status":        "completed",
		"query":         resp.Query,
		"provider":      resp.Provider,
		"result_count":  len(resp.Results),
		"artifact_path": artifactPath,
	})
	if err != nil {
		return RunResult{}, err
	}
	emitProgress(onProgress, "persist.memory", "Writing GitHub issues into memory...")
	if err := r.persistGitHubIssueMemory(task, resp, artifactPath, obsPath); err != nil {
		return RunResult{}, err
	}
	updated, err := r.runtimeStore.MoveTask(task.TaskID, airuntime.TaskStateDone, func(t *airuntime.Task) {
		ensureMetadata(t)
		t.NextAction = "github issue search completed"
		t.Metadata["github_issue_artifact"] = artifactPath
		t.Metadata["github_issue_memory_card_id"] = githubIssueMemoryCardID(task.TaskID)
		t.Metadata["github_issue_summary_card_id"] = githubIssueSummaryCardID(task.TaskID)
	})
	if err != nil {
		return RunResult{}, err
	}
	if err := r.finalizeSuccessfulTask(updated, onProgress); err != nil {
		return RunResult{}, err
	}
	return RunResult{Task: updated, ArtifactPaths: []string{artifactPath}, ObservationPaths: []string{obsPath}}, nil
}

func (r *Runner) runEmailInbox(task airuntime.Task, onProgress func(ProgressEvent)) (RunResult, error) {
	if r.connectors == nil {
		return r.blockTask(task, "connector runtime is not configured")
	}
	query := ""
	if task.Metadata != nil {
		query = strings.TrimSpace(task.Metadata["query"])
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	emitProgress(onProgress, "connector.request", "Calling the email connector...")
	resp, err := r.connectors.ListEmails(ctx, connectors.EmailRequest{
		Query: query,
		Limit: 10,
	})
	if err != nil {
		return r.blockTask(task, err.Error())
	}
	emitProgress(onProgress, "persist.artifacts", "Writing email artifact and observation...")
	artifactPath, err := r.writeArtifact("reports", task.TaskID+"-email-inbox.md", renderEmailArtifact(task, resp))
	if err != nil {
		return RunResult{}, err
	}
	obsPath, err := r.writeObservation("os", task.TaskID+"-email-inbox.json", map[string]any{
		"type":          "email-inbox",
		"task_id":       task.TaskID,
		"status":        "completed",
		"query":         resp.Query,
		"provider":      resp.Provider,
		"result_count":  len(resp.Results),
		"artifact_path": artifactPath,
	})
	if err != nil {
		return RunResult{}, err
	}
	emitProgress(onProgress, "persist.memory", "Writing email threads into memory...")
	if err := r.persistEmailMemory(task, resp, artifactPath, obsPath); err != nil {
		return RunResult{}, err
	}
	updated, err := r.runtimeStore.MoveTask(task.TaskID, airuntime.TaskStateDone, func(t *airuntime.Task) {
		ensureMetadata(t)
		t.NextAction = "email inbox checked"
		t.Metadata["email_artifact"] = artifactPath
		t.Metadata["email_memory_card_id"] = emailMemoryCardID(task.TaskID)
		t.Metadata["email_summary_card_id"] = emailSummaryCardID(task.TaskID)
	})
	if err != nil {
		return RunResult{}, err
	}
	if err := r.finalizeSuccessfulTask(updated, onProgress); err != nil {
		return RunResult{}, err
	}
	return RunResult{Task: updated, ArtifactPaths: []string{artifactPath}, ObservationPaths: []string{obsPath}}, nil
}

func (r *Runner) runMemoryConsolidate(task airuntime.Task, onProgress func(ProgressEvent)) (RunResult, error) {
	if parseBool(task.Metadata["extract_procedures"]) {
		emitProgress(onProgress, "consolidate.procedures", "Extracting procedural candidates from successful task runs...")
		if err := r.extractProcedureCandidates(task); err != nil {
			return RunResult{}, err
		}
		if r.model != nil && parseBool(task.Metadata["smart_extract_procedures"]) {
			if err := r.extractSmartProcedureCandidates(task, onProgress); err != nil {
				emitProgress(onProgress, "consolidate.smart_procedures", "Smart procedural extraction skipped due to transient model error.")
			}
		}
	}
	emitProgress(onProgress, "consolidate.replay", "Reviewing candidate memories for promotion...")
	consolidationResult, err := r.consolidator.PromoteCandidates(memory.ConsolidateRequest{
		CardType: strings.TrimSpace(task.Metadata["card_type"]),
		Scope:    strings.TrimSpace(task.Metadata["scope"]),
	})
	if err != nil {
		return RunResult{}, err
	}
	emitProgress(onProgress, "consolidate.generate", "Generating the memory summary...")
	now := time.Now().UTC()
	body, modelResp, modelErr := r.generateMemorySummary(task, now)
	body = fmt.Sprintf("%s\nPromoted candidates: %d\nExamined candidates: %d\n", body, consolidationResult.Promoted, consolidationResult.Examined)
	body = attachProcedureHintsToPlan(body, task.Metadata)
	emitProgress(onProgress, "persist.artifacts", "Writing the memory artifact and observation...")
	artifactPath, err := r.writeArtifact("reports", task.TaskID+"-memory.md", body)
	if err != nil {
		return RunResult{}, err
	}
	payload := map[string]any{
		"type":           "memory-consolidate",
		"task_id":        task.TaskID,
		"generated_at":   now.Format(time.RFC3339),
		"model_provider": modelResp.Provider,
		"model_name":     modelResp.Model,
		"model_error":    errorString(modelErr),
		"examined":       consolidationResult.Examined,
		"promoted":       consolidationResult.Promoted,
		"promoted_cards": consolidationResult.PromotedCards,
		"artifact_path":  artifactPath,
	}
	if hints := procedureHintsFromMetadata(task.Metadata); hints.hasEvidence() {
		payload["procedure_task_class"] = firstNonEmpty(strings.TrimSpace(task.Metadata["procedure_task_class"]), strings.TrimSpace(task.Metadata["task_class"]))
		payload["procedure_steps"] = hints.Steps
		payload["procedure_guardrails"] = hints.Guardrails
		payload["procedure_summary"] = hints.Summary
		payload["procedure_success_signal"] = hints.SuccessSignal
	}
	obsPath, err := r.writeObservation("os", task.TaskID+"-memory.json", payload)
	if err != nil {
		return RunResult{}, err
	}
	updated, err := r.runtimeStore.MoveTask(task.TaskID, airuntime.TaskStateDone, func(t *airuntime.Task) {
		ensureMetadata(t)
		t.NextAction = "memory consolidation recorded"
		t.Metadata["memory_artifact"] = artifactPath
		t.Metadata["memory_observation"] = obsPath
	})
	if err != nil {
		return RunResult{}, err
	}
	if err := r.clearActiveTask(updated.TaskID); err != nil {
		return RunResult{}, err
	}
	return RunResult{Task: updated, ArtifactPaths: []string{artifactPath}, ObservationPaths: []string{obsPath}}, nil
}

func (r *Runner) extractProcedureCandidates(task airuntime.Task) error {
	tasks, err := r.runtimeStore.ListTasks()
	if err != nil {
		return err
	}
	minRuns := parseInt(task.Metadata["min_runs"], 2)
	candidates, _ := memory.BuildProcedureCandidates(memory.ProcedureExtractionRequest{
		Tasks:            tasks,
		TaskClass:        strings.TrimSpace(task.Metadata["task_class"]),
		SelectedSkill:    strings.TrimSpace(task.Metadata["selected_skill"]),
		Scope:            firstNonEmpty(strings.TrimSpace(task.Metadata["scope"]), memoryScopeProject),
		MinRuns:          minRuns,
		EvidenceResolver: r.procedureEvidenceForTask,
	})
	for _, candidate := range candidates {
		if _, err := r.memoryStore.CreateCard(candidate); err != nil && !errors.Is(err, memory.ErrAlreadyExists) {
			return err
		}
	}
	return nil
}

func (r *Runner) extractSmartProcedureCandidates(task airuntime.Task, onProgress func(ProgressEvent)) error {
	tasks, err := r.runtimeStore.ListTasks()
	if err != nil {
		return err
	}
	minRuns := parseInt(task.Metadata["min_runs"], 2)
	if minRuns <= 0 {
		minRuns = 2
	}
	scope := firstNonEmpty(strings.TrimSpace(task.Metadata["scope"]), memoryScopeProject)

	groups := make(map[string][]airuntime.Task)
	for _, t := range tasks {
		if t.State != airuntime.TaskStateDone {
			continue
		}
		if strings.TrimSpace(t.Metadata["procedure_steps"]) != "" {
			continue
		}
		taskClass := firstNonEmpty(strings.TrimSpace(t.Metadata["procedure_task_class"]), strings.TrimSpace(t.Metadata["task_class"]))
		selectedSkill := strings.TrimSpace(t.SelectedSkill)
		if taskClass == "" || selectedSkill == "" {
			continue
		}
		key := fmt.Sprintf("%s|%s", taskClass, selectedSkill)
		groups[key] = append(groups[key], t)
	}

	for key, group := range groups {
		if len(group) < minRuns {
			continue
		}
		parts := strings.SplitN(key, "|", 2)
		taskClass := parts[0]
		selectedSkill := parts[1]

		existing := r.memoryStore.Query(memory.QueryRequest{
			CardType: "procedure",
			Scope:    scope,
		})
		alreadyExists := false
		for _, card := range existing.Cards {
			if fmt.Sprint(card.Content["task_class"]) == taskClass && fmt.Sprint(card.Content["selected_skill"]) == selectedSkill && (card.Status == memory.CardStatusActive || card.Status == memory.CardStatusCandidate) {
				alreadyExists = true
				break
			}
		}
		if alreadyExists {
			continue
		}

		emitProgress(onProgress, "consolidate.smart_procedures", fmt.Sprintf("Extracting smart procedural template for %s...", taskClass))

		var evidenceBuilder strings.Builder
		for i, t := range group {
			if i >= minRuns {
				break
			}
			evidenceBuilder.WriteString(fmt.Sprintf("Run %d:\nTitle: %s\nGoal: %s\n\n", i+1, t.Title, t.Goal))
		}

		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		resp, err := r.model.GenerateText(ctx, model.TextRequest{
			SystemPrompt: "You are the procedural memory extractor of an AgentOS. You extract standard operating procedures (steps, guardrails, success signal) from a series of successful task runs. Return ONLY a valid JSON object matching this schema: {\"summary\": \"...\", \"steps\": \"step1\\nstep2\", \"guardrails\": \"rule1\\nrule2\", \"success_signal\": \"...\"}. Do not wrap in markdown code blocks.",
			UserPrompt:   fmt.Sprintf("Extract a generalized procedure for task class '%s' using skill '%s' based on these successful runs:\n%s", taskClass, selectedSkill, evidenceBuilder.String()),
			MaxTokens:    800,
			Temperature:  0.1,
			Profile:      model.ProfileSkills,
		})
		cancel()

		if err != nil {
			continue
		}

		text := strings.TrimSpace(resp.Text)
		if strings.HasPrefix(text, "```json") {
			text = strings.TrimPrefix(text, "```json")
			text = strings.TrimSuffix(text, "```")
			text = strings.TrimSpace(text)
		} else if strings.HasPrefix(text, "```") {
			text = strings.TrimPrefix(text, "```")
			text = strings.TrimSuffix(text, "```")
			text = strings.TrimSpace(text)
		}

		var parsed struct {
			Summary       string `json:"summary"`
			Steps         string `json:"steps"`
			Guardrails    string `json:"guardrails"`
			SuccessSignal string `json:"success_signal"`
		}
		if err := json.Unmarshal([]byte(text), &parsed); err != nil {
			continue
		}
		if strings.TrimSpace(parsed.Steps) == "" {
			continue
		}

		signature := fmt.Sprintf("%s|%s|%s|%s", taskClass, selectedSkill, parsed.Steps, parsed.Guardrails)
		sum := sha1.Sum([]byte(signature))

		sanitizedPrefix := strings.ReplaceAll(taskClass, " ", "_")
		if len(sanitizedPrefix) == 0 {
			sanitizedPrefix = "generic"
		}
		cardID := fmt.Sprintf("procedure:%s:%x", sanitizedPrefix, sum[:6])

		supportingRuns := make([]string, 0, len(group))
		for _, t := range group {
			supportingRuns = append(supportingRuns, t.TaskID)
		}

		card := memory.CreateCardRequest{
			CardID:   cardID,
			CardType: "procedure",
			Scope:    scope,
			Status:   memory.CardStatusCandidate,
			Content: map[string]any{
				"name":            fmt.Sprintf("%s_smart", sanitizedPrefix),
				"task_class":      taskClass,
				"selected_skill":  selectedSkill,
				"summary":         strings.TrimSpace(parsed.Summary),
				"steps":           strings.TrimSpace(parsed.Steps),
				"guardrails":      strings.TrimSpace(parsed.Guardrails),
				"success_signal":  strings.TrimSpace(parsed.SuccessSignal),
				"supporting_runs": supportingRuns,
			},
			Provenance: memory.Provenance{
				Source:     "smart-procedure-extractor",
				Confidence: 0.8,
			},
		}

		if _, err := r.memoryStore.CreateCard(card); err != nil && !errors.Is(err, memory.ErrAlreadyExists) {
			return err
		}
	}
	return nil
}

type procedureHints struct {
	Steps         string
	Guardrails    string
	Summary       string
	SuccessSignal string
}

func procedureHintsFromMetadata(metadata map[string]string) procedureHints {
	return procedureHints{
		Steps:         strings.TrimSpace(metadata["procedure_steps"]),
		Guardrails:    strings.TrimSpace(metadata["procedure_guardrails"]),
		Summary:       strings.TrimSpace(metadata["procedure_summary"]),
		SuccessSignal: strings.TrimSpace(metadata["procedure_success_signal"]),
	}
}

func (h procedureHints) hasEvidence() bool {
	return strings.TrimSpace(h.Steps) != ""
}

func attachProcedureHintsToPlan(body string, metadata map[string]string) string {
	hints := procedureHintsFromMetadata(metadata)
	if !hints.hasEvidence() {
		return body
	}
	var b strings.Builder
	b.WriteString(strings.TrimRight(body, "\n"))
	b.WriteString("\n\n## Procedure Candidate\n\n")
	if strings.TrimSpace(hints.Summary) != "" {
		fmt.Fprintf(&b, "Summary: %s\n\n", hints.Summary)
	}
	b.WriteString("Steps:\n")
	for _, line := range strings.Split(hints.Steps, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fmt.Fprintf(&b, "- %s\n", line)
	}
	if strings.TrimSpace(hints.Guardrails) != "" {
		b.WriteString("\nGuardrails:\n")
		for _, line := range strings.Split(hints.Guardrails, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			fmt.Fprintf(&b, "- %s\n", line)
		}
	}
	if strings.TrimSpace(hints.SuccessSignal) != "" {
		fmt.Fprintf(&b, "\nSuccess signal: %s\n", hints.SuccessSignal)
	}
	b.WriteString("\n")
	return b.String()
}

func (r *Runner) procedureEvidenceForTask(task airuntime.Task) memory.ProcedureEvidence {
	for _, key := range metadataKeysBySuffix(task.Metadata, "_observation") {
		if evidence := readProcedureEvidenceObservation(strings.TrimSpace(task.Metadata[key])); strings.TrimSpace(evidence.Steps) != "" {
			return evidence
		}
	}
	for _, key := range metadataKeysBySuffix(task.Metadata, "_artifact") {
		if evidence := readProcedureEvidenceArtifact(strings.TrimSpace(task.Metadata[key])); strings.TrimSpace(evidence.Steps) != "" {
			return evidence
		}
	}
	hints := procedureHintsFromMetadata(task.Metadata)
	return memory.ProcedureEvidence{
		Steps:           hints.Steps,
		Guardrails:      hints.Guardrails,
		Summary:         hints.Summary,
		SuccessSignal:   hints.SuccessSignal,
		ArtifactPath:    firstMetadataValue(task.Metadata, "_artifact"),
		ObservationPath: firstMetadataValue(task.Metadata, "_observation"),
	}
}

func metadataKeysBySuffix(metadata map[string]string, suffix string) []string {
	keys := make([]string, 0)
	for key, value := range metadata {
		if strings.HasSuffix(key, suffix) && strings.TrimSpace(value) != "" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	return keys
}

func firstMetadataValue(metadata map[string]string, suffix string) string {
	keys := metadataKeysBySuffix(metadata, suffix)
	if len(keys) == 0 {
		return ""
	}
	return strings.TrimSpace(metadata[keys[0]])
}

func readProcedureEvidenceObservation(path string) memory.ProcedureEvidence {
	if strings.TrimSpace(path) == "" {
		return memory.ProcedureEvidence{}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return memory.ProcedureEvidence{}
	}
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		return memory.ProcedureEvidence{}
	}
	evidence := memory.ProcedureEvidence{
		Steps:           strings.TrimSpace(stringValue(payload["procedure_steps"])),
		Guardrails:      strings.TrimSpace(stringValue(payload["procedure_guardrails"])),
		Summary:         strings.TrimSpace(stringValue(payload["procedure_summary"])),
		SuccessSignal:   strings.TrimSpace(stringValue(payload["procedure_success_signal"])),
		ArtifactPath:    strings.TrimSpace(stringValue(payload["artifact_path"])),
		ObservationPath: path,
	}
	if strings.TrimSpace(evidence.Summary) == "" {
		evidence.Summary = strings.TrimSpace(stringValue(payload["summary"]))
	}
	return evidence
}

func readProcedureEvidenceArtifact(path string) memory.ProcedureEvidence {
	if strings.TrimSpace(path) == "" {
		return memory.ProcedureEvidence{}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return memory.ProcedureEvidence{}
	}
	text := string(data)
	steps := markdownSectionBullets(text, "Steps:")
	if strings.TrimSpace(steps) == "" {
		return memory.ProcedureEvidence{}
	}
	return memory.ProcedureEvidence{
		Steps:         steps,
		Guardrails:    markdownSectionBullets(text, "Guardrails:"),
		Summary:       markdownInlinePrefix(text, "Summary:"),
		SuccessSignal: markdownInlinePrefix(text, "Success signal:"),
		ArtifactPath:  path,
	}
}

func markdownInlinePrefix(text, prefix string) string {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(line, prefix))
		}
	}
	return ""
}

func markdownSectionBullets(text, header string) string {
	lines := strings.Split(text, "\n")
	inSection := false
	var collected []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		switch {
		case trimmed == header:
			inSection = true
			continue
		case !inSection:
			continue
		case trimmed == "":
			if len(collected) > 0 {
				break
			}
			continue
		case strings.HasPrefix(trimmed, "## "):
			return strings.Join(collected, "\n")
		case strings.HasPrefix(trimmed, "- "):
			collected = append(collected, strings.TrimSpace(strings.TrimPrefix(trimmed, "- ")))
		}
	}
	return strings.Join(collected, "\n")
}

func stringValue(value any) string {
	if text, ok := value.(string); ok {
		return text
	}
	return ""
}

func (r *Runner) blockTask(task airuntime.Task, reason string) (RunResult, error) {
	updated, err := r.runtimeStore.MoveTask(task.TaskID, airuntime.TaskStateBlocked, func(t *airuntime.Task) {
		t.FailureReason = reason
		t.NextAction = "blocked"
	})
	if err != nil {
		return RunResult{}, err
	}
	if err := r.clearActiveTask(updated.TaskID); err != nil {
		return RunResult{}, err
	}
	return RunResult{Task: updated}, nil
}

func (r *Runner) blockSearchTask(task airuntime.Task, reason string) (RunResult, error) {
	obsPath, err := r.writeObservation("os", task.TaskID+"-web-search.json", map[string]any{
		"type":    "web-search",
		"task_id": task.TaskID,
		"status":  "blocked",
		"reason":  reason,
		"goal":    task.Goal,
	})
	if err != nil {
		return RunResult{}, err
	}
	result, err := r.blockTask(task, reason)
	if err != nil {
		return RunResult{}, err
	}
	result.ObservationPaths = []string{obsPath}
	return result, nil
}

func (r *Runner) finalizeSuccessfulTask(task airuntime.Task, onProgress func(ProgressEvent)) error {
	if err := r.clearActiveTask(task.TaskID); err != nil {
		return err
	}
	if task.State != airuntime.TaskStateDone {
		return nil
	}
	scheduler, ok := r.schedulerForSkill(task.SelectedSkill)
	if !ok {
		return nil
	}
	decision, err := scheduler.EvaluateAndSchedule("task_completion")
	if err != nil {
		return nil
	}
	if decision.Triggered {
		emitProgress(onProgress, "maintenance.schedule", "Queued memory consolidation maintenance.")
	}
	return nil
}

func (r *Runner) schedulerForSkill(selectedSkill string) (*memoryscheduler.Scheduler, bool) {
	if r == nil || r.registry == nil {
		return nil, false
	}
	def, ok := r.registry.Resolve(strings.TrimSpace(selectedSkill))
	if !ok || !def.Enabled || def.MaintenancePolicy == nil || !def.MaintenancePolicy.Enabled {
		return nil, false
	}
	config := r.schedulerCfg
	if strings.TrimSpace(def.MaintenancePolicy.Scope) != "" {
		config.Scope = strings.TrimSpace(def.MaintenancePolicy.Scope)
	}
	if len(def.MaintenancePolicy.AllowedCardTypes) > 0 {
		config.AllowedCardTypes = append([]string(nil), def.MaintenancePolicy.AllowedCardTypes...)
	}
	if def.MaintenancePolicy.MinCandidates > 0 {
		config.MinCandidates = def.MaintenancePolicy.MinCandidates
	}
	return memoryscheduler.New(r.runtimeStore, r.memoryStore, config), true
}

func (r *Runner) clearActiveTask(taskID string) error {
	state, err := r.runtimeStore.LoadState()
	if err != nil {
		return err
	}
	if state.ActiveTaskID != nil && *state.ActiveTaskID == taskID {
		state.ActiveTaskID = nil
		state.Status = "idle"
	}
	return r.runtimeStore.SaveState(state)
}

func (r *Runner) writeArtifact(kind, name, content string) (string, error) {
	path := filepath.Join(r.runtimeStore.RootDir(), "artifacts", kind, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func (r *Runner) writeObservation(kind, name string, payload map[string]any) (string, error) {
	path := filepath.Join(r.runtimeStore.RootDir(), "observations", kind, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "", err
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func emitProgress(onProgress func(ProgressEvent), stage, message string) {
	if onProgress == nil {
		return
	}
	onProgress(ProgressEvent{
		Stage:   strings.TrimSpace(stage),
		Message: strings.TrimSpace(message),
	})
}

func parseBool(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
}

func parseInt(raw string, fallback int) int {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback
	}
	var parsed int
	if _, err := fmt.Sscanf(raw, "%d", &parsed); err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func ensureMetadata(task *airuntime.Task) {
	if task.Metadata == nil {
		task.Metadata = map[string]string{}
	}
}

func renderSearchArtifact(task airuntime.Task, resp connectors.SearchResponse) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Web Search Result\n\n")
	fmt.Fprintf(&b, "Task: %s\n\n", task.Title)
	fmt.Fprintf(&b, "Goal: %s\n\n", task.Goal)
	fmt.Fprintf(&b, "Query: %s\n\n", resp.Query)
	fmt.Fprintf(&b, "Provider: %s\n\n", resp.Provider)
	if len(resp.Results) == 0 {
		fmt.Fprintf(&b, "No results returned.\n")
		return b.String()
	}
	fmt.Fprintf(&b, "Results:\n")
	for _, result := range resp.Results {
		fmt.Fprintf(&b, "- %s (%s)\n", result.Title, result.URL)
		if result.Snippet != "" {
			fmt.Fprintf(&b, "  %s\n", previewText(result.Snippet, 300))
		}
	}
	return b.String()
}

func renderGitHubIssueArtifact(task airuntime.Task, resp connectors.GitHubIssueResponse) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# GitHub Issue Search Result\n\n")
	fmt.Fprintf(&b, "Task: %s\n\n", task.Title)
	fmt.Fprintf(&b, "Goal: %s\n\n", task.Goal)
	fmt.Fprintf(&b, "Query: %s\n\n", resp.Query)
	fmt.Fprintf(&b, "Provider: %s\n\n", resp.Provider)
	if len(resp.Results) == 0 {
		fmt.Fprintf(&b, "No issues returned.\n")
		return b.String()
	}
	fmt.Fprintf(&b, "Issues:\n")
	for _, result := range resp.Results {
		fmt.Fprintf(&b, "- #%d %s (%s)\n", result.Number, result.Title, result.State)
		fmt.Fprintf(&b, "  %s\n", result.URL)
		if result.Body != "" {
			fmt.Fprintf(&b, "  %s\n", previewText(result.Body, 240))
		}
	}
	return b.String()
}

func renderEmailArtifact(task airuntime.Task, resp connectors.EmailResponse) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Email Inbox Result\n\n")
	fmt.Fprintf(&b, "Task: %s\n\n", task.Title)
	fmt.Fprintf(&b, "Goal: %s\n\n", task.Goal)
	if strings.TrimSpace(resp.Query) != "" {
		fmt.Fprintf(&b, "Query: %s\n\n", resp.Query)
	}
	fmt.Fprintf(&b, "Provider: %s\n\n", resp.Provider)
	if len(resp.Results) == 0 {
		fmt.Fprintf(&b, "No messages returned.\n")
		return b.String()
	}
	fmt.Fprintf(&b, "Messages:\n")
	for _, msg := range resp.Results {
		state := "read"
		if msg.Unread {
			state = "unread"
		}
		fmt.Fprintf(&b, "- %s [%s]\n", firstNonEmpty(msg.Subject, "(no subject)"), state)
		fmt.Fprintf(&b, "  From: %s\n", firstNonEmpty(msg.From, "unknown"))
		if msg.Date != "" {
			fmt.Fprintf(&b, "  Date: %s\n", msg.Date)
		}
		if msg.Snippet != "" {
			fmt.Fprintf(&b, "  %s\n", previewText(msg.Snippet, 220))
		}
	}
	return b.String()
}

func (r *Runner) persistWebSearchMemory(task airuntime.Task, resp connectors.SearchResponse, artifactPath, observationPath string) error {
	if r.memoryStore == nil {
		return nil
	}

	rootCardID := searchMemoryCardID(task.TaskID)
	summaryCardID := searchSummaryCardID(task.TaskID)
	summaryText := r.summarizeSearchResults(task, resp)
	rootContent := map[string]any{
		"task_id":          task.TaskID,
		"title":            task.Title,
		"goal":             task.Goal,
		"query":            resp.Query,
		"provider":         resp.Provider,
		"result_count":     len(resp.Results),
		"persisted_count":  len(resp.Results),
		"artifact_path":    artifactPath,
		"observation_path": observationPath,
		"summary_card_id":  summaryCardID,
	}
	if err := upsertCard(r.memoryStore, rootCardID, memory.CreateCardRequest{
		CardID:   rootCardID,
		CardType: "web_search",
		Scope:    memoryScopeProject,
		Content:  rootContent,
		Provenance: memory.Provenance{
			AgentID:    "skills.web-search",
			Source:     "web-search",
			Confidence: 0.9,
		},
	}); err != nil {
		return err
	}
	if err := upsertCard(r.memoryStore, summaryCardID, memory.CreateCardRequest{
		CardID:   summaryCardID,
		CardType: "search_summary",
		Scope:    memoryScopeProject,
		Content: map[string]any{
			"task_id":   task.TaskID,
			"query":     resp.Query,
			"provider":  resp.Provider,
			"summary":   summaryText,
			"parent_id": rootCardID,
		},
		Provenance: memory.Provenance{
			AgentID:    "skills.web-search",
			Source:     "search-summary",
			Confidence: 0.8,
		},
		EvidenceRefs: []memory.EvidenceRef{
			{CardID: rootCardID, Snippet: previewText(summaryText, 180)},
		},
	}); err != nil {
		return err
	}
	if _, err := r.memoryStore.CreateEdge(memory.CreateEdgeRequest{
		EdgeID:     searchSummaryEdgeID(task.TaskID),
		FromCardID: rootCardID,
		ToCardID:   summaryCardID,
		EdgeType:   "search_summary",
		Weight:     1.0,
		Confidence: 0.8,
		EvidenceRefs: []memory.EvidenceRef{
			{CardID: summaryCardID, Snippet: previewText(summaryText, 180)},
		},
	}); err != nil && !errors.Is(err, memory.ErrAlreadyExists) {
		return err
	}

	for idx, result := range resp.Results {
		resultCardID := canonicalSearchResultCardID(result.URL)
		resultContent := map[string]any{
			"task_id":   task.TaskID,
			"query":     resp.Query,
			"provider":  resp.Provider,
			"rank":      idx + 1,
			"title":     result.Title,
			"url":       result.URL,
			"url_key":   canonicalURL(result.URL),
			"snippet":   result.Snippet,
			"parent_id": rootCardID,
		}
		if err := upsertCard(r.memoryStore, resultCardID, memory.CreateCardRequest{
			CardID:   resultCardID,
			CardType: "web_result",
			Scope:    memoryScopeProject,
			Status:   memory.CardStatusCandidate,
			Content:  resultContent,
			Provenance: memory.Provenance{
				AgentID:    "skills.web-search",
				Source:     resp.Provider,
				Confidence: 0.75,
			},
			EvidenceRefs: []memory.EvidenceRef{
				{CardID: rootCardID, Snippet: previewText(result.Snippet, 180)},
			},
		}); err != nil {
			return err
		}
		edgeID := searchResultEdgeID(task.TaskID, idx)
		if _, err := r.memoryStore.CreateEdge(memory.CreateEdgeRequest{
			EdgeID:     edgeID,
			FromCardID: rootCardID,
			ToCardID:   resultCardID,
			EdgeType:   "search_result",
			Weight:     1.0 / float64(idx+1),
			Confidence: 0.75,
			EvidenceRefs: []memory.EvidenceRef{
				{CardID: resultCardID, Snippet: previewText(result.Snippet, 180)},
			},
		}); err != nil && !errors.Is(err, memory.ErrAlreadyExists) {
			return err
		}
	}

	return nil
}

func (r *Runner) persistGitHubIssueMemory(task airuntime.Task, resp connectors.GitHubIssueResponse, artifactPath, observationPath string) error {
	if r.memoryStore == nil {
		return nil
	}

	rootCardID := githubIssueMemoryCardID(task.TaskID)
	summaryCardID := githubIssueSummaryCardID(task.TaskID)
	summaryText := r.summarizeGitHubIssues(task, resp)
	rootContent := map[string]any{
		"task_id":          task.TaskID,
		"title":            task.Title,
		"goal":             task.Goal,
		"query":            resp.Query,
		"provider":         resp.Provider,
		"result_count":     len(resp.Results),
		"persisted_count":  len(resp.Results),
		"artifact_path":    artifactPath,
		"observation_path": observationPath,
		"summary_card_id":  summaryCardID,
	}
	if err := upsertCard(r.memoryStore, rootCardID, memory.CreateCardRequest{
		CardID:   rootCardID,
		CardType: "github_issue_search",
		Scope:    memoryScopeProject,
		Content:  rootContent,
		Provenance: memory.Provenance{
			AgentID:    "skills.github-issue-search",
			Source:     "github-issue-search",
			Confidence: 0.9,
		},
	}); err != nil {
		return err
	}
	if err := upsertCard(r.memoryStore, summaryCardID, memory.CreateCardRequest{
		CardID:   summaryCardID,
		CardType: "github_issue_summary",
		Scope:    memoryScopeProject,
		Content: map[string]any{
			"task_id":   task.TaskID,
			"query":     resp.Query,
			"provider":  resp.Provider,
			"summary":   summaryText,
			"parent_id": rootCardID,
		},
		Provenance: memory.Provenance{
			AgentID:    "skills.github-issue-search",
			Source:     "github-issue-summary",
			Confidence: 0.8,
		},
		EvidenceRefs: []memory.EvidenceRef{
			{CardID: rootCardID, Snippet: previewText(summaryText, 180)},
		},
	}); err != nil {
		return err
	}
	if _, err := r.memoryStore.CreateEdge(memory.CreateEdgeRequest{
		EdgeID:     githubIssueSummaryEdgeID(task.TaskID),
		FromCardID: rootCardID,
		ToCardID:   summaryCardID,
		EdgeType:   "github_issue_summary",
		Weight:     1.0,
		Confidence: 0.8,
		EvidenceRefs: []memory.EvidenceRef{
			{CardID: summaryCardID, Snippet: previewText(summaryText, 180)},
		},
	}); err != nil && !errors.Is(err, memory.ErrAlreadyExists) {
		return err
	}

	for idx, issue := range resp.Results {
		issueCardID := canonicalGitHubIssueCardID(issue)
		issueContent := map[string]any{
			"task_id":   task.TaskID,
			"query":     resp.Query,
			"provider":  resp.Provider,
			"rank":      idx + 1,
			"number":    issue.Number,
			"title":     issue.Title,
			"url":       issue.URL,
			"state":     issue.State,
			"body":      issue.Body,
			"repo":      issue.Repo,
			"parent_id": rootCardID,
		}
		if err := upsertCard(r.memoryStore, issueCardID, memory.CreateCardRequest{
			CardID:   issueCardID,
			CardType: "github_issue",
			Scope:    memoryScopeProject,
			Status:   memory.CardStatusCandidate,
			Content:  issueContent,
			Provenance: memory.Provenance{
				AgentID:    "skills.github-issue-search",
				Source:     resp.Provider,
				Confidence: 0.75,
			},
			EvidenceRefs: []memory.EvidenceRef{
				{CardID: rootCardID, Snippet: previewText(firstNonEmpty(issue.Body, issue.Title), 180)},
			},
		}); err != nil {
			return err
		}
		if _, err := r.memoryStore.CreateEdge(memory.CreateEdgeRequest{
			EdgeID:     githubIssueEdgeID(task.TaskID, idx),
			FromCardID: rootCardID,
			ToCardID:   issueCardID,
			EdgeType:   "github_issue",
			Weight:     1.0 / float64(idx+1),
			Confidence: 0.75,
			EvidenceRefs: []memory.EvidenceRef{
				{CardID: issueCardID, Snippet: previewText(firstNonEmpty(issue.Body, issue.Title), 180)},
			},
		}); err != nil && !errors.Is(err, memory.ErrAlreadyExists) {
			return err
		}
	}

	return nil
}

func (r *Runner) persistEmailMemory(task airuntime.Task, resp connectors.EmailResponse, artifactPath, observationPath string) error {
	if r.memoryStore == nil {
		return nil
	}

	rootCardID := emailMemoryCardID(task.TaskID)
	summaryCardID := emailSummaryCardID(task.TaskID)
	summaryText := r.summarizeEmailResults(task, resp)
	threadGroups := groupEmailThreads(resp.Results)
	rootContent := map[string]any{
		"task_id":          task.TaskID,
		"title":            task.Title,
		"goal":             task.Goal,
		"query":            resp.Query,
		"provider":         resp.Provider,
		"result_count":     len(resp.Results),
		"persisted_count":  len(resp.Results),
		"thread_count":     len(threadGroups),
		"artifact_path":    artifactPath,
		"observation_path": observationPath,
		"summary_card_id":  summaryCardID,
	}
	if err := upsertCard(r.memoryStore, rootCardID, memory.CreateCardRequest{
		CardID:   rootCardID,
		CardType: "email_inbox",
		Scope:    memoryScopeUser,
		Content:  rootContent,
		Provenance: memory.Provenance{
			AgentID:    "skills.email-inbox",
			Source:     "email-inbox",
			Confidence: 0.9,
		},
	}); err != nil {
		return err
	}
	if err := upsertCard(r.memoryStore, summaryCardID, memory.CreateCardRequest{
		CardID:   summaryCardID,
		CardType: "email_summary",
		Scope:    memoryScopeUser,
		Content: map[string]any{
			"task_id":   task.TaskID,
			"query":     resp.Query,
			"provider":  resp.Provider,
			"summary":   summaryText,
			"parent_id": rootCardID,
		},
		Provenance: memory.Provenance{
			AgentID:    "skills.email-inbox",
			Source:     "email-summary",
			Confidence: 0.8,
		},
		EvidenceRefs: []memory.EvidenceRef{
			{CardID: rootCardID, Snippet: previewText(summaryText, 180)},
		},
	}); err != nil {
		return err
	}
	if _, err := r.memoryStore.CreateEdge(memory.CreateEdgeRequest{
		EdgeID:     emailSummaryEdgeID(task.TaskID),
		FromCardID: rootCardID,
		ToCardID:   summaryCardID,
		EdgeType:   "email_summary",
		Weight:     1.0,
		Confidence: 0.8,
		EvidenceRefs: []memory.EvidenceRef{
			{CardID: summaryCardID, Snippet: previewText(summaryText, 180)},
		},
	}); err != nil && !errors.Is(err, memory.ErrAlreadyExists) {
		return err
	}

	for _, group := range threadGroups {
		threadCardID := canonicalEmailThreadCardID(group.key)
		threadContent := map[string]any{
			"task_id":       task.TaskID,
			"query":         resp.Query,
			"provider":      resp.Provider,
			"thread_key":    group.key,
			"subject":       group.subject,
			"message_count": len(group.messages),
			"unread_count":  group.unreadCount,
			"latest_date":   group.latestDate,
			"participants":  group.participants,
			"parent_id":     rootCardID,
		}
		if err := upsertCard(r.memoryStore, threadCardID, memory.CreateCardRequest{
			CardID:   threadCardID,
			CardType: "email_thread",
			Scope:    memoryScopeUser,
			Status:   memory.CardStatusCandidate,
			Content:  threadContent,
			Provenance: memory.Provenance{
				AgentID:    "skills.email-inbox",
				Source:     resp.Provider,
				Confidence: 0.8,
			},
			EvidenceRefs: []memory.EvidenceRef{
				{CardID: rootCardID, Snippet: previewText(firstNonEmpty(group.summarySnippet(), group.subject), 180)},
			},
		}); err != nil {
			return err
		}
		if _, err := r.memoryStore.CreateEdge(memory.CreateEdgeRequest{
			EdgeID:     emailThreadEdgeID(task.TaskID, group.key),
			FromCardID: rootCardID,
			ToCardID:   threadCardID,
			EdgeType:   "email_thread",
			Weight:     1.0,
			Confidence: 0.8,
			EvidenceRefs: []memory.EvidenceRef{
				{CardID: threadCardID, Snippet: previewText(firstNonEmpty(group.summarySnippet(), group.subject), 180)},
			},
		}); err != nil && !errors.Is(err, memory.ErrAlreadyExists) {
			return err
		}
	}

	for idx, msg := range resp.Results {
		messageCardID := canonicalEmailMessageCardID(msg)
		threadKey := canonicalEmailThreadKey(msg)
		threadCardID := canonicalEmailThreadCardID(threadKey)
		messageContent := map[string]any{
			"task_id":    task.TaskID,
			"query":      resp.Query,
			"provider":   resp.Provider,
			"rank":       idx + 1,
			"message_id": msg.MessageID,
			"mailbox":    msg.Mailbox,
			"from":       msg.From,
			"subject":    msg.Subject,
			"date":       msg.Date,
			"snippet":    msg.Snippet,
			"path":       msg.Path,
			"unread":     msg.Unread,
			"thread_key": threadKey,
			"thread_id":  threadCardID,
			"parent_id":  rootCardID,
		}
		if err := upsertCard(r.memoryStore, messageCardID, memory.CreateCardRequest{
			CardID:   messageCardID,
			CardType: "email_message",
			Scope:    memoryScopeUser,
			Status:   memory.CardStatusCandidate,
			Content:  messageContent,
			Provenance: memory.Provenance{
				AgentID:    "skills.email-inbox",
				Source:     resp.Provider,
				Confidence: 0.75,
			},
			EvidenceRefs: []memory.EvidenceRef{
				{CardID: rootCardID, Snippet: previewText(msg.Snippet, 180)},
			},
		}); err != nil {
			return err
		}
		if _, err := r.memoryStore.CreateEdge(memory.CreateEdgeRequest{
			EdgeID:     emailMessageEdgeID(task.TaskID, idx),
			FromCardID: rootCardID,
			ToCardID:   messageCardID,
			EdgeType:   "email_message",
			Weight:     1.0 / float64(idx+1),
			Confidence: 0.75,
			EvidenceRefs: []memory.EvidenceRef{
				{CardID: messageCardID, Snippet: previewText(msg.Snippet, 180)},
			},
		}); err != nil && !errors.Is(err, memory.ErrAlreadyExists) {
			return err
		}
		if _, err := r.memoryStore.CreateEdge(memory.CreateEdgeRequest{
			EdgeID:     emailThreadMessageEdgeID(task.TaskID, idx),
			FromCardID: threadCardID,
			ToCardID:   messageCardID,
			EdgeType:   "thread_message",
			Weight:     1.0,
			Confidence: 0.75,
			EvidenceRefs: []memory.EvidenceRef{
				{CardID: messageCardID, Snippet: previewText(msg.Snippet, 180)},
			},
		}); err != nil && !errors.Is(err, memory.ErrAlreadyExists) {
			return err
		}
	}

	return nil
}

func (r *Runner) requestRootApproval(task airuntime.Task, actionKind, summary string, metadata map[string]string) (RunResult, error) {
	if r.approvals == nil {
		return r.blockTask(task, "root execution requested but approval flow is not configured")
	}
	if task.Metadata != nil {
		if approvalID := strings.TrimSpace(task.Metadata["root_approval_id"]); approvalID != "" {
			existing, err := r.approvals.Get(approvalID)
			if err == nil && existing.Status == approval.StatusPending {
				return r.awaitApproval(task, existing)
			}
		}
	}

	record, err := r.approvals.Create(approval.CreateRequest{
		TaskID:           task.TaskID,
		ExecutionProfile: "root",
		ActionKind:       actionKind,
		Summary:          summary,
		RequestedBy:      firstNonEmpty(task.RequestedBy, "runtime"),
		Metadata:         metadata,
	})
	if err != nil {
		return RunResult{}, err
	}
	return r.awaitApproval(task, record)
}

func (r *Runner) resolveRootApproval(task airuntime.Task, actionKind, summary string, metadata map[string]string) (string, *RunResult, error) {
	if task.ExecutionProfile != "root" {
		return "", nil, nil
	}
	if r.approvals == nil {
		result, err := r.blockTask(task, "root execution requested but approval flow is not configured")
		return "", &result, err
	}
	approvalID := ""
	if task.Metadata != nil {
		approvalID = task.Metadata["root_approval_id"]
	}
	if strings.TrimSpace(approvalID) == "" {
		result, err := r.requestRootApproval(task, actionKind, summary, metadata)
		return "", &result, err
	}
	record, err := r.approvals.Get(approvalID)
	if err != nil {
		result, blockErr := r.blockTask(task, "root approval record is missing")
		return "", &result, blockErr
	}
	switch record.Status {
	case approval.StatusPending:
		result, err := r.awaitApproval(task, record)
		return "", &result, err
	case approval.StatusApproved:
		return record.ApprovalToken, nil, nil
	case approval.StatusDenied:
		result, err := r.blockTask(task, firstNonEmpty(record.DeniedReason, "root approval denied"))
		return "", &result, err
	case approval.StatusConsumed:
		result, err := r.blockTask(task, "root approval already consumed; request a new approval")
		return "", &result, err
	default:
		result, err := r.blockTask(task, "root approval is not usable")
		return "", &result, err
	}
}

func (r *Runner) awaitApproval(task airuntime.Task, record approval.Request) (RunResult, error) {
	updated, err := r.runtimeStore.MoveTask(task.TaskID, airuntime.TaskStateAwaitingApproval, func(t *airuntime.Task) {
		ensureMetadata(t)
		delete(t.Metadata, "approval_token")
		t.Metadata["root_approval_id"] = record.ApprovalID
		t.NextAction = "await root approval"
		t.FailureReason = ""
	})
	if err != nil {
		return RunResult{}, err
	}
	if err := r.clearActiveTask(updated.TaskID); err != nil {
		return RunResult{}, err
	}
	obsPath, err := r.writeObservation("os", task.TaskID+"-root-approval.json", map[string]any{
		"type":           "root-approval-request",
		"task_id":        task.TaskID,
		"approval_id":    record.ApprovalID,
		"execution":      record.ExecutionProfile,
		"action_kind":    record.ActionKind,
		"summary":        record.Summary,
		"status":         record.Status,
		"requested_by":   record.RequestedBy,
		"requested_at":   record.CreatedAt.Format(time.RFC3339),
		"requires_human": true,
	})
	if err != nil {
		return RunResult{}, err
	}
	return RunResult{Task: updated, ObservationPaths: []string{obsPath}}, nil
}

func upsertCard(store *memory.Store, cardID string, req memory.CreateCardRequest) error {
	if _, err := store.CreateCard(req); err == nil {
		return nil
	} else if !errors.Is(err, memory.ErrAlreadyExists) {
		return err
	}

	status := req.Status
	if status == "" {
		status = memory.CardStatusActive
	}
	_, err := store.UpdateCard(cardID, memory.UpdateCardRequest{
		Content:      req.Content,
		Scope:        req.Scope,
		EvidenceRefs: req.EvidenceRefs,
		Provenance:   req.Provenance,
		Status:       status,
		Supersedes:   req.Supersedes,
	})
	return err
}

func searchMemoryCardID(taskID string) string {
	return "search:" + taskID
}

func emailMemoryCardID(taskID string) string {
	return "email:" + taskID
}

func emailSummaryCardID(taskID string) string {
	return "email:" + taskID + ":summary"
}

func emailSummaryEdgeID(taskID string) string {
	return "edge:email:" + taskID + ":summary"
}

func githubIssueMemoryCardID(taskID string) string {
	return "github:" + taskID
}

func githubIssueSummaryCardID(taskID string) string {
	return "github:" + taskID + ":summary"
}

func githubIssueSummaryEdgeID(taskID string) string {
	return "edge:github:" + taskID + ":summary"
}

func githubIssueEdgeID(taskID string, index int) string {
	return fmt.Sprintf("edge:github:%s:issue:%d", taskID, index+1)
}

func emailThreadEdgeID(taskID, threadKey string) string {
	sum := sha1.Sum([]byte(threadKey))
	return fmt.Sprintf("edge:email:%s:thread:%x", taskID, sum[:6])
}

func emailMessageEdgeID(taskID string, index int) string {
	return fmt.Sprintf("edge:email:%s:message:%d", taskID, index+1)
}

func emailThreadMessageEdgeID(taskID string, index int) string {
	return fmt.Sprintf("edge:email:%s:thread-message:%d", taskID, index+1)
}

func searchSummaryCardID(taskID string) string {
	return "search:" + taskID + ":summary"
}

func searchSummaryEdgeID(taskID string) string {
	return "edge:search:" + taskID + ":summary"
}

func canonicalSearchResultCardID(rawURL string) string {
	key := canonicalURL(rawURL)
	sum := sha1.Sum([]byte(key))
	return fmt.Sprintf("web_result:%x", sum[:8])
}

func canonicalEmailMessageCardID(msg connectors.EmailMessage) string {
	key := strings.TrimSpace(firstNonEmpty(msg.MessageID, msg.Path, msg.Subject+"|"+msg.Date))
	sum := sha1.Sum([]byte(key))
	return fmt.Sprintf("email_message:%x", sum[:8])
}

func canonicalEmailThreadCardID(threadKey string) string {
	sum := sha1.Sum([]byte(threadKey))
	return fmt.Sprintf("email_thread:%x", sum[:8])
}

func canonicalGitHubIssueCardID(issue connectors.GitHubIssue) string {
	key := strings.TrimSpace(firstNonEmpty(issue.Repo+"#"+fmt.Sprintf("%d", issue.Number), issue.URL, issue.Title))
	sum := sha1.Sum([]byte(key))
	return fmt.Sprintf("github_issue:%x", sum[:8])
}

func canonicalEmailThreadKey(msg connectors.EmailMessage) string {
	subject := normalizeEmailSubject(msg.Subject)
	if subject != "" {
		return subject
	}
	return strings.TrimSpace(firstNonEmpty(msg.MessageID, msg.Path, msg.Date))
}

func searchResultEdgeID(taskID string, index int) string {
	return fmt.Sprintf("edge:search:%s:result:%d", taskID, index+1)
}

func canonicalURL(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return strings.TrimSpace(raw)
	}
	parsed.Fragment = ""
	parsed.Host = strings.ToLower(parsed.Host)
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	return parsed.String()
}

func (r *Runner) summarizeSearchResults(task airuntime.Task, resp connectors.SearchResponse) string {
	fallback := heuristicSearchSummary(resp)
	if r.model == nil {
		return fallback
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	respModel, err := r.model.GenerateText(ctx, model.TextRequest{
		SystemPrompt: "You are the search summarizer of an AgentOS. Produce a short, factual summary of the strongest web search findings.",
		UserPrompt: fmt.Sprintf("Task: %s\nGoal: %s\nQuery: %s\nResults:\n%s",
			task.Title,
			task.Goal,
			resp.Query,
			searchResultsPrompt(resp.Results),
		),
		MaxTokens:   180,
		Temperature: 0.1,
		Profile:     model.ProfileSkills,
	})
	if err != nil || strings.TrimSpace(respModel.Text) == "" {
		return fallback
	}
	return strings.TrimSpace(respModel.Text)
}

func (r *Runner) summarizeEmailResults(task airuntime.Task, resp connectors.EmailResponse) string {
	fallback := heuristicEmailSummary(resp)
	if r.model == nil {
		return fallback
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	respModel, err := r.model.GenerateText(ctx, model.TextRequest{
		SystemPrompt: "You are the email summarizer of an AgentOS. Produce a short factual summary of the most relevant inbox messages.",
		UserPrompt: fmt.Sprintf("Task: %s\nGoal: %s\nQuery: %s\nMessages:\n%s",
			task.Title,
			task.Goal,
			resp.Query,
			emailResultsPrompt(resp.Results),
		),
		MaxTokens:   180,
		Temperature: 0.1,
		Profile:     model.ProfileSkills,
	})
	if err != nil || strings.TrimSpace(respModel.Text) == "" {
		return fallback
	}
	return strings.TrimSpace(respModel.Text)
}

func (r *Runner) summarizeGitHubIssues(task airuntime.Task, resp connectors.GitHubIssueResponse) string {
	fallback := heuristicGitHubIssueSummary(resp)
	if r.model == nil {
		return fallback
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	respModel, err := r.model.GenerateText(ctx, model.TextRequest{
		SystemPrompt: "You are the GitHub issue summarizer of an AgentOS. Produce a short factual summary of the strongest issue findings.",
		UserPrompt: fmt.Sprintf("Task: %s\nGoal: %s\nQuery: %s\nIssues:\n%s",
			task.Title,
			task.Goal,
			resp.Query,
			githubIssueResultsPrompt(resp.Results),
		),
		MaxTokens:   180,
		Temperature: 0.1,
		Profile:     model.ProfileSkills,
	})
	if err != nil || strings.TrimSpace(respModel.Text) == "" {
		return fallback
	}
	return strings.TrimSpace(respModel.Text)
}

func heuristicSearchSummary(resp connectors.SearchResponse) string {
	if len(resp.Results) == 0 {
		return "No results returned."
	}
	parts := make([]string, 0, min(3, len(resp.Results)))
	for i, result := range resp.Results {
		if i >= 3 {
			break
		}
		parts = append(parts, fmt.Sprintf("%d. %s - %s", i+1, firstNonEmpty(result.Title, result.URL), previewText(firstNonEmpty(result.Snippet, result.URL), 140)))
	}
	return strings.Join(parts, "\n")
}

func heuristicEmailSummary(resp connectors.EmailResponse) string {
	if len(resp.Results) == 0 {
		return "No messages returned."
	}
	parts := make([]string, 0, min(3, len(resp.Results)))
	for i, msg := range resp.Results {
		if i >= 3 {
			break
		}
		state := "read"
		if msg.Unread {
			state = "unread"
		}
		parts = append(parts, fmt.Sprintf("%d. %s [%s] from %s - %s",
			i+1,
			firstNonEmpty(msg.Subject, "(no subject)"),
			state,
			firstNonEmpty(msg.From, "unknown"),
			previewText(firstNonEmpty(msg.Snippet, msg.Path), 140),
		))
	}
	return strings.Join(parts, "\n")
}

func heuristicGitHubIssueSummary(resp connectors.GitHubIssueResponse) string {
	if len(resp.Results) == 0 {
		return "No issues returned."
	}
	parts := make([]string, 0, min(3, len(resp.Results)))
	for i, issue := range resp.Results {
		if i >= 3 {
			break
		}
		parts = append(parts, fmt.Sprintf("%d. #%d %s (%s) - %s",
			i+1,
			issue.Number,
			firstNonEmpty(issue.Title, issue.URL),
			firstNonEmpty(issue.State, "unknown"),
			previewText(firstNonEmpty(issue.Body, issue.URL), 140),
		))
	}
	return strings.Join(parts, "\n")
}

type emailThreadGroup struct {
	key          string
	subject      string
	messages     []connectors.EmailMessage
	unreadCount  int
	latestDate   string
	participants []string
}

func (g emailThreadGroup) summarySnippet() string {
	for _, msg := range g.messages {
		if strings.TrimSpace(msg.Snippet) != "" {
			return msg.Snippet
		}
	}
	return ""
}

func groupEmailThreads(messages []connectors.EmailMessage) []emailThreadGroup {
	groupMap := make(map[string]*emailThreadGroup)
	order := make([]string, 0)
	for _, msg := range messages {
		key := canonicalEmailThreadKey(msg)
		if _, ok := groupMap[key]; !ok {
			groupMap[key] = &emailThreadGroup{
				key:      key,
				subject:  normalizeEmailSubject(msg.Subject),
				messages: make([]connectors.EmailMessage, 0),
			}
			order = append(order, key)
		}
		group := groupMap[key]
		group.messages = append(group.messages, msg)
		if msg.Unread {
			group.unreadCount++
		}
		if msg.Date > group.latestDate {
			group.latestDate = msg.Date
		}
		if sender := strings.TrimSpace(msg.From); sender != "" && !containsString(group.participants, sender) {
			group.participants = append(group.participants, sender)
		}
		if group.subject == "" {
			group.subject = normalizeEmailSubject(msg.Subject)
		}
	}

	out := make([]emailThreadGroup, 0, len(order))
	for _, key := range order {
		out = append(out, *groupMap[key])
	}
	return out
}

func normalizeEmailSubject(subject string) string {
	subject = strings.TrimSpace(strings.ToLower(subject))
	for {
		switch {
		case strings.HasPrefix(subject, "re:"):
			subject = strings.TrimSpace(strings.TrimPrefix(subject, "re:"))
		case strings.HasPrefix(subject, "fw:"):
			subject = strings.TrimSpace(strings.TrimPrefix(subject, "fw:"))
		case strings.HasPrefix(subject, "fwd:"):
			subject = strings.TrimSpace(strings.TrimPrefix(subject, "fwd:"))
		default:
			return subject
		}
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func searchResultsPrompt(results []connectors.SearchResult) string {
	var b strings.Builder
	for i, result := range results {
		if i >= 5 {
			break
		}
		fmt.Fprintf(&b, "%d. %s\nURL: %s\nSnippet: %s\n\n", i+1, result.Title, result.URL, previewText(result.Snippet, 240))
	}
	return b.String()
}

func emailResultsPrompt(results []connectors.EmailMessage) string {
	var b strings.Builder
	for i, msg := range results {
		if i >= 5 {
			break
		}
		state := "read"
		if msg.Unread {
			state = "unread"
		}
		fmt.Fprintf(&b, "%d. Subject: %s\nFrom: %s\nState: %s\nDate: %s\nSnippet: %s\n\n",
			i+1,
			firstNonEmpty(msg.Subject, "(no subject)"),
			firstNonEmpty(msg.From, "unknown"),
			state,
			msg.Date,
			previewText(msg.Snippet, 240),
		)
	}
	return b.String()
}

func githubIssueResultsPrompt(results []connectors.GitHubIssue) string {
	var b strings.Builder
	for i, issue := range results {
		if i >= 5 {
			break
		}
		fmt.Fprintf(&b, "%d. Repo: %s\nNumber: %d\nTitle: %s\nState: %s\nURL: %s\nBody: %s\n\n",
			i+1,
			issue.Repo,
			issue.Number,
			firstNonEmpty(issue.Title, "(no title)"),
			firstNonEmpty(issue.State, "unknown"),
			issue.URL,
			previewText(issue.Body, 240),
		)
	}
	return b.String()
}

func min(left, right int) int {
	if left < right {
		return left
	}
	return right
}

func splitArgs(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	args := make([]string, 0)
	var current strings.Builder
	var quote rune
	escaped := false
	flush := func() {
		if current.Len() == 0 {
			return
		}
		args = append(args, current.String())
		current.Reset()
	}
	for _, r := range raw {
		switch {
		case escaped:
			if r == '\\' || r == '"' || r == '\'' || r == ' ' || r == '\t' || r == '\n' {
				current.WriteRune(r)
			} else {
				current.WriteRune('\\')
				current.WriteRune(r)
			}
			escaped = false
		case r == '\\' && quote == '\'':
			current.WriteRune(r)
		case r == '\\':
			escaped = true
		case quote != 0:
			if r == quote {
				quote = 0
				continue
			}
			current.WriteRune(r)
		case r == '\'' || r == '"':
			quote = r
		case r == ' ' || r == '\t' || r == '\n':
			flush()
		default:
			current.WriteRune(r)
		}
	}
	flush()
	return args
}

func (r *Runner) generateTaskPlan(task airuntime.Task, now time.Time) (string, model.TextResponse, error) {
	fallback := fmt.Sprintf(
		"# Task Plan\n\nTask ID: %s\nTitle: %s\nGoal: %s\nSelected Skill: %s\nGenerated At: %s\n\nNext Step: execute via runtime action pipeline.\n",
		task.TaskID,
		task.Title,
		task.Goal,
		firstNonEmpty(task.SelectedSkill, "task-plan"),
		now.Format(time.RFC3339),
	)
	if r.model == nil {
		return fallback, model.TextResponse{}, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	resp, err := r.model.GenerateText(ctx, model.TextRequest{
		SystemPrompt: "You are the planning module of an AgentOS. Write concise, practical markdown plans. Do not tell the user to approve, sign off, or wait for review of this plan as a separate workflow step — the plan is informational; execution is handled by the runtime.",
		UserPrompt: fmt.Sprintf(
			"Create a short markdown task plan.\nTask ID: %s\nTitle: %s\nGoal: %s\nSelected Skill: %s\nTime: %s",
			task.TaskID,
			task.Title,
			task.Goal,
			firstNonEmpty(task.SelectedSkill, "task-plan"),
			now.Format(time.RFC3339),
		),
		MaxTokens:   400,
		Temperature: 0.2,
		Profile:     model.ProfileSkills,
	})
	if err != nil || strings.TrimSpace(resp.Text) == "" {
		return fallback, resp, err
	}
	return resp.Text + "\n", resp, nil
}

func (r *Runner) generateMemorySummary(task airuntime.Task, now time.Time) (string, model.TextResponse, error) {
	fallback := fmt.Sprintf(
		"# Memory Consolidation\n\nTask: %s\nGoal: %s\nGenerated At: %s\n\nSummary: memory consolidation placeholder artifact.\n",
		task.Title,
		task.Goal,
		now.Format(time.RFC3339),
	)
	if r.model == nil {
		return fallback, model.TextResponse{}, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	resp, err := r.model.GenerateText(ctx, model.TextRequest{
		SystemPrompt: "You are the memory steward of an AgentOS. Summarize durable facts, context, and next-useful memory in concise markdown.",
		UserPrompt: fmt.Sprintf(
			"Write a markdown memory consolidation note.\nTask: %s\nGoal: %s\nTime: %s",
			task.Title,
			task.Goal,
			now.Format(time.RFC3339),
		),
		MaxTokens:   400,
		Temperature: 0.2,
		Profile:     model.ProfileSkills,
	})
	if err != nil || strings.TrimSpace(resp.Text) == "" {
		return fallback, resp, err
	}
	return resp.Text + "\n", resp, nil
}

func previewText(input string, max int) string {
	if len(input) <= max {
		return input
	}
	if max <= 3 {
		return input[:max]
	}
	return input[:max-3] + "..."
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
