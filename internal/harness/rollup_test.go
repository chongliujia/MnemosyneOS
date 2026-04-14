package harness

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestBuildRollupAggregatesReports(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	reportA := RunReport{
		ScenarioName:     "email_inbox_summary",
		ScenarioLane:     "regression",
		ScenarioTags:     []string{"email", "memory"},
		RunDir:           root + "/run-a",
		StartedAt:        time.Now().UTC(),
		FinishedAt:       time.Now().UTC().Add(10 * time.Millisecond),
		Passed:           true,
		AssertionResults: []AssertionResult{{Type: AssertDurableCardCount, Passed: true}, {Type: AssertProcedureCount, Passed: true}},
		StepReports: []StepReport{
			{Type: StepTypeConsolidate, CardType: "procedure", PromotedCount: 2, SupersededCount: 1},
			{Type: StepTypeSendChat, MemoryFeedbackUpdates: 3, ProcedureFeedbackUpdates: 1},
			{Type: StepTypeRunTask, RetryAttempts: 1, RetrySucceeded: true, ActionReplayed: true},
			{Type: StepTypeScheduleMemory, SchedulerTriggered: true},
			{Type: StepTypeScheduleMemory, SchedulerSkipReason: "cooldown"},
			{Type: StepTypeScheduleMemory, SchedulerSkipReason: "candidate_threshold"},
			{Type: StepTypeScheduleMemory, SchedulerSkipReason: "no_eligible_candidates"},
			{Type: StepTypeScheduleMemory, SchedulerSkipReason: "existing_consolidation_task"},
			{Type: StepTypeScheduleMemory, SchedulerSkipReason: "runtime_busy"},
		},
	}
	reportB := RunReport{
		ScenarioName:     "email_inbox_summary",
		ScenarioLane:     "regression",
		ScenarioTags:     []string{"email", "memory"},
		RunDir:           root + "/run-b",
		StartedAt:        time.Now().UTC(),
		FinishedAt:       time.Now().UTC().Add(20 * time.Millisecond),
		Passed:           false,
		AssertionResults: []AssertionResult{{Type: AssertRecallContains, Passed: false, Details: "recall mismatch"}},
		Error:            "failed",
	}
	if err := os.MkdirAll(filepath.Join(root, "a"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "b"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(root, "a", "report.json"), reportA); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(root, "b", "report.json"), reportB); err != nil {
		t.Fatal(err)
	}

	rollup, err := BuildRollup(root)
	if err != nil {
		t.Fatalf("BuildRollup returned error: %v", err)
	}
	if rollup.RunCount != 2 || rollup.FailedCount != 1 || rollup.PassedCount != 1 {
		t.Fatalf("unexpected rollup counts: %+v", rollup)
	}
	if len(rollup.ScenarioResults) != 1 {
		t.Fatalf("expected one scenario result, got %+v", rollup.ScenarioResults)
	}
	if rollup.ScenarioResults[0].FailedAssertions != 1 {
		t.Fatalf("expected failed assertions to be counted, got %+v", rollup.ScenarioResults[0])
	}
	if rollup.ScenarioResults[0].DurableAssertions != 1 || rollup.ScenarioResults[0].ProcedureAssertions != 1 || rollup.ScenarioResults[0].RecallAssertions != 1 || rollup.ScenarioResults[0].MemoryFailures != 1 {
		t.Fatalf("expected memory assertion counters to be tracked, got %+v", rollup.ScenarioResults[0])
	}
	if rollup.ScenarioResults[0].ProcedurePromotions != 2 || rollup.ScenarioResults[0].ProcedureSupersedes != 1 {
		t.Fatalf("expected procedure rollup counters to be tracked, got %+v", rollup.ScenarioResults[0])
	}
	if rollup.ScenarioResults[0].MemoryFeedbackUpdates != 3 || rollup.ScenarioResults[0].ProcedureFeedbackUpdates != 1 {
		t.Fatalf("expected memory feedback counters to be tracked, got %+v", rollup.ScenarioResults[0])
	}
	if rollup.ScenarioResults[0].RetryAttempts != 1 || rollup.ScenarioResults[0].RetrySuccesses != 1 {
		t.Fatalf("expected retry counters to be tracked, got %+v", rollup.ScenarioResults[0])
	}
	if rollup.ScenarioResults[0].ActionReplays != 1 {
		t.Fatalf("expected action replay counters to be tracked, got %+v", rollup.ScenarioResults[0])
	}
	if rollup.ScenarioResults[0].SchedulerTriggers != 1 ||
		rollup.ScenarioResults[0].SchedulerCooldownSkips != 1 ||
		rollup.ScenarioResults[0].SchedulerThresholdSkips != 1 ||
		rollup.ScenarioResults[0].SchedulerTypeSkips != 1 ||
		rollup.ScenarioResults[0].SchedulerExistingSkips != 1 ||
		rollup.ScenarioResults[0].SchedulerBusySkips != 1 {
		t.Fatalf("expected scheduler counters to be tracked, got %+v", rollup.ScenarioResults[0])
	}
}

