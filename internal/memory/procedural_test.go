package memory

import (
	"testing"
	"time"

	"mnemosyneos/internal/airuntime"
)

func TestBuildProcedureCandidatesFromRepeatedSuccessfulTasks(t *testing.T) {
	now := time.Now().UTC()
	tasks := []airuntime.Task{
		{
			TaskID:        "task-1",
			State:         airuntime.TaskStateDone,
			SelectedSkill: "task-plan",
			CreatedAt:     now,
			Metadata: map[string]string{
				"task_class":               "expense_audit",
				"procedure_steps":          "extract_fields\nvalidate_policy\nflag_missing_evidence",
				"procedure_guardrails":     "never invent invoice ids",
				"procedure_summary":        "Audit reimbursements with explicit policy validation.",
				"procedure_success_signal": "exceptions enumerated",
			},
		},
		{
			TaskID:        "task-2",
			State:         airuntime.TaskStateDone,
			SelectedSkill: "task-plan",
			CreatedAt:     now.Add(time.Second),
			Metadata: map[string]string{
				"task_class":               "expense_audit",
				"procedure_steps":          "extract_fields\nvalidate_policy\nflag_missing_evidence",
				"procedure_guardrails":     "never invent invoice ids",
				"procedure_summary":        "Audit reimbursements with explicit policy validation.",
				"procedure_success_signal": "exceptions enumerated",
			},
		},
		{
			TaskID:        "task-3",
			State:         airuntime.TaskStateFailed,
			SelectedSkill: "task-plan",
			CreatedAt:     now.Add(2 * time.Second),
			Metadata: map[string]string{
				"task_class":      "expense_audit",
				"procedure_steps": "extract_fields",
			},
		},
	}

	candidates, result := BuildProcedureCandidates(ProcedureExtractionRequest{
		Tasks:         tasks,
		TaskClass:     "expense_audit",
		SelectedSkill: "task-plan",
		Scope:         ScopeProject,
		MinRuns:       2,
	})
	if result.Examined != 3 || result.Matched != 2 || result.Candidates != 1 {
		t.Fatalf("unexpected extraction result: %+v", result)
	}
	if len(candidates) != 1 {
		t.Fatalf("expected one candidate, got %d", len(candidates))
	}
	card := candidates[0]
	if card.CardType != "procedure" || card.Status != CardStatusCandidate || card.Scope != ScopeProject {
		t.Fatalf("unexpected procedure candidate: %+v", card)
	}
	if card.Content["task_class"] != "expense_audit" {
		t.Fatalf("expected task_class in procedure content, got %+v", card.Content)
	}
	if card.Content["steps"] != "extract_fields\nvalidate_policy\nflag_missing_evidence" {
		t.Fatalf("unexpected steps content: %+v", card.Content)
	}
}
