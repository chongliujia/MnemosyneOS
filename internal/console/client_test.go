package console

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"mnemosyneos/internal/airuntime"
	"mnemosyneos/internal/api"
	"mnemosyneos/internal/approval"
	"mnemosyneos/internal/chat"
	"mnemosyneos/internal/execution"
	"mnemosyneos/internal/memory"
	"mnemosyneos/internal/model"
	"mnemosyneos/internal/recall"
	"mnemosyneos/internal/skills"
)

func TestClientTaskFlow(t *testing.T) {
	runtimeRoot := tempConsoleRuntimeRoot(t)
	workspaceRoot := t.TempDir()

	runtimeStore := airuntime.NewStore(runtimeRoot)
	orchestrator := airuntime.NewOrchestrator(runtimeStore)
	approvalStore := approval.NewStore(runtimeRoot, 10*time.Minute)
	actionStore := execution.NewStore(runtimeRoot)
	executor, err := execution.NewExecutor(actionStore, workspaceRoot)
	if err != nil {
		t.Fatalf("NewExecutor returned error: %v", err)
	}
	memoryStore := memory.NewStore()
	modelConfig, err := model.NewConfigStore(runtimeRoot)
	if err != nil {
		t.Fatalf("NewConfigStore returned error: %v", err)
	}
	if err := modelConfig.Save(model.Config{Provider: "none"}); err != nil {
		t.Fatalf("Save model config returned error: %v", err)
	}
	skillRunner := skills.NewRunner(runtimeStore, memoryStore, executor, nil, approvalStore, nil)
	recallService := recall.NewService(memoryStore)
	chatService := chat.NewService(chat.NewStore(runtimeRoot), orchestrator, runtimeStore, recallService, skillRunner, nil)
	handler := api.NewServer(memoryStore, runtimeStore, approvalStore, chatService, recallService, orchestrator, executor, skillRunner, modelConfig).Routes()

	client := NewClient("http://mnemosyne.test")
	client.httpClient = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			return rec.Result(), nil
		}),
	}

	state, err := client.RuntimeState()
	if err != nil {
		t.Fatalf("RuntimeState returned error: %v", err)
	}
	if state.Status != "idle" {
		t.Fatalf("expected idle runtime, got %s", state.Status)
	}

	task, err := client.CreateTask(airuntime.CreateTaskRequest{
		Title:       "Plan work",
		Goal:        "Plan the next repository step",
		RequestedBy: "test",
		Source:      "console-test",
	})
	if err != nil {
		t.Fatalf("CreateTask returned error: %v", err)
	}
	if task.State != airuntime.TaskStateActive {
		t.Fatalf("expected active task, got %s", task.State)
	}

	tasks, err := client.ListTasks()
	if err != nil {
		t.Fatalf("ListTasks returned error: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected one task, got %d", len(tasks))
	}

	result, err := client.RunTask(task.TaskID)
	if err != nil {
		t.Fatalf("RunTask returned error: %v", err)
	}
	if result.Task.State != airuntime.TaskStateDone {
		t.Fatalf("expected done task, got %s", result.Task.State)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func tempConsoleRuntimeRoot(t *testing.T) string {
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
