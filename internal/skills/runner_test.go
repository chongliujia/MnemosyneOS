package skills

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mnemosyneos/internal/airuntime"
	"mnemosyneos/internal/approval"
	"mnemosyneos/internal/connectors"
	"mnemosyneos/internal/execution"
	"mnemosyneos/internal/memory"
	"mnemosyneos/internal/model"
)

func TestRunTaskPlanCompletesTask(t *testing.T) {
	runtimeRoot := tempSkillRuntimeRoot(t)
	workspaceRoot := t.TempDir()

	runtimeStore := airuntime.NewStore(runtimeRoot)
	execStore := execution.NewStore(runtimeRoot)
	memoryStore := memory.NewStore()
	executor, err := execution.NewExecutor(execStore, workspaceRoot)
	if err != nil {
		t.Fatalf("NewExecutor returned error: %v", err)
	}
	runner := NewRunner(runtimeStore, memoryStore, executor, nil, nil, nil)
	orch := airuntime.NewOrchestrator(runtimeStore)

	task, err := orch.SubmitTask(airuntime.CreateTaskRequest{
		Title: "Plan next step",
		Goal:  "Plan the repository work",
	})
	if err != nil {
		t.Fatalf("SubmitTask returned error: %v", err)
	}

	result, err := runner.RunTask(task.TaskID)
	if err != nil {
		t.Fatalf("RunTask returned error: %v", err)
	}
	if result.Task.State != airuntime.TaskStateDone {
		t.Fatalf("expected done task, got %s", result.Task.State)
	}
	if len(result.ArtifactPaths) != 1 {
		t.Fatalf("expected one artifact, got %d", len(result.ArtifactPaths))
	}
}

func TestRunWebSearchCompletesTask(t *testing.T) {
	runtimeRoot := tempSkillRuntimeRoot(t)
	workspaceRoot := t.TempDir()

	runtimeStore := airuntime.NewStore(runtimeRoot)
	execStore := execution.NewStore(runtimeRoot)
	memoryStore := memory.NewStore()
	executor, err := execution.NewExecutor(execStore, workspaceRoot)
	if err != nil {
		t.Fatalf("NewExecutor returned error: %v", err)
	}
	runner := NewRunner(runtimeStore, memoryStore, executor, connectors.NewRuntime(fakeSearchClient{
		resp: connectors.SearchResponse{
			Query:    "Search the web for docs",
			Provider: "fake-search",
			Results: []connectors.SearchResult{
				{Title: "Docs", URL: "https://example.com/a", Snippet: "Alpha"},
			},
		},
	}, fakeGitHubClient{}, fakeEmailClient{}), nil, nil)
	orch := airuntime.NewOrchestrator(runtimeStore)

	task, err := orch.SubmitTask(airuntime.CreateTaskRequest{
		Title: "Search docs",
		Goal:  "Search the web for docs",
	})
	if err != nil {
		t.Fatalf("SubmitTask returned error: %v", err)
	}

	result, err := runner.RunTask(task.TaskID)
	if err != nil {
		t.Fatalf("RunTask returned error: %v", err)
	}
	if result.Task.State != airuntime.TaskStateDone {
		t.Fatalf("expected done task, got %s", result.Task.State)
	}
	if len(result.ObservationPaths) != 1 || len(result.ArtifactPaths) != 1 {
		t.Fatalf("expected one observation and one artifact, got obs=%d artifacts=%d", len(result.ObservationPaths), len(result.ArtifactPaths))
	}
	data, err := os.ReadFile(result.ArtifactPaths[0])
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	if !strings.Contains(string(data), "fake-search") {
		t.Fatalf("expected provider in artifact, got %q", string(data))
	}

	rootCard := memoryStore.Query(memory.QueryRequest{CardID: searchMemoryCardID(task.TaskID)})
	if len(rootCard.Cards) != 1 {
		t.Fatalf("expected one root memory card, got %d", len(rootCard.Cards))
	}
	if got := rootCard.Cards[0].Content["provider"]; got != "fake-search" {
		t.Fatalf("unexpected memory provider content: %#v", got)
	}
	if len(rootCard.Edges) != 2 {
		t.Fatalf("expected summary edge plus one search result edge, got %d", len(rootCard.Edges))
	}

	summaryCard := memoryStore.Query(memory.QueryRequest{CardID: searchSummaryCardID(task.TaskID)})
	if len(summaryCard.Cards) != 1 {
		t.Fatalf("expected one summary card, got %d", len(summaryCard.Cards))
	}

	resultCard := memoryStore.Query(memory.QueryRequest{CardID: canonicalSearchResultCardID("https://example.com/a")})
	if len(resultCard.Cards) != 1 {
		t.Fatalf("expected one canonical result card, got %d", len(resultCard.Cards))
	}
}

