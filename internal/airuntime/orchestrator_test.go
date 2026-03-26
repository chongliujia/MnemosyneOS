package airuntime

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestSubmitTaskActivatesRunnableTask(t *testing.T) {
	root := tempRuntimeRoot(t)
	store := NewStore(root)
	orch := NewOrchestrator(store)

	task, err := orch.SubmitTask(CreateTaskRequest{
		Title: "Summarize repository",
		Goal:  "Summarize the repository and write a report",
	})
	if err != nil {
		t.Fatalf("SubmitTask returned error: %v", err)
	}
	if task.State != TaskStateActive {
		t.Fatalf("expected active task, got %s", task.State)
	}
	if task.SelectedSkill != "task-plan" {
		t.Fatalf("expected task-plan skill, got %s", task.SelectedSkill)
	}

	state, err := store.LoadState()
	if err != nil {
		t.Fatalf("LoadState returned error: %v", err)
	}
	if state.ActiveTaskID == nil || *state.ActiveTaskID != task.TaskID {
		t.Fatalf("expected active task id %s, got %#v", task.TaskID, state.ActiveTaskID)
	}

	taskPath := filepath.Join(root, "tasks", TaskStateActive, task.TaskID+".json")
	if _, err := os.Stat(taskPath); err != nil {
		t.Fatalf("expected task file at %s: %v", taskPath, err)
	}
}

func TestSubmitTaskLeavesApprovalTasksWaiting(t *testing.T) {
	root := tempRuntimeRoot(t)
	store := NewStore(root)
	orch := NewOrchestrator(store)

	task, err := orch.SubmitTask(CreateTaskRequest{
		Title:            "Install system package",
		Goal:             "Install a package using web research first",
		ExecutionProfile: "root",
		RequiresApproval: true,
	})
	if err != nil {
		t.Fatalf("SubmitTask returned error: %v", err)
	}
	if task.State != TaskStateAwaitingApproval {
		t.Fatalf("expected awaiting approval, got %s", task.State)
	}
	if task.SelectedSkill != "web-search" {
		t.Fatalf("expected web-search skill, got %s", task.SelectedSkill)
	}

	state, err := store.LoadState()
	if err != nil {
		t.Fatalf("LoadState returned error: %v", err)
	}
	if state.ActiveTaskID != nil {
		t.Fatalf("expected no active task, got %#v", state.ActiveTaskID)
	}
}

func TestRecoverMovesActiveTaskBackToPlanned(t *testing.T) {
	root := tempRuntimeRoot(t)
	store := NewStore(root)
	orch := NewOrchestrator(store)

	task, err := orch.SubmitTask(CreateTaskRequest{
		Title: "Search docs",
		Goal:  "Search the web for docs",
	})
	if err != nil {
		t.Fatalf("SubmitTask returned error: %v", err)
	}
	if task.State != TaskStateActive {
		t.Fatalf("expected active task, got %s", task.State)
	}

	if err := orch.Recover(); err != nil {
		t.Fatalf("Recover returned error: %v", err)
	}

	recovered, err := store.GetTask(task.TaskID)
	if err != nil {
		t.Fatalf("GetTask returned error: %v", err)
	}
	if recovered.State != TaskStatePlanned {
		t.Fatalf("expected planned task after recovery, got %s", recovered.State)
	}
	if recovered.Metadata["recovered_from_active"] != "true" {
		t.Fatalf("expected recovery metadata, got %#v", recovered.Metadata)
	}

	state, err := store.LoadState()
	if err != nil {
		t.Fatalf("LoadState returned error: %v", err)
	}
	if state.ActiveTaskID != nil || state.Status != "idle" {
		t.Fatalf("expected idle runtime after recovery, got status=%s active=%#v", state.Status, state.ActiveTaskID)
	}
}

func TestSubmitTaskSelectsGitHubIssueSkill(t *testing.T) {
	root := tempRuntimeRoot(t)
	store := NewStore(root)
	orch := NewOrchestrator(store)

	task, err := orch.SubmitTask(CreateTaskRequest{
		Title: "Search GitHub issues",
		Goal:  "Search github issues for approval flow",
	})
	if err != nil {
		t.Fatalf("SubmitTask returned error: %v", err)
	}
	if task.SelectedSkill != "github-issue-search" {
		t.Fatalf("expected github-issue-search skill, got %s", task.SelectedSkill)
	}
}

func TestSubmitTaskSelectsEmailInboxSkill(t *testing.T) {
	root := tempRuntimeRoot(t)
	store := NewStore(root)
	orch := NewOrchestrator(store)

	task, err := orch.SubmitTask(CreateTaskRequest{
		Title: "Check email inbox",
		Goal:  "Check email inbox",
	})
	if err != nil {
		t.Fatalf("SubmitTask returned error: %v", err)
	}
	if task.SelectedSkill != "email-inbox" {
		t.Fatalf("expected email-inbox skill, got %s", task.SelectedSkill)
	}
}

func tempRuntimeRoot(t *testing.T) string {
	t.Helper()

	root := t.TempDir()
	dirs := []string{
		filepath.Join(root, "state"),
		filepath.Join(root, "tasks", TaskStateInbox),
		filepath.Join(root, "tasks", TaskStatePlanned),
		filepath.Join(root, "tasks", TaskStateActive),
		filepath.Join(root, "tasks", TaskStateBlocked),
		filepath.Join(root, "tasks", TaskStateAwaitingApproval),
		filepath.Join(root, "tasks", TaskStateDone),
		filepath.Join(root, "tasks", TaskStateFailed),
		filepath.Join(root, "tasks", TaskStateArchived),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("MkdirAll(%s): %v", dir, err)
		}
	}

	state := RuntimeState{
		RuntimeID:        "test-runtime",
		ActiveUserID:     "default-user",
		Status:           "idle",
		ExecutionProfile: "user",
	}
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("Marshal runtime state: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "state", "runtime.json"), append(data, '\n'), 0o644); err != nil {
		t.Fatalf("WriteFile runtime state: %v", err)
	}
	return root
}
