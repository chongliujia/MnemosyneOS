package harness

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type ReportDiff struct {
	LeftPath     string
	RightPath    string
	SameScenario bool
	HasDiff      bool
	Lines        []string
}

func LoadRunReport(path string) (RunReport, string, error) {
	reportPath := strings.TrimSpace(path)
	if reportPath == "" {
		return RunReport{}, "", fmt.Errorf("report path is required")
	}
	info, err := os.Stat(reportPath)
	if err != nil {
		return RunReport{}, "", err
	}
	if info.IsDir() {
		reportPath = filepath.Join(reportPath, "report.json")
	}
	raw, err := os.ReadFile(reportPath)
	if err != nil {
		return RunReport{}, "", err
	}
	var report RunReport
	if err := json.Unmarshal(raw, &report); err != nil {
		return RunReport{}, "", err
	}
	return report, reportPath, nil
}

func DiffReports(left RunReport, leftPath string, right RunReport, rightPath string) ReportDiff {
	diff := ReportDiff{
		LeftPath:     leftPath,
		RightPath:    rightPath,
		SameScenario: left.ScenarioName == right.ScenarioName,
		Lines:        make([]string, 0),
	}
	appendDiff := func(format string, args ...any) {
		diff.HasDiff = true
		diff.Lines = append(diff.Lines, fmt.Sprintf(format, args...))
	}

	if left.ScenarioName != right.ScenarioName {
		appendDiff("scenario_name: %q != %q", left.ScenarioName, right.ScenarioName)
	}
	if left.Passed != right.Passed {
		appendDiff("passed: %t != %t", left.Passed, right.Passed)
	}
	if len(left.AssertionResults) != len(right.AssertionResults) {
		appendDiff("assertion_count: %d != %d", len(left.AssertionResults), len(right.AssertionResults))
	}
	if len(left.StepReports) != len(right.StepReports) {
		appendDiff("step_count: %d != %d", len(left.StepReports), len(right.StepReports))
	}

	maxSteps := len(left.StepReports)
	if len(right.StepReports) > maxSteps {
		maxSteps = len(right.StepReports)
	}
	for i := 0; i < maxSteps; i++ {
		switch {
		case i >= len(left.StepReports):
			appendDiff("step[%d]: missing on left, right=%s/%s", i, right.StepReports[i].ID, right.StepReports[i].Type)
			continue
		case i >= len(right.StepReports):
			appendDiff("step[%d]: missing on right, left=%s/%s", i, left.StepReports[i].ID, left.StepReports[i].Type)
			continue
		}
		l := left.StepReports[i]
		r := right.StepReports[i]
		prefix := fmt.Sprintf("step[%d](%s)", i, firstNonEmpty(l.ID, r.ID))
		if l.ID != r.ID {
			appendDiff("%s id: %q != %q", prefix, l.ID, r.ID)
		}
		if l.Type != r.Type {
			appendDiff("%s type: %q != %q", prefix, l.Type, r.Type)
		}
		if l.TaskState != r.TaskState {
			appendDiff("%s task_state: %q != %q", prefix, l.TaskState, r.TaskState)
		}
		if l.SelectedSkill != r.SelectedSkill {
			appendDiff("%s selected_skill: %q != %q", prefix, l.SelectedSkill, r.SelectedSkill)
		}
		if len(l.ArtifactPaths) != len(r.ArtifactPaths) {
			appendDiff("%s artifact_count: %d != %d", prefix, len(l.ArtifactPaths), len(r.ArtifactPaths))
		}
		compareArtifactContents(prefix, l, left.RuntimeRoot, r, right.RuntimeRoot, appendDiff)
		if len(l.ObservationPaths) != len(r.ObservationPaths) {
			appendDiff("%s observation_count: %d != %d", prefix, len(l.ObservationPaths), len(r.ObservationPaths))
		}
		if len(l.Progress) != len(r.Progress) {
			appendDiff("%s progress_count: %d != %d", prefix, len(l.Progress), len(r.Progress))
		}
	}

	maxAssertions := len(left.AssertionResults)
	if len(right.AssertionResults) > maxAssertions {
		maxAssertions = len(right.AssertionResults)
	}
	for i := 0; i < maxAssertions; i++ {
		switch {
		case i >= len(left.AssertionResults):
			appendDiff("assertion[%d]: missing on left, right=%s", i, right.AssertionResults[i].Description)
			continue
		case i >= len(right.AssertionResults):
			appendDiff("assertion[%d]: missing on right, left=%s", i, left.AssertionResults[i].Description)
			continue
		}
		l := left.AssertionResults[i]
		r := right.AssertionResults[i]
		prefix := fmt.Sprintf("assertion[%d](%s)", i, l.Type)
		if l.Type != r.Type || l.Step != r.Step {
			appendDiff("%s identity: (%s,%s) != (%s,%s)", prefix, l.Type, l.Step, r.Type, r.Step)
		}
		if l.Passed != r.Passed {
			appendDiff("%s passed: %t != %t", prefix, l.Passed, r.Passed)
		}
		if l.Details != r.Details {
			appendDiff("%s details: %q != %q", prefix, l.Details, r.Details)
		}
	}

	if !diff.HasDiff {
		diff.Lines = append(diff.Lines, "no differences")
	}
	return diff
}

