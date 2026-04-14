package harness

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiffReportsDetectsChanges(t *testing.T) {
	t.Parallel()

	left := RunReport{
		ScenarioName:     "email_inbox_summary",
		Passed:           true,
		StepReports:      []StepReport{{ID: "run_email", Type: StepTypeRunTask, TaskState: "done", SelectedSkill: "email-inbox"}},
		AssertionResults: []AssertionResult{{Type: AssertTaskState, Step: "run_email", Passed: true, Details: "done"}},
	}
	right := left
	right.Passed = false
	right.StepReports[0].TaskState = "failed"

	diff := DiffReports(left, "left/report.json", right, "right/report.json")
	if !diff.HasDiff {
		t.Fatalf("expected diff to detect changes")
	}
	if len(diff.Lines) == 0 {
		t.Fatalf("expected diff lines")
	}
}

func TestDiffReportsDetectsMemoryFeedbackCounterChanges(t *testing.T) {
	t.Parallel()

	left := RunReport{
		ScenarioName: "memory_usefulness_feedback",
		Passed:       true,
		StepReports: []StepReport{{
			ID:                       "ask_plan",
			Type:                     StepTypeSendChat,
			MemoryFeedbackUpdates:    2,
			ProcedureFeedbackUpdates: 1,
		}},
	}
	right := left
	right.StepReports = append([]StepReport(nil), left.StepReports...)
	right.StepReports[0].MemoryFeedbackUpdates = 0
	right.StepReports[0].ProcedureFeedbackUpdates = 0

	diff := DiffReports(left, "left/report.json", right, "right/report.json")
	if !diff.HasDiff {
		t.Fatalf("expected diff to detect memory feedback counter changes")
	}
	foundMemory := false
	foundProcedure := false
	for _, line := range diff.Lines {
		if strings.Contains(line, "memory_feedback_updates") {
			foundMemory = true
		}
		if strings.Contains(line, "procedure_feedback_updates") {
			foundProcedure = true
		}
	}
	if !foundMemory || !foundProcedure {
		t.Fatalf("expected feedback counter diff lines, got %+v", diff.Lines)
	}
}