func TestRunWebSearchBlocksWhenSearchClientMissing(t *testing.T) {
	runtimeRoot := tempSkillRuntimeRoot(t)
	workspaceRoot := t.TempDir()

	runtimeStore := airuntime.NewStore(runtimeRoot)
	execStore := execution.NewStore(runtimeRoot)
	memoryStore := memory.NewStore()
	executor, err := execution.NewExecutor(execStore, workspaceRoot)
	if err != nil {
		t.Fatalf("NewExecutor returned error: %v", err)
	}
	runner := NewRunner(runtimeStore, memoryStore, executor, nil, nil, nil)
	orch := airuntime.NewOrchestrator(runtimeStore)

	task, err := orch.SubmitTask(airuntime.CreateTaskRequest{
		Title: "Search docs",
		Goal:  "Search the web for docs",
	})
	if err != nil {
		t.Fatalf("SubmitTask returned error: %v", err)
	}

	result, err := runner.RunTask(task.TaskID)
	if err != nil {
		t.Fatalf("RunTask returned error: %v", err)
	}
	if result.Task.State != airuntime.TaskStateBlocked {
		t.Fatalf("expected blocked task, got %s", result.Task.State)
	}
	if len(result.ObservationPaths) != 1 {
		t.Fatalf("expected one observation, got %d", len(result.ObservationPaths))
	}
}

func TestRunFileEditWritesFile(t *testing.T) {
	runtimeRoot := tempSkillRuntimeRoot(t)
	workspaceRoot := t.TempDir()

	runtimeStore := airuntime.NewStore(runtimeRoot)
	execStore := execution.NewStore(runtimeRoot)
	memoryStore := memory.NewStore()
	executor, err := execution.NewExecutor(execStore, workspaceRoot)
	if err != nil {
		t.Fatalf("NewExecutor returned error: %v", err)
	}
	runner := NewRunner(runtimeStore, memoryStore, executor, nil, nil, nil)
	orch := airuntime.NewOrchestrator(runtimeStore)

	task, err := orch.SubmitTask(airuntime.CreateTaskRequest{
		Title: "Edit a file",
		Goal:  "Edit a file in the workspace",
		Metadata: map[string]string{
			"path":    "notes/todo.txt",
			"content": "ship runtime MVP",
		},
	})
	if err != nil {
		t.Fatalf("SubmitTask returned error: %v", err)
	}

	result, err := runner.RunTask(task.TaskID)
	if err != nil {
		t.Fatalf("RunTask returned error: %v", err)
	}
	if result.Task.State != airuntime.TaskStateDone {
		t.Fatalf("expected done task, got %s", result.Task.State)
	}
	if result.Action == nil || result.Action.Status != execution.ActionStatusCompleted {
		t.Fatalf("expected completed action, got %#v", result.Action)
	}
}

