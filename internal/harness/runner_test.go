package harness

import (
	"context"
	"path/filepath"
	"testing"
)

func TestRunScenarioFixtures(t *testing.T) {
	t.Parallel()

	scenarios := []string{
		filepath.Join("..", "..", "scenarios", "email_inbox_summary"),
		filepath.Join("..", "..", "scenarios", "email_followup_continuity"),
		filepath.Join("..", "..", "scenarios", "file_read_roundtrip"),
		filepath.Join("..", "..", "scenarios", "shell_failure_observability"),
		filepath.Join("..", "..", "scenarios", "web_search_summary"),
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