func TestDiffReportsDetectsRetryFieldChanges(t *testing.T) {
	t.Parallel()

	left := RunReport{
		ScenarioName: "retryable_timeout_shell",
		Passed:       true,
		StepReports: []StepReport{{
			ID:                    "run_shell_retry",
			Type:                  StepTypeRunTask,
			ActionStatus:          "completed",
			ActionFailureCategory: "timeout",
			ActionAttempts:        2,
			RetryAttempts:         1,
			RetrySucceeded:        true,
		}},
	}
	right := left
	right.StepReports = append([]StepReport(nil), left.StepReports...)
	right.StepReports[0].ActionStatus = "failed"
	right.StepReports[0].ActionFailureCategory = "process_exit"
	right.StepReports[0].ActionReplayed = true
	right.StepReports[0].ReplayOfActionID = "action-previous"
	right.StepReports[0].ActionAttempts = 1
	right.StepReports[0].RetryAttempts = 0
	right.StepReports[0].RetrySucceeded = false

	diff := DiffReports(left, "left/report.json", right, "right/report.json")
	if !diff.HasDiff {
		t.Fatalf("expected diff to detect retry field changes")
	}
	expectedMarkers := []string{
		"action_status",
		"action_failure_category",
		"action_replayed",
		"replay_of_action_id",
		"action_attempts",
		"retry_attempts",
		"retry_succeeded",
	}
	for _, marker := range expectedMarkers {
		found := false
		for _, line := range diff.Lines {
			if strings.Contains(line, marker) {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected diff line containing %q, got %+v", marker, diff.Lines)
		}
	}
}

func TestDiffReportsDetectsSchedulerFieldChanges(t *testing.T) {
	t.Parallel()

	left := RunReport{
		ScenarioName: "scheduled_memory_consolidation_after_web_search",
		Passed:       true,
		StepReports: []StepReport{{
			ID:                      "schedule_memory",
			Type:                    StepTypeScheduleMemory,
			SchedulerTriggered:      true,
			SchedulerSkipReason:     "",
			SchedulerCandidateCount: 2,
		}},
	}
	right := left
	right.StepReports = append([]StepReport(nil), left.StepReports...)
	right.StepReports[0].SchedulerTriggered = false
	right.StepReports[0].SchedulerSkipReason = "no_candidates"
	right.StepReports[0].SchedulerCandidateCount = 0

	diff := DiffReports(left, "left/report.json", right, "right/report.json")
	if !diff.HasDiff {
		t.Fatalf("expected diff to detect scheduler field changes")
	}
	expectedMarkers := []string{
		"scheduler_triggered",
		"scheduler_skip_reason",
		"scheduler_candidate_count",
	}
	for _, marker := range expectedMarkers {
		found := false
		for _, line := range diff.Lines {
			if strings.Contains(line, marker) {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected diff line containing %q, got %+v", marker, diff.Lines)
		}
	}
}

func TestDiffReportsDetectsArtifactContentChanges(t *testing.T) {
	t.Parallel()

	leftRoot := t.TempDir()
	rightRoot := t.TempDir()
	leftArtifact := filepath.Join(leftRoot, "artifacts", "reports", "result.md")
	rightArtifact := filepath.Join(rightRoot, "artifacts", "reports", "result.md")
	if err := os.MkdirAll(filepath.Dir(leftArtifact), 0o755); err != nil {
		t.Fatalf("mkdir left artifact dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(rightArtifact), 0o755); err != nil {
		t.Fatalf("mkdir right artifact dir: %v", err)
	}
	if err := os.WriteFile(leftArtifact, []byte("artifact left\n"), 0o644); err != nil {
		t.Fatalf("write left artifact: %v", err)
	}
	if err := os.WriteFile(rightArtifact, []byte("artifact right\n"), 0o644); err != nil {
		t.Fatalf("write right artifact: %v", err)
	}

	left := RunReport{ScenarioName: "file_read_roundtrip", RuntimeRoot: leftRoot, StepReports: []StepReport{{ID: "run", Type: StepTypeRunTask, ArtifactPaths: []string{leftArtifact}}}}
	right := RunReport{ScenarioName: "file_read_roundtrip", RuntimeRoot: rightRoot, StepReports: []StepReport{{ID: "run", Type: StepTypeRunTask, ArtifactPaths: []string{rightArtifact}}}}

	diff := DiffReports(left, "left/report.json", right, "right/report.json")
	if !diff.HasDiff {
		t.Fatalf("expected artifact content diff to be detected")
	}
	found := false
	for _, line := range diff.Lines {
		if line != "" && filepath.Base(leftArtifact) != "" && containsAll(line, "artifact[0]", "artifacts/reports/result.md") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected artifact content diff line, got %+v", diff.Lines)
	}
}

func TestDiffReportsIgnoresFormattingOnlyJSONArtifactChanges(t *testing.T) {
	t.Parallel()

	leftRoot := t.TempDir()
	rightRoot := t.TempDir()
	leftArtifact := filepath.Join(leftRoot, "artifacts", "reports", "result.json")
	rightArtifact := filepath.Join(rightRoot, "artifacts", "reports", "result.json")
	if err := os.MkdirAll(filepath.Dir(leftArtifact), 0o755); err != nil {
		t.Fatalf("mkdir left artifact dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(rightArtifact), 0o755); err != nil {
		t.Fatalf("mkdir right artifact dir: %v", err)
	}
	if err := os.WriteFile(leftArtifact, []byte("{\"a\":1,\"b\":[2,3]}"), 0o644); err != nil {
		t.Fatalf("write left artifact: %v", err)
	}
	if err := os.WriteFile(rightArtifact, []byte("{\n  \"b\": [2,3],\n  \"a\": 1\n}\n"), 0o644); err != nil {
		t.Fatalf("write right artifact: %v", err)
	}

	left := RunReport{ScenarioName: "file_read_roundtrip", RuntimeRoot: leftRoot, StepReports: []StepReport{{ID: "run", Type: StepTypeRunTask, ArtifactPaths: []string{leftArtifact}}}}
	right := RunReport{ScenarioName: "file_read_roundtrip", RuntimeRoot: rightRoot, StepReports: []StepReport{{ID: "run", Type: StepTypeRunTask, ArtifactPaths: []string{rightArtifact}}}}

	diff := DiffReports(left, "left/report.json", right, "right/report.json")
	if diff.HasDiff {
		t.Fatalf("expected formatting-only JSON artifact changes to be ignored, got %+v", diff.Lines)
	}
}

func containsAll(s string, parts ...string) bool {
	for _, p := range parts {
		if !strings.Contains(s, p) {
			return false
		}
	}
	return true
}