func TestRunRootFileEditRequestsApproval(t *testing.T) {
	runtimeRoot := tempSkillRuntimeRoot(t)
	workspaceRoot := t.TempDir()

	runtimeStore := airuntime.NewStore(runtimeRoot)
	execStore := execution.NewStore(runtimeRoot)
	memoryStore := memory.NewStore()
	executor, err := execution.NewExecutor(execStore, workspaceRoot)
	if err != nil {
		t.Fatalf("NewExecutor returned error: %v", err)
	}
	approvalStore := approval.NewStore(runtimeRoot, 10*time.Minute)
	runner := NewRunner(runtimeStore, memoryStore, executor, nil, approvalStore, nil)
	orch := airuntime.NewOrchestrator(runtimeStore)

	task, err := orch.SubmitTask(airuntime.CreateTaskRequest{
		Title:            "Edit a root file",
		Goal:             "Edit a root-owned file",
		ExecutionProfile: "root",
		Metadata: map[string]string{
			"path":    "notes/root.txt",
			"content": "needs approval",
		},
	})
	if err != nil {
		t.Fatalf("SubmitTask returned error: %v", err)
	}

	result, err := runner.RunTask(task.TaskID)
	if err != nil {
		t.Fatalf("RunTask returned error: %v", err)
	}
	if result.Task.State != airuntime.TaskStateAwaitingApproval {
		t.Fatalf("expected awaiting approval task, got %s", result.Task.State)
	}
	approvals, err := approvalStore.List(approval.StatusPending)
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(approvals) != 1 {
		t.Fatalf("expected one pending approval, got %d", len(approvals))
	}
}

func TestRunTaskPlanUsesModelWhenAvailable(t *testing.T) {
	runtimeRoot := tempSkillRuntimeRoot(t)
	workspaceRoot := t.TempDir()

	runtimeStore := airuntime.NewStore(runtimeRoot)
	execStore := execution.NewStore(runtimeRoot)
	memoryStore := memory.NewStore()
	executor, err := execution.NewExecutor(execStore, workspaceRoot)
	if err != nil {
		t.Fatalf("NewExecutor returned error: %v", err)
	}
	runner := NewRunner(runtimeStore, memoryStore, executor, nil, nil, fakeTextModel{
		resp: model.TextResponse{
			Provider: "fake",
			Model:    "fake-model",
			Text:     "# Task Plan\n\nModel-generated plan.\n",
		},
	})
	orch := airuntime.NewOrchestrator(runtimeStore)

	task, err := orch.SubmitTask(airuntime.CreateTaskRequest{
		Title: "Plan next step with model",
		Goal:  "Plan with actual model output",
	})
	if err != nil {
		t.Fatalf("SubmitTask returned error: %v", err)
	}

	result, err := runner.RunTask(task.TaskID)
	if err != nil {
		t.Fatalf("RunTask returned error: %v", err)
	}
	data, err := os.ReadFile(result.ArtifactPaths[0])
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	if !strings.Contains(string(data), "Model-generated plan.") {
		t.Fatalf("expected model-generated artifact, got %q", string(data))
	}
}

func TestRunRootFileReadRequestsApproval(t *testing.T) {
	runtimeRoot := tempSkillRuntimeRoot(t)
	workspaceRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspaceRoot, "notes.txt"), []byte("read me"), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	runtimeStore := airuntime.NewStore(runtimeRoot)
	execStore := execution.NewStore(runtimeRoot)
	memoryStore := memory.NewStore()
	executor, err := execution.NewExecutor(execStore, workspaceRoot)
	if err != nil {
		t.Fatalf("NewExecutor returned error: %v", err)
	}
	approvalStore := approval.NewStore(runtimeRoot, 10*time.Minute)
	runner := NewRunner(runtimeStore, memoryStore, executor, nil, approvalStore, nil)
	orch := airuntime.NewOrchestrator(runtimeStore)

	task, err := orch.SubmitTask(airuntime.CreateTaskRequest{
		Title:            "Read a root file",
		Goal:             "Read a file in the workspace",
		ExecutionProfile: "root",
		Metadata: map[string]string{
			"path": "notes.txt",
		},
	})
	if err != nil {
		t.Fatalf("SubmitTask returned error: %v", err)
	}

	result, err := runner.RunTask(task.TaskID)
	if err != nil {
		t.Fatalf("RunTask returned error: %v", err)
	}
	if result.Task.State != airuntime.TaskStateAwaitingApproval {
		t.Fatalf("expected awaiting approval task, got %s", result.Task.State)
	}
}

