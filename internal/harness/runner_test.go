package harness

import (
	"context"
	"path/filepath"
	"testing"

	"mnemosyneos/internal/chat"
	"mnemosyneos/internal/memory"
)

func TestRunScenarioFixtures(t *testing.T) {
	t.Parallel()

	scenarios := []string{
		filepath.Join("..", "..", "scenarios", "approval_memory_boundary"),
		filepath.Join("..", "..", "scenarios", "candidate_memory_promotion"),
		filepath.Join("..", "..", "scenarios", "email_inbox_summary"),
		filepath.Join("..", "..", "scenarios", "email_followup_continuity"),
		filepath.Join("..", "..", "scenarios", "file_read_roundtrip"),
		filepath.Join("..", "..", "scenarios", "fact_supersession_lifecycle"),
		filepath.Join("..", "..", "scenarios", "memory_write_recall_roundtrip"),
		filepath.Join("..", "..", "scenarios", "memory_contamination_resistance"),
		filepath.Join("..", "..", "scenarios", "memory_contamination_recovery"),
		filepath.Join("..", "..", "scenarios", "procedural_promotion"),
		filepath.Join("..", "..", "scenarios", "procedural_extraction_repeated_runs"),
		filepath.Join("..", "..", "scenarios", "shell_failure_observability"),
		filepath.Join("..", "..", "scenarios", "session_recovery_continuity"),
		filepath.Join("..", "..", "scenarios", "web_search_summary"),
		filepath.Join("..", "..", "scenarios", "working_memory_followup"),
		filepath.Join("..", "..", "scenarios", "root_approval_flow"),
		filepath.Join("..", "..", "scenarios", "chat_followup_continuity"),
	}

	for _, path := range scenarios {
		path := path
		t.Run(filepath.Base(path), func(t *testing.T) {
			t.Parallel()
			scenario, err := LoadScenario(path)
			if err != nil {
				t.Fatalf("LoadScenario returned error: %v", err)
			}
			report, err := RunScenario(context.Background(), scenario, t.TempDir())
			if err != nil {
				t.Fatalf("RunScenario returned error: %v\nreport=%+v", err, report)
			}
			if !report.Passed {
				t.Fatalf("expected scenario to pass, report=%+v", report)
			}
		})
	}
}

func TestFilterScenarioPathsByTags(t *testing.T) {
	t.Parallel()

	paths := []string{
		filepath.Join("..", "..", "scenarios", "chat_followup_continuity"),
		filepath.Join("..", "..", "scenarios", "root_approval_flow"),
		filepath.Join("..", "..", "scenarios", "email_inbox_summary"),
	}

	filtered, err := FilterScenarioPathsByTags(paths, []string{"chat", "approval"})
	if err != nil {
		t.Fatalf("FilterScenarioPathsByTags returned error: %v", err)
	}
	if len(filtered) != 2 {
		t.Fatalf("expected 2 scenarios after tag filter, got %d: %+v", len(filtered), filtered)
	}
}

func TestFilterScenarioPathsByLane(t *testing.T) {
	t.Parallel()

	paths := []string{
		filepath.Join("..", "..", "scenarios", "chat_followup_continuity"),
		filepath.Join("..", "..", "scenarios", "root_approval_flow"),
		filepath.Join("..", "..", "scenarios", "email_inbox_summary"),
	}

	filtered, err := FilterScenarioPathsByLane(paths, "smoke")
	if err != nil {
		t.Fatalf("FilterScenarioPathsByLane returned error: %v", err)
	}
	if len(filtered) != 2 {
		t.Fatalf("expected 2 scenarios after lane filter, got %d: %+v", len(filtered), filtered)
	}
}

