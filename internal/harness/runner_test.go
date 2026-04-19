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
		filepath.Join("..", "..", "scenarios", "memory_feedback_noop_direct_reply"),
		filepath.Join("..", "..", "scenarios", "memory_usefulness_feedback"),
		filepath.Join("..", "..", "scenarios", "metrics_runtime_snapshot"),
		filepath.Join("..", "..", "scenarios", "scheduled_memory_consolidation_after_web_search"),
		filepath.Join("..", "..", "scenarios", "failed_shell_manual_rerun"),
		filepath.Join("..", "..", "scenarios", "idempotent_shell_replay"),
		filepath.Join("..", "..", "scenarios", "procedural_promotion"),
		filepath.Join("..", "..", "scenarios", "procedural_extraction_repeated_runs"),
		filepath.Join("..", "..", "scenarios", "procedural_extraction_memory_consolidate"),
		filepath.Join("..", "..", "scenarios", "procedural_supersession_lifecycle"),
		filepath.Join("..", "..", "scenarios", "process_exit_not_retried"),
		filepath.Join("..", "..", "scenarios", "retryable_timeout_shell"),
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
		Activation: &memory.ActivationState{
			Score: 0.42,
		},
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
		{Type: AssertDurableCardVersionEquals, Field: "search_summary", Contains: "hybrid retrieval", Expected: 1},
		{Type: AssertDurableCardVersionAtLeast, Field: "search_summary", Contains: "hybrid retrieval", Min: 1},
		{Type: AssertDurableCardActivationRange, Field: "search_summary", Contains: "hybrid retrieval", MinConfidence: 0.40, MaxConfidence: 0.50},
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

func TestEvaluateMetricsAssertions(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	env, err := newRuntimeEnv(root, filepath.Join(root, "runtime"), Scenario{Name: "metrics-assertions"})
	if err != nil {
		t.Fatalf("newRuntimeEnv returned error: %v", err)
	}

	report := StepReport{
		ID:   "fetch_metrics",
		Type: StepTypeFetchMetrics,
		Metrics: MetricsSnapshot{
			TotalTasks:       3,
			TotalActions:     2,
			TotalMemoryCards: 4,
			ActiveSkills:     6,
			TasksByState:     map[string]int{"done": 1, "failed": 1, "planned": 1},
			ActionsByStatus:  map[string]int{"completed": 1, "failed": 1},
			MemoryByStatus:   map[string]int{"active": 2, "candidate": 2},
		},
	}
	env.stepResults["fetch_metrics"] = report

	assertions := []Assertion{
		{Type: AssertMetricsTotalTasksAtLeast, Step: "fetch_metrics", Min: 3},
		{Type: AssertMetricsTotalActionsAtLeast, Step: "fetch_metrics", Min: 2},
		{Type: AssertMetricsTotalMemoryAtLeast, Step: "fetch_metrics", Min: 4},
		{Type: AssertMetricsActiveSkillsAtLeast, Step: "fetch_metrics", Min: 1},
		{Type: AssertMetricsTaskStateAtLeast, Step: "fetch_metrics", Field: "failed", Min: 1},
		{Type: AssertMetricsActionStatusAtLeast, Step: "fetch_metrics", Field: "completed", Min: 1},
		{Type: AssertMetricsMemoryStatusAtLeast, Step: "fetch_metrics", Field: "active", Min: 1},
	}
	for _, assertion := range assertions {
		result := env.evaluateAssertion(assertion)
		if !result.Passed {
			t.Fatalf("assertion %s failed unexpectedly: %+v", assertion.Type, result)
		}
	}
}