func TestRunRootShellRequestsApproval(t *testing.T) {
	runtimeRoot := tempSkillRuntimeRoot(t)
	workspaceRoot := t.TempDir()

	runtimeStore := airuntime.NewStore(runtimeRoot)
	execStore := execution.NewStore(runtimeRoot)
	memoryStore := memory.NewStore()
	executor, err := execution.NewExecutor(execStore, workspaceRoot)
	if err != nil {
		t.Fatalf("NewExecutor returned error: %v", err)
	}
	approvalStore := approval.NewStore(runtimeRoot, 10*time.Minute)
	runner := NewRunner(runtimeStore, memoryStore, executor, nil, approvalStore, nil)
	orch := airuntime.NewOrchestrator(runtimeStore)

	task, err := orch.SubmitTask(airuntime.CreateTaskRequest{
		Title:            "Run a root shell command",
		Goal:             "Run a shell command in the workspace",
		ExecutionProfile: "root",
		Metadata: map[string]string{
			"command": "pwd",
			"workdir": ".",
		},
	})
	if err != nil {
		t.Fatalf("SubmitTask returned error: %v", err)
	}

	result, err := runner.RunTask(task.TaskID)
	if err != nil {
		t.Fatalf("RunTask returned error: %v", err)
	}
	if result.Task.State != airuntime.TaskStateAwaitingApproval {
		t.Fatalf("expected awaiting approval task, got %s", result.Task.State)
	}
}

func TestRunGitHubIssueSearchCompletesTask(t *testing.T) {
	runtimeRoot := tempSkillRuntimeRoot(t)
	workspaceRoot := t.TempDir()

	runtimeStore := airuntime.NewStore(runtimeRoot)
	execStore := execution.NewStore(runtimeRoot)
	memoryStore := memory.NewStore()
	executor, err := execution.NewExecutor(execStore, workspaceRoot)
	if err != nil {
		t.Fatalf("NewExecutor returned error: %v", err)
	}
	runner := NewRunner(runtimeStore, memoryStore, executor, connectors.NewRuntime(nil, fakeGitHubClient{
		resp: connectors.GitHubIssueResponse{
			Query:    "approval flow",
			Provider: "github",
			Results: []connectors.GitHubIssue{
				{Number: 12, Title: "Approval flow", URL: "https://example.com/issues/12", State: "open", Body: "Need root approval flow", Repo: "mnemosyne/agentos"},
			},
		},
	}, fakeEmailClient{}), nil, nil)
	orch := airuntime.NewOrchestrator(runtimeStore)

	task, err := orch.SubmitTask(airuntime.CreateTaskRequest{
		Title: "Search GitHub issues",
		Goal:  "Search github issues for approval flow",
	})
	if err != nil {
		t.Fatalf("SubmitTask returned error: %v", err)
	}

	result, err := runner.RunTask(task.TaskID)
	if err != nil {
		t.Fatalf("RunTask returned error: %v", err)
	}
	if result.Task.State != airuntime.TaskStateDone {
		t.Fatalf("expected done task, got %s", result.Task.State)
	}
	data, err := os.ReadFile(result.ArtifactPaths[0])
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	if !strings.Contains(string(data), "Approval flow") {
		t.Fatalf("expected github issue in artifact, got %q", string(data))
	}

	rootCard := memoryStore.Query(memory.QueryRequest{CardID: githubIssueMemoryCardID(task.TaskID)})
	if len(rootCard.Cards) != 1 {
		t.Fatalf("expected one github root card, got %d", len(rootCard.Cards))
	}
	if len(rootCard.Edges) != 2 {
		t.Fatalf("expected summary edge plus one issue edge, got %d", len(rootCard.Edges))
	}

	summaryCard := memoryStore.Query(memory.QueryRequest{CardID: githubIssueSummaryCardID(task.TaskID)})
	if len(summaryCard.Cards) != 1 {
		t.Fatalf("expected one github summary card, got %d", len(summaryCard.Cards))
	}

	issueCard := memoryStore.Query(memory.QueryRequest{CardID: canonicalGitHubIssueCardID(connectors.GitHubIssue{
		Number: 12,
		Title:  "Approval flow",
		URL:    "https://example.com/issues/12",
		State:  "open",
		Body:   "Need root approval flow",
		Repo:   "mnemosyne/agentos",
	})})
	if len(issueCard.Cards) != 1 {
		t.Fatalf("expected one canonical github issue card, got %d", len(issueCard.Cards))
	}
}

