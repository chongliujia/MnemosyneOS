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
	"strings"
	"time"

	"mnemosyneos/internal/airuntime"
	"mnemosyneos/internal/approval"
	"mnemosyneos/internal/connectors"
	"mnemosyneos/internal/execution"
	"mnemosyneos/internal/memory"
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
	executor     *execution.Executor
	connectors   *connectors.Runtime
	approvals    *approval.Store
	model        model.TextGateway
}

func NewRunner(runtimeStore *airuntime.Store, memoryStore *memory.Store, executor *execution.Executor, connectorRuntime *connectors.Runtime, approvalStore *approval.Store, textModel model.TextGateway) *Runner {
	return &Runner{
		runtimeStore: runtimeStore,
		memoryStore:  memoryStore,
		consolidator: memory.NewConsolidator(memoryStore),
		executor:     executor,
		connectors:   connectorRuntime,
		approvals:    approvalStore,
		model:        textModel,
	}
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

	switch task.SelectedSkill {
	case "task-plan":
		return r.runTaskPlan(task, onProgress)
	case "file-edit":
		return r.runFileEdit(task, onProgress)
	case "file-read":
		return r.runFileRead(task, onProgress)
	case "shell-command":
		return r.runShellCommand(task, onProgress)
	case "web-search":
		return r.runWebSearch(task, onProgress)
	case "github-issue-search":
		return r.runGitHubIssueSearch(task, onProgress)
	case "email-inbox":
		return r.runEmailInbox(task, onProgress)
	case "memory-consolidate":
		return r.runMemoryConsolidate(task, onProgress)
	default:
		return r.runTaskPlan(task, onProgress)
	}
}

func (r *Runner) runTaskPlan(task airuntime.Task, onProgress func(ProgressEvent)) (RunResult, error) {
	emitProgress(onProgress, "planning.generate", "Generating the task plan...")
	now := time.Now().UTC()
	body, modelResp, modelErr := r.generateTaskPlan(task, now)
	emitProgress(onProgress, "planning.persist", "Writing the plan artifact and observation...")
	artifactPath, err := r.writeArtifact("reports", task.TaskID+"-plan.md", body)
	if err != nil {
		return RunResult{}, err
	}
	obsPath, err := r.writeObservation("os", task.TaskID+"-plan.json", map[string]any{
		"type":           "task-plan",
		"task_id":        task.TaskID,
		"selected_skill": firstNonEmpty(task.SelectedSkill, "task-plan"),
		"generated_at":   now.Format(time.RFC3339),
		"summary":        "plan artifact generated",
		"model_provider": modelResp.Provider,
		"model_name":     modelResp.Model,
		"model_error":    errorString(modelErr),
	})
	if err != nil {
		return RunResult{}, err
	}

	updated, err := r.runtimeStore.MoveTask(task.TaskID, airuntime.TaskStateDone, func(t *airuntime.Task) {
		ensureMetadata(t)
		t.NextAction = "plan generated"
		t.Metadata["plan_artifact"] = artifactPath
	})
	if err != nil {
		return RunResult{}, err
	}
	if err := r.clearActiveTask(updated.TaskID); err != nil {
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
		if err := r.clearActiveTask(updated.TaskID); err != nil {
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
		if err := r.clearActiveTask(updated.TaskID); err != nil {
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
		if err := r.clearActiveTask(updated.TaskID); err != nil {
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
	if err := r.clearActiveTask(updated.TaskID); err != nil {
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
	if err := r.clearActiveTask(updated.TaskID); err != nil {
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
	if err := r.clearActiveTask(updated.TaskID); err != nil {
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
	if err := r.clearActiveTask(updated.TaskID); err != nil {
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
	emitProgress(onProgress, "persist.artifacts", "Writing the memory artifact and observation...")
	artifactPath, err := r.writeArtifact("reports", task.TaskID+"-memory.md", body)
	if err != nil {
		return RunResult{}, err
	}
	obsPath, err := r.writeObservation("os", task.TaskID+"-memory.json", map[string]any{
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
	})
	if err != nil {
		return RunResult{}, err
	}
	updated, err := r.runtimeStore.MoveTask(task.TaskID, airuntime.TaskStateDone, func(t *airuntime.Task) {
		ensureMetadata(t)
		t.NextAction = "memory consolidation recorded"
		t.Metadata["memory_artifact"] = artifactPath
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
		Tasks:         tasks,
		TaskClass:     strings.TrimSpace(task.Metadata["task_class"]),
		SelectedSkill: strings.TrimSpace(task.Metadata["selected_skill"]),
		Scope:         firstNonEmpty(strings.TrimSpace(task.Metadata["scope"]), memoryScopeProject),
		MinRuns:       minRuns,
	})
	for _, candidate := range candidates {
		if _, err := r.memoryStore.CreateCard(candidate); err != nil && !errors.Is(err, memory.ErrAlreadyExists) {
			return err
		}
	}
	return nil
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
	return strings.Fields(raw)
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
		SystemPrompt: "You are the planning module of an AgentOS. Write concise, practical markdown plans.",
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