func TestBuildRollupWithTagsFiltersReports(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	reports := []RunReport{
		{ScenarioName: "chat_followup_continuity", ScenarioLane: "smoke", ScenarioTags: []string{"chat", "memory"}, RunDir: filepath.Join(root, "run-chat"), StartedAt: time.Now().UTC(), FinishedAt: time.Now().UTC().Add(5 * time.Millisecond), Passed: true},
		{ScenarioName: "root_approval_flow", ScenarioLane: "smoke", ScenarioTags: []string{"approval", "execution"}, RunDir: filepath.Join(root, "run-root"), StartedAt: time.Now().UTC(), FinishedAt: time.Now().UTC().Add(7 * time.Millisecond), Passed: true},
	}
	for i, report := range reports {
		dir := filepath.Join(root, string(rune('a'+i)))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := writeJSON(filepath.Join(dir, "report.json"), report); err != nil {
			t.Fatal(err)
		}
	}

	rollup, err := BuildRollupWithTags(root, []string{"chat"})
	if err != nil {
		t.Fatalf("BuildRollupWithTags returned error: %v", err)
	}
	if rollup.RunCount != 1 || len(rollup.ScenarioResults) != 1 || rollup.ScenarioResults[0].ScenarioName != "chat_followup_continuity" {
		t.Fatalf("unexpected filtered rollup: %+v", rollup)
	}
}

func TestBuildRollupProcedureCountersIgnoreNonProcedureConsolidation(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	report := RunReport{
		ScenarioName: "memory_write_recall_roundtrip",
		ScenarioLane: "regression",
		ScenarioTags: []string{"memory"},
		RunDir:       filepath.Join(root, "run-memory"),
		StartedAt:    time.Now().UTC(),
		FinishedAt:   time.Now().UTC().Add(5 * time.Millisecond),
		Passed:       true,
		StepReports: []StepReport{
			{Type: StepTypeConsolidate, CardType: "search_result", PromotedCount: 3, SupersededCount: 1},
			{Type: StepTypeConsolidate, CardType: "procedure", PromotedCount: 1, SupersededCount: 0},
		},
	}
	dir := filepath.Join(root, "a")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(dir, "report.json"), report); err != nil {
		t.Fatal(err)
	}

	rollup, err := BuildRollup(root)
	if err != nil {
		t.Fatalf("BuildRollup returned error: %v", err)
	}
	if len(rollup.ScenarioResults) != 1 {
		t.Fatalf("expected one scenario result, got %+v", rollup.ScenarioResults)
	}
	stats := rollup.ScenarioResults[0]
	if stats.ProcedurePromotions != 1 || stats.ProcedureSupersedes != 0 {
		t.Fatalf("expected only procedure consolidation to count, got %+v", stats)
	}
}

func TestSaveAndCheckBaselineWithTags(t *testing.T) {
	t.Parallel()

	runsRoot := t.TempDir()
	baselineRoot := t.TempDir()
	report := RunReport{
		ScenarioName: "email_inbox_summary",
		ScenarioLane: "regression",
		ScenarioTags: []string{"email", "memory"},
		RunDir:       filepath.Join(runsRoot, "run-email"),
		StartedAt:    time.Now().UTC(),
		FinishedAt:   time.Now().UTC().Add(5 * time.Millisecond),
		Passed:       true,
	}
	runDir := filepath.Join(runsRoot, "run-1")
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(runDir, "report.json"), report); err != nil {
		t.Fatal(err)
	}

	written, err := SaveBaselineWithTags(runsRoot, baselineRoot, []string{"email"})
	if err != nil {
		t.Fatalf("SaveBaselineWithTags returned error: %v", err)
	}
	if len(written) != 1 {
		t.Fatalf("expected one baseline file, got %d", len(written))
	}

	result, err := CheckBaselineWithTags(runsRoot, baselineRoot, []string{"email"})
	if err != nil {
		t.Fatalf("CheckBaselineWithTags returned error: %v", err)
	}
	if !result.Passed || result.Compared != 1 {
		t.Fatalf("unexpected baseline result: %+v", result)
	}
}

func TestBuildRollupWithScopeFiltersByLane(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	reports := []RunReport{
		{ScenarioName: "chat_followup_continuity", ScenarioLane: "smoke", ScenarioTags: []string{"chat", "memory"}, RunDir: filepath.Join(root, "run-chat"), StartedAt: time.Now().UTC(), FinishedAt: time.Now().UTC().Add(5 * time.Millisecond), Passed: true},
		{ScenarioName: "email_inbox_summary", ScenarioLane: "regression", ScenarioTags: []string{"email", "memory"}, RunDir: filepath.Join(root, "run-email"), StartedAt: time.Now().UTC(), FinishedAt: time.Now().UTC().Add(7 * time.Millisecond), Passed: true},
	}
	for i, report := range reports {
		dir := filepath.Join(root, string(rune('a'+i)))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := writeJSON(filepath.Join(dir, "report.json"), report); err != nil {
			t.Fatal(err)
		}
	}

	rollup, err := BuildRollupWithScope(root, []string{"memory"}, "regression")
	if err != nil {
		t.Fatalf("BuildRollupWithScope returned error: %v", err)
	}
	if rollup.RunCount != 1 || len(rollup.ScenarioResults) != 1 || rollup.ScenarioResults[0].ScenarioName != "email_inbox_summary" {
		t.Fatalf("unexpected scoped rollup: %+v", rollup)
	}
}

func TestNormalizeArtifactContentJSONIgnoresFormatting(t *testing.T) {
	t.Parallel()

	left := []byte("{\"a\":1,\"b\":[2,3]}")
	right := []byte("{\n  \"b\": [2,3],\n  \"a\": 1\n}")
	leftKind, leftNormalized, _ := normalizeArtifactContent(left)
	rightKind, rightNormalized, _ := normalizeArtifactContent(right)
	if leftKind != "json" || rightKind != "json" {
		t.Fatalf("expected json normalization, got %s / %s", leftKind, rightKind)
	}
	if leftNormalized != rightNormalized {
		t.Fatalf("expected equivalent normalized JSON, got %s != %s", leftNormalized, rightNormalized)
	}
	var leftObj any
	if err := json.Unmarshal([]byte(leftNormalized), &leftObj); err != nil {
		t.Fatalf("normalized JSON should remain valid: %v", err)
	}
}