func TestRunEmailInboxCompletesTask(t *testing.T) {
	runtimeRoot := tempSkillRuntimeRoot(t)
	workspaceRoot := t.TempDir()

	runtimeStore := airuntime.NewStore(runtimeRoot)
	execStore := execution.NewStore(runtimeRoot)
	memoryStore := memory.NewStore()
	executor, err := execution.NewExecutor(execStore, workspaceRoot)
	if err != nil {
		t.Fatalf("NewExecutor returned error: %v", err)
	}
	runner := NewRunner(runtimeStore, memoryStore, executor, connectors.NewRuntime(nil, fakeGitHubClient{}, fakeEmailClient{
		resp: connectors.EmailResponse{
			Provider: "maildir",
			Results: []connectors.EmailMessage{
				{MessageID: "<msg-1>", Subject: "Root approval required", From: "agent@example.com", Snippet: "Please approve root action", Unread: true, Date: "2026-03-23T10:00:00Z"},
				{MessageID: "<msg-2>", Subject: "Re: Root approval required", From: "reviewer@example.com", Snippet: "Approved, rerun the task", Unread: false, Date: "2026-03-23T10:05:00Z"},
			},
		},
	}), nil, nil)
	orch := airuntime.NewOrchestrator(runtimeStore)

	task, err := orch.SubmitTask(airuntime.CreateTaskRequest{
		Title: "Check email inbox",
		Goal:  "Check email inbox",
	})
	if err != nil {
		t.Fatalf("SubmitTask returned error: %v", err)
	}

	result, err := runner.RunTask(task.TaskID)
	if err != nil {
		t.Fatalf("RunTask returned error: %v", err)
	}
	if result.Task.State != airuntime.TaskStateDone {
		t.Fatalf("expected done task, got %s", result.Task.State)
	}
	data, err := os.ReadFile(result.ArtifactPaths[0])
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	if !strings.Contains(string(data), "Root approval required") {
		t.Fatalf("expected email subject in artifact, got %q", string(data))
	}

	rootCard := memoryStore.Query(memory.QueryRequest{CardID: emailMemoryCardID(task.TaskID)})
	if len(rootCard.Cards) != 1 {
		t.Fatalf("expected one email root card, got %d", len(rootCard.Cards))
	}
	if got := rootCard.Cards[0].Content["thread_count"]; got != 1 {
		t.Fatalf("expected one thread, got %#v", got)
	}
	if len(rootCard.Edges) != 4 {
		t.Fatalf("expected summary edge, thread edge, and two message edges, got %d", len(rootCard.Edges))
	}

	summaryCard := memoryStore.Query(memory.QueryRequest{CardID: emailSummaryCardID(task.TaskID)})
	if len(summaryCard.Cards) != 1 {
		t.Fatalf("expected one email summary card, got %d", len(summaryCard.Cards))
	}
	threadCard := memoryStore.Query(memory.QueryRequest{CardID: canonicalEmailThreadCardID("root approval required")})
	if len(threadCard.Cards) != 1 {
		t.Fatalf("expected one email thread card, got %d", len(threadCard.Cards))
	}
	threadMessageEdges := 0
	for _, edge := range threadCard.Edges {
		if edge.EdgeType == "thread_message" {
			threadMessageEdges++
		}
	}
	if threadMessageEdges != 2 {
		t.Fatalf("expected two thread_message edges, got %d", threadMessageEdges)
	}

	messageCard := memoryStore.Query(memory.QueryRequest{CardID: canonicalEmailMessageCardID(connectors.EmailMessage{
		MessageID: "<msg-1>",
		Subject:   "Root approval required",
		From:      "agent@example.com",
		Snippet:   "Please approve root action",
		Unread:    true,
	})})
	if len(messageCard.Cards) != 1 {
		t.Fatalf("expected one canonical email message card, got %d", len(messageCard.Cards))
	}
}

