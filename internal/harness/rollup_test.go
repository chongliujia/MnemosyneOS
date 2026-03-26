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
		ScenarioTags:     []string{"email", "memory"},
		RunDir:           root + "/run-a",
		StartedAt:        time.Now().UTC(),
		FinishedAt:       time.Now().UTC().Add(10 * time.Millisecond),
		Passed:           true,
		AssertionResults: []AssertionResult{{Type: AssertTaskState, Passed: true}},
	}
	reportB := RunReport{
		ScenarioName:     "email_inbox_summary",
		ScenarioTags:     []string{"email", "memory"},
		RunDir:           root + "/run-b",
		StartedAt:        time.Now().UTC(),
		FinishedAt:       time.Now().UTC().Add(20 * time.Millisecond),
		Passed:           false,
		AssertionResults: []AssertionResult{{Type: AssertTaskState, Passed: false, Details: "task_state mismatch"}},
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
}

func TestBuildRollupWithTagsFiltersReports(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	reports := []RunReport{
		{ScenarioName: "chat_followup_continuity", ScenarioTags: []string{"chat", "memory"}, RunDir: filepath.Join(root, "run-chat"), StartedAt: time.Now().UTC(), FinishedAt: time.Now().UTC().Add(5 * time.Millisecond), Passed: true},
		{ScenarioName: "root_approval_flow", ScenarioTags: []string{"approval", "execution"}, RunDir: filepath.Join(root, "run-root"), StartedAt: time.Now().UTC(), FinishedAt: time.Now().UTC().Add(7 * time.Millisecond), Passed: true},
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

func TestSaveAndCheckBaselineWithTags(t *testing.T) {
	t.Parallel()

	runsRoot := t.TempDir()
	baselineRoot := t.TempDir()
	report := RunReport{
		ScenarioName: "email_inbox_summary",
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