func compareArtifactContents(prefix string, left StepReport, leftRoot string, right StepReport, rightRoot string, appendDiff func(format string, args ...any)) {
	maxArtifacts := len(left.ArtifactPaths)
	if len(right.ArtifactPaths) > maxArtifacts {
		maxArtifacts = len(right.ArtifactPaths)
	}
	for i := 0; i < maxArtifacts; i++ {
		switch {
		case i >= len(left.ArtifactPaths):
			appendDiff("%s artifact[%d]: missing on left, right=%s", prefix, i, shortArtifactPath(rightRoot, right.ArtifactPaths[i]))
			continue
		case i >= len(right.ArtifactPaths):
			appendDiff("%s artifact[%d]: missing on right, left=%s", prefix, i, shortArtifactPath(leftRoot, left.ArtifactPaths[i]))
			continue
		}

		leftPath := left.ArtifactPaths[i]
		rightPath := right.ArtifactPaths[i]
		leftRaw, leftErr := os.ReadFile(leftPath)
		rightRaw, rightErr := os.ReadFile(rightPath)
		if leftErr != nil || rightErr != nil {
			if leftErr != nil {
				appendDiff("%s artifact[%d]: read left failed: %v", prefix, i, leftErr)
			}
			if rightErr != nil {
				appendDiff("%s artifact[%d]: read right failed: %v", prefix, i, rightErr)
			}
			continue
		}
		if string(leftRaw) == string(rightRaw) {
			continue
		}
		leftKind, leftNormalized, leftOK := normalizeArtifactContent(leftRaw)
		rightKind, rightNormalized, rightOK := normalizeArtifactContent(rightRaw)
		if leftOK && rightOK && leftKind == rightKind && leftNormalized == rightNormalized {
			continue
		}
		diffKind := "content"
		if leftKind == rightKind && leftKind != "" {
			diffKind = leftKind + " semantic"
		}
		appendDiff(
			"%s artifact[%d] %s differs: %s != %s",
			prefix,
			i,
			diffKind,
			shortArtifactPath(leftRoot, leftPath),
			shortArtifactPath(rightRoot, rightPath),
		)
	}
}

func shortArtifactPath(root string, full string) string {
	if root == "" || !filepath.IsAbs(full) {
		return filepath.Base(full)
	}
	if rel, err := filepath.Rel(root, full); err == nil && !strings.HasPrefix(rel, "..") {
		return rel
	}
	return filepath.Base(full)
}
