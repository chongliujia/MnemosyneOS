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
