package memoryscheduler

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"mnemosyneos/internal/airuntime"
	"mnemosyneos/internal/memory"
)

func TestEvaluateAndScheduleCreatesMemoryConsolidationTask(t *testing.T) {
	root := tempSchedulerRuntimeRoot(t)
	runtimeStore := airuntime.NewStore(root)
	memoryStore := memory.NewStore()
	if _, err := memoryStore.CreateCard(memory.CreateCardRequest{
		CardID:   "candidate:web:1",
		CardType: "web_result",
		Scope:    memory.ScopeProject,
		Status:   memory.CardStatusCandidate,
		Content:  map[string]any{"snippet": "candidate"},
	}); err != nil {
		t.Fatalf("CreateCard returned error: %v", err)
	}

	scheduler := New(runtimeStore, memoryStore, Config{Scope: memory.ScopeProject, Cooldown: time.Minute})
	decision, err := scheduler.EvaluateAndSchedule("task_completion")
	if err != nil {
		t.Fatalf("EvaluateAndSchedule returned error: %v", err)
	}
	if !decision.Triggered {
		t.Fatalf("expected scheduler to trigger, got %+v", decision)
	}
	task, err := runtimeStore.GetTask(decision.TaskID)
	if err != nil {
		t.Fatalf("GetTask returned error: %v", err)
	}
	if task.SelectedSkill != "memory-consolidate" {
		t.Fatalf("expected memory-consolidate skill, got %s", task.SelectedSkill)
	}
	if task.Metadata["scheduled_reason"] != "task_completion" {
		t.Fatalf("expected scheduled_reason metadata, got %+v", task.Metadata)
	}
	if _, err := os.Stat(filepath.Join(root, "state", "memory_scheduler.json")); err != nil {
		t.Fatalf("expected scheduler state file: %v", err)
	}
}

func TestEvaluateAndScheduleSkipsWhenNoCandidates(t *testing.T) {
	root := tempSchedulerRuntimeRoot(t)
	runtimeStore := airuntime.NewStore(root)
	memoryStore := memory.NewStore()

	scheduler := New(runtimeStore, memoryStore, Config{Scope: memory.ScopeProject, Cooldown: time.Minute})
	decision, err := scheduler.EvaluateAndSchedule("task_completion")
	if err != nil {
		t.Fatalf("EvaluateAndSchedule returned error: %v", err)
	}
	if decision.Triggered {
		t.Fatalf("expected scheduler not to trigger, got %+v", decision)
	}
	if decision.SkipReason != "no_eligible_candidates" {
		t.Fatalf("expected no_eligible_candidates skip, got %+v", decision)
	}
}

func TestEvaluateAndScheduleSkipsWhenExistingConsolidationTaskExists(t *testing.T) {
	root := tempSchedulerRuntimeRoot(t)
	runtimeStore := airuntime.NewStore(root)
	memoryStore := memory.NewStore()
	if _, err := memoryStore.CreateCard(memory.CreateCardRequest{
		CardID:   "candidate:web:1",
		CardType: "web_result",
		Scope:    memory.ScopeProject,
		Status:   memory.CardStatusCandidate,
		Content:  map[string]any{"snippet": "candidate"},
	}); err != nil {
		t.Fatalf("CreateCard returned error: %v", err)
	}
	if _, err := runtimeStore.CreateTask(airuntime.CreateTaskRequest{
		Title:         "Pending memory consolidate",
		Goal:          "Consolidate candidate memory",
		SelectedSkill: "memory-consolidate",
	}); err != nil {
		t.Fatalf("CreateTask returned error: %v", err)
	}

	scheduler := New(runtimeStore, memoryStore, Config{Scope: memory.ScopeProject, Cooldown: time.Minute})
	decision, err := scheduler.EvaluateAndSchedule("task_completion")
	if err != nil {
		t.Fatalf("EvaluateAndSchedule returned error: %v", err)
	}
	if decision.Triggered {
		t.Fatalf("expected scheduler not to trigger, got %+v", decision)
	}
	if decision.SkipReason != "existing_consolidation_task" {
		t.Fatalf("expected existing_consolidation_task skip, got %+v", decision)
	}
	if decision.TaskID == "" || decision.SelectedSkill != "memory-consolidate" {
		t.Fatalf("expected existing consolidation task details, got %+v", decision)
	}
}

