package harness

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type ReportDiff struct {
	LeftPath     string
	RightPath    string
	SameScenario bool
	HasDiff      bool
	Lines        []string
}

var (
	dynamicEntityIDPattern = regexp.MustCompile(`\b(task|approval|action|browser|session)-[0-9]+\b`)
	recallDetailsPattern   = regexp.MustCompile(`\brecall (hit|card)\b`)
)

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
	report = resolveReportPaths(report, filepath.Dir(reportPath))
	return report, reportPath, nil
}

func resolveReportPaths(report RunReport, reportDir string) RunReport {
	for i := range report.StepReports {
		report.StepReports[i].ArtifactPaths = resolveReportFileList(report.StepReports[i].ArtifactPaths, reportDir, "artifacts")
		report.StepReports[i].ObservationPaths = resolveReportFileList(report.StepReports[i].ObservationPaths, reportDir, "observations")
	}
	return report
}

func resolveReportFileList(paths []string, reportDir, kind string) []string {
	if len(paths) == 0 {
		return paths
	}
	out := make([]string, len(paths))
	for i, path := range paths {
		out[i] = resolveReportFilePath(path, reportDir, kind)
	}
	return out
}

func resolveReportFilePath(path, reportDir, kind string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return path
	}
	if filepath.IsAbs(path) {
		if _, err := os.Stat(path); err == nil {
			return path
		}
		candidate := filepath.Join(reportDir, kind, filepath.Base(path))
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		return path
	}
	candidate := filepath.Join(reportDir, filepath.FromSlash(path))
	if _, err := os.Stat(candidate); err == nil {
		return candidate
	}
	fallback := filepath.Join(reportDir, kind, filepath.Base(path))
	if _, err := os.Stat(fallback); err == nil {
		return fallback
	}
	return candidate
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
		if l.SchedulerTriggered != r.SchedulerTriggered {
			appendDiff("%s scheduler_triggered: %t != %t", prefix, l.SchedulerTriggered, r.SchedulerTriggered)
		}
		if l.SchedulerSkipReason != r.SchedulerSkipReason {
			appendDiff("%s scheduler_skip_reason: %q != %q", prefix, l.SchedulerSkipReason, r.SchedulerSkipReason)
		}
		if l.SchedulerCandidateCount != r.SchedulerCandidateCount {
			appendDiff("%s scheduler_candidate_count: %d != %d", prefix, l.SchedulerCandidateCount, r.SchedulerCandidateCount)
		}
		if l.Metrics.TotalTasks != r.Metrics.TotalTasks {
			appendDiff("%s metrics.total_tasks: %d != %d", prefix, l.Metrics.TotalTasks, r.Metrics.TotalTasks)
		}
		if l.Metrics.TotalActions != r.Metrics.TotalActions {
			appendDiff("%s metrics.total_actions: %d != %d", prefix, l.Metrics.TotalActions, r.Metrics.TotalActions)
		}
		if l.Metrics.TotalMemoryCards != r.Metrics.TotalMemoryCards {
			appendDiff("%s metrics.total_memory_cards: %d != %d", prefix, l.Metrics.TotalMemoryCards, r.Metrics.TotalMemoryCards)
		}
		if l.Metrics.ActiveSkills != r.Metrics.ActiveSkills {
			appendDiff("%s metrics.active_skills: %d != %d", prefix, l.Metrics.ActiveSkills, r.Metrics.ActiveSkills)
		}
		if !equalStringIntMap(l.Metrics.TasksByState, r.Metrics.TasksByState) {
			appendDiff("%s metrics.tasks_by_state: %v != %v", prefix, l.Metrics.TasksByState, r.Metrics.TasksByState)
		}
		if !equalStringIntMap(l.Metrics.ActionsByStatus, r.Metrics.ActionsByStatus) {
			appendDiff("%s metrics.actions_by_status: %v != %v", prefix, l.Metrics.ActionsByStatus, r.Metrics.ActionsByStatus)
		}
		if !equalStringIntMap(l.Metrics.ActionsByFailureCategory, r.Metrics.ActionsByFailureCategory) {
			appendDiff("%s metrics.actions_by_failure_category: %v != %v", prefix, l.Metrics.ActionsByFailureCategory, r.Metrics.ActionsByFailureCategory)
		}
		if !equalStringIntMap(l.Metrics.MemoryByStatus, r.Metrics.MemoryByStatus) {
			appendDiff("%s metrics.memory_by_status: %v != %v", prefix, l.Metrics.MemoryByStatus, r.Metrics.MemoryByStatus)
		}
		if l.ActionStatus != r.ActionStatus {
			appendDiff("%s action_status: %q != %q", prefix, l.ActionStatus, r.ActionStatus)
		}
		if l.ActionFailureCategory != r.ActionFailureCategory {
			appendDiff("%s action_failure_category: %q != %q", prefix, l.ActionFailureCategory, r.ActionFailureCategory)
		}
		if l.ActionReplayed != r.ActionReplayed {
			appendDiff("%s action_replayed: %t != %t", prefix, l.ActionReplayed, r.ActionReplayed)
		}
		if strings.TrimSpace(l.ReplayOfActionID) == "" && strings.TrimSpace(r.ReplayOfActionID) != "" ||
			strings.TrimSpace(l.ReplayOfActionID) != "" && strings.TrimSpace(r.ReplayOfActionID) == "" {
			appendDiff("%s replay_of_action_id presence: %q != %q", prefix, l.ReplayOfActionID, r.ReplayOfActionID)
		}
		if l.ActionAttempts != r.ActionAttempts {
			appendDiff("%s action_attempts: %d != %d", prefix, l.ActionAttempts, r.ActionAttempts)
		}
		if l.RetryAttempts != r.RetryAttempts {
			appendDiff("%s retry_attempts: %d != %d", prefix, l.RetryAttempts, r.RetryAttempts)
		}
		if l.RetrySucceeded != r.RetrySucceeded {
			appendDiff("%s retry_succeeded: %t != %t", prefix, l.RetrySucceeded, r.RetrySucceeded)
		}
		if l.MemoryFeedbackUpdates != r.MemoryFeedbackUpdates {
			appendDiff("%s memory_feedback_updates: %d != %d", prefix, l.MemoryFeedbackUpdates, r.MemoryFeedbackUpdates)
		}
		if l.ProcedureFeedbackUpdates != r.ProcedureFeedbackUpdates {
			appendDiff("%s procedure_feedback_updates: %d != %d", prefix, l.ProcedureFeedbackUpdates, r.ProcedureFeedbackUpdates)
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
		leftDetails := normalizeAssertionDetails(left, l.Details)
		rightDetails := normalizeAssertionDetails(right, r.Details)
		if leftDetails != rightDetails {
			appendDiff("%s details: %q != %q", prefix, leftDetails, rightDetails)
		}
	}

	if !diff.HasDiff {
		diff.Lines = append(diff.Lines, "no differences")
	}
	return diff
}

func normalizeAssertionDetails(report RunReport, details string) string {
	details = strings.TrimSpace(details)
	if details == "" {
		return ""
	}
	for _, root := range []string{strings.TrimSpace(report.RuntimeRoot), strings.TrimSpace(report.RunDir)} {
		if root == "" {
			continue
		}
		details = strings.ReplaceAll(details, root, "<runtime-root>")
	}
	details = dynamicEntityIDPattern.ReplaceAllStringFunc(details, func(id string) string {
		parts := strings.SplitN(id, "-", 2)
		return parts[0] + "-<id>"
	})
	details = recallDetailsPattern.ReplaceAllString(details, "recall result")
	return details
}

func equalStringIntMap(left, right map[string]int) bool {
	if len(left) != len(right) {
		return false
	}
	for key, value := range left {
		if right[key] != value {
			return false
		}
	}
	return true
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