func TestEvaluateMemoryAssertions(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	env, err := newRuntimeEnv(root, filepath.Join(root, "runtime"), Scenario{Name: "memory-assertions"})
	if err != nil {
		t.Fatalf("newRuntimeEnv returned error: %v", err)
	}

	if err := env.chatStore.SaveSessionState(chat.SessionState{
		SessionID:       "default",
		Topic:           "OpenClaw memory architecture",
		FocusTaskID:     "task-123",
		PendingQuestion: "继续展开 OpenClaw memory design?",
		PendingAction:   "summarize_search_results",
	}); err != nil {
		t.Fatalf("SaveSessionState returned error: %v", err)
	}

	if _, err := env.memoryStore.CreateCard(memory.CreateCardRequest{
		CardID:     "search:1",
		CardType:   "search_summary",
		Scope:      "project",
		Status:     memory.CardStatusSuperseded,
		Supersedes: "search:0",
		Content: map[string]any{
			"summary": "OpenClaw uses hybrid retrieval and temporal decay in memory ranking.",
		},
		Provenance: memory.Provenance{
			Source:     "web-search",
			Confidence: 0.92,
		},
	}); err != nil {
		t.Fatalf("CreateCard summary returned error: %v", err)
	}
	if _, err := env.memoryStore.CreateCard(memory.CreateCardRequest{
		CardID:   "web:1",
		CardType: "web_result",
		Scope:    "project",
		Content: map[string]any{
			"title":   "OpenClaw memory ranking",
			"snippet": "Temporal decay improves recall ranking.",
		},
		Provenance: memory.Provenance{
			Source:     "web-search",
			Confidence: 0.81,
		},
	}); err != nil {
		t.Fatalf("CreateCard web_result returned error: %v", err)
	}
	if _, err := env.memoryStore.CreateEdge(memory.CreateEdgeRequest{
		EdgeID:     "edge:search:1:result:1",
		FromCardID: "search:1",
		ToCardID:   "web:1",
		EdgeType:   "search_result",
		Weight:     1.0,
		Confidence: 0.81,
	}); err != nil {
		t.Fatalf("CreateEdge returned error: %v", err)
	}
	if _, err := env.memoryStore.CreateCard(memory.CreateCardRequest{
		CardID:   "procedure:expense_audit:v1",
		CardType: "procedure",
		Scope:    "project",
		Status:   memory.CardStatusActive,
		Content: map[string]any{
			"name":           "expense_audit_v1",
			"summary":        "Audit reimbursements by extracting fields, validating policy, and flagging missing evidence.",
			"steps":          "extract_fields\nvalidate_policy\nflag_missing_evidence",
			"success_signal": "exceptions enumerated",
		},
		Provenance: memory.Provenance{
			Source:     "harness",
			Confidence: 0.95,
		},
	}); err != nil {
		t.Fatalf("CreateCard procedure returned error: %v", err)
	}

	assertions := []Assertion{
		{Type: AssertWorkingTopicContains, SessionID: "default", Contains: "OpenClaw"},
		{Type: AssertWorkingFocusTaskEquals, SessionID: "default", Equals: "task-123"},
		{Type: AssertWorkingPendingQuestionContains, SessionID: "default", Contains: "memory design"},
		{Type: AssertWorkingPendingActionContains, SessionID: "default", Contains: "summarize_search_results"},
		{Type: AssertDurableCardCount, Field: "search_summary", Min: 1},
		{Type: AssertDurableCardContains, Field: "search_summary", Contains: "hybrid retrieval"},
		{Type: AssertDurableCardStatus, Field: "search_summary", Contains: "hybrid retrieval", Equals: memory.CardStatusSuperseded},
		{Type: AssertDurableCardConfidenceRange, Field: "search_summary", Contains: "hybrid retrieval", MinConfidence: 0.90, MaxConfidence: 1.0},
		{Type: AssertDurableCardScope, Field: "search_summary", Contains: "hybrid retrieval", Equals: "project"},
		{Type: AssertDurableCardSupersedes, Field: "search_summary", Contains: "hybrid retrieval", Equals: "search:0"},
		{Type: AssertEdgeExists, Field: "search_result", Contains: "search:1"},
		{Type: AssertRecallNotContains, Query: "OpenClaw ranking", Source: "web", Contains: "nonexistent phrase"},
		{Type: AssertProcedureCount, Min: 1},
		{Type: AssertProcedureContains, Contains: "expense_audit_v1"},
		{Type: AssertProcedureStepContains, Contains: "validate_policy"},
	}
	for _, assertion := range assertions {
		result := env.evaluateAssertion(assertion)
		if !result.Passed {
			t.Fatalf("assertion %s failed unexpectedly: %+v", assertion.Type, result)
		}
	}
}