func TestEvaluateAndScheduleCooldownReturnsLastScheduledTask(t *testing.T) {
	root := tempSchedulerRuntimeRoot(t)
	runtimeStore := airuntime.NewStore(root)
	memoryStore := memory.NewStore()
	if _, err := memoryStore.CreateCard(memory.CreateCardRequest{
		CardID:   "candidate:web:1",
		CardType: "web_result",
		Scope:    memory.ScopeProject,
		Status:   memory.CardStatusCandidate,
		Content:  map[string]any{"snippet": "candidate"},
	}); err != nil {
		t.Fatalf("CreateCard returned error: %v", err)
	}

	scheduler := New(runtimeStore, memoryStore, Config{Scope: memory.ScopeProject, Cooldown: time.Hour})
	first, err := scheduler.EvaluateAndSchedule("task_completion")
	if err != nil {
		t.Fatalf("first EvaluateAndSchedule returned error: %v", err)
	}
	if !first.Triggered {
		t.Fatalf("expected first scheduler run to trigger, got %+v", first)
	}
	second, err := scheduler.EvaluateAndSchedule("task_completion")
	if err != nil {
		t.Fatalf("second EvaluateAndSchedule returned error: %v", err)
	}
	if second.Triggered || second.SkipReason != "cooldown" {
		t.Fatalf("expected cooldown skip, got %+v", second)
	}
	if second.TaskID != first.TaskID || second.SelectedSkill != "memory-consolidate" {
		t.Fatalf("expected cooldown result to carry prior task details, got %+v", second)
	}
}

func TestEvaluateAndScheduleSkipsWhenCandidateThresholdNotMet(t *testing.T) {
	root := tempSchedulerRuntimeRoot(t)
	runtimeStore := airuntime.NewStore(root)
	memoryStore := memory.NewStore()
	if _, err := memoryStore.CreateCard(memory.CreateCardRequest{
		CardID:   "candidate:web:1",
		CardType: "web_result",
		Scope:    memory.ScopeProject,
		Status:   memory.CardStatusCandidate,
		Content:  map[string]any{"snippet": "candidate"},
	}); err != nil {
		t.Fatalf("CreateCard returned error: %v", err)
	}

	scheduler := New(runtimeStore, memoryStore, Config{
		Scope:         memory.ScopeProject,
		Cooldown:      time.Minute,
		MinCandidates: 2,
	})
	decision, err := scheduler.EvaluateAndSchedule("task_completion")
	if err != nil {
		t.Fatalf("EvaluateAndSchedule returned error: %v", err)
	}
	if decision.Triggered {
		t.Fatalf("expected scheduler not to trigger, got %+v", decision)
	}
	if decision.SkipReason != "candidate_threshold" {
		t.Fatalf("expected candidate_threshold skip, got %+v", decision)
	}
	if decision.CandidateCount != 1 {
		t.Fatalf("expected one eligible candidate, got %+v", decision)
	}
}

func TestEvaluateAndScheduleSkipsWhenCandidatesDoNotMatchAllowedTypes(t *testing.T) {
	root := tempSchedulerRuntimeRoot(t)
	runtimeStore := airuntime.NewStore(root)
	memoryStore := memory.NewStore()
	if _, err := memoryStore.CreateCard(memory.CreateCardRequest{
		CardID:   "candidate:file:1",
		CardType: "file_note",
		Scope:    memory.ScopeProject,
		Status:   memory.CardStatusCandidate,
		Content:  map[string]any{"snippet": "candidate"},
	}); err != nil {
		t.Fatalf("CreateCard returned error: %v", err)
	}

	scheduler := New(runtimeStore, memoryStore, Config{
		Scope:            memory.ScopeProject,
		Cooldown:         time.Minute,
		AllowedCardTypes: []string{"web_result", "email_thread", "procedure"},
	})
	decision, err := scheduler.EvaluateAndSchedule("task_completion")
	if err != nil {
		t.Fatalf("EvaluateAndSchedule returned error: %v", err)
	}
	if decision.Triggered {
		t.Fatalf("expected scheduler not to trigger, got %+v", decision)
	}
	if decision.SkipReason != "no_eligible_candidates" {
		t.Fatalf("expected no_eligible_candidates skip, got %+v", decision)
	}
}

func tempSchedulerRuntimeRoot(t *testing.T) string {
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
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("MkdirAll(%s): %v", dir, err)
		}
	}
	raw := []byte("{\"runtime_id\":\"scheduler-runtime\",\"active_user_id\":\"scheduler-user\",\"status\":\"idle\",\"execution_profile\":\"user\"}\n")
	if err := os.WriteFile(filepath.Join(root, "state", "runtime.json"), raw, 0o644); err != nil {
		t.Fatalf("WriteFile runtime state: %v", err)
	}
	return root
}