type fakeTextModel struct {
	resp model.TextResponse
	err  error
}

func (f fakeTextModel) GenerateText(_ context.Context, _ model.TextRequest) (model.TextResponse, error) {
	return f.resp, f.err
}

func (f fakeTextModel) StreamText(_ context.Context, _ model.TextRequest, onDelta func(model.TextDelta) error) (model.TextResponse, error) {
	if f.err != nil {
		return model.TextResponse{}, f.err
	}
	if onDelta != nil && strings.TrimSpace(f.resp.Text) != "" {
		if err := onDelta(model.TextDelta{Text: f.resp.Text}); err != nil {
			return model.TextResponse{}, err
		}
	}
	return f.resp, nil
}

type fakeSearchClient struct {
	resp connectors.SearchResponse
	err  error
}

func (f fakeSearchClient) Search(_ context.Context, _ connectors.SearchRequest) (connectors.SearchResponse, error) {
	return f.resp, f.err
}

type fakeGitHubClient struct {
	resp connectors.GitHubIssueResponse
	err  error
}

func (f fakeGitHubClient) SearchIssues(_ context.Context, _ connectors.GitHubIssueRequest) (connectors.GitHubIssueResponse, error) {
	return f.resp, f.err
}

type fakeEmailClient struct {
	resp connectors.EmailResponse
	err  error
}

func (f fakeEmailClient) ListMessages(_ context.Context, _ connectors.EmailRequest) (connectors.EmailResponse, error) {
	return f.resp, f.err
}

func tempSkillRuntimeRoot(t *testing.T) string {
	t.Helper()

	root := t.TempDir()
	dirs := []string{
		filepath.Join(root, "state"),
		filepath.Join(root, "tasks", airuntime.TaskStateInbox),
		filepath.Join(root, "tasks", airuntime.TaskStatePlanned),
		filepath.Join(root, "tasks", airuntime.TaskStateActive),
		filepath.Join(root, "tasks", airuntime.TaskStateBlocked),
		filepath.Join(root, "tasks", airuntime.TaskStateAwaitingApproval),
		filepath.Join(root, "tasks", airuntime.TaskStateDone),
		filepath.Join(root, "tasks", airuntime.TaskStateFailed),
		filepath.Join(root, "tasks", airuntime.TaskStateArchived),
		filepath.Join(root, "approvals", approval.StatusPending),
		filepath.Join(root, "approvals", approval.StatusApproved),
		filepath.Join(root, "approvals", approval.StatusDenied),
		filepath.Join(root, "approvals", approval.StatusConsumed),
		filepath.Join(root, "actions", execution.ActionStatusPending),
		filepath.Join(root, "actions", execution.ActionStatusRunning),
		filepath.Join(root, "actions", execution.ActionStatusCompleted),
		filepath.Join(root, "actions", execution.ActionStatusFailed),
		filepath.Join(root, "artifacts", "reports"),
		filepath.Join(root, "observations", "filesystem"),
		filepath.Join(root, "observations", "os"),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("MkdirAll(%s): %v", dir, err)
		}
	}

	state := airuntime.RuntimeState{
		RuntimeID:        "test-runtime",
		ActiveUserID:     "default-user",
		Status:           "idle",
		ExecutionProfile: "user",
	}
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("Marshal returned error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "state", "runtime.json"), append(data, '\n'), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
	return root
}
