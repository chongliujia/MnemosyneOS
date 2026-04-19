package harness

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Rollup struct {
	RootDir         string          `json:"root_dir"`
	GeneratedAt     time.Time       `json:"generated_at"`
	RunCount        int             `json:"run_count"`
	PassedCount     int             `json:"passed_count"`
	FailedCount     int             `json:"failed_count"`
	ScenarioResults []ScenarioStats `json:"scenario_results"`
}

type ScenarioStats struct {
	ScenarioName             string   `json:"scenario_name"`
	RunCount                 int      `json:"run_count"`
	PassedCount              int      `json:"passed_count"`
	FailedCount              int      `json:"failed_count"`
	AssertionCount           int      `json:"assertion_count"`
	FailedAssertions         int      `json:"failed_assertions"`
	WorkingAssertions        int      `json:"working_assertions,omitempty"`
	DurableAssertions        int      `json:"durable_assertions,omitempty"`
	RecallAssertions         int      `json:"recall_assertions,omitempty"`
	ProcedureAssertions      int      `json:"procedure_assertions,omitempty"`
	ProcedurePromotions      int      `json:"procedure_promotions,omitempty"`
	ProcedureSupersedes      int      `json:"procedure_supersessions,omitempty"`
	MemoryFeedbackUpdates    int      `json:"memory_feedback_updates,omitempty"`
	ProcedureFeedbackUpdates int      `json:"procedure_feedback_updates,omitempty"`
	RetryAttempts            int      `json:"retry_attempts,omitempty"`
	RetrySuccesses           int      `json:"retry_successes,omitempty"`
	ActionReplays            int      `json:"action_replays,omitempty"`
	MetricsSnapshots         int      `json:"metrics_snapshots,omitempty"`
	MaxObservedTasks         int      `json:"max_observed_tasks,omitempty"`
	MaxObservedActions       int      `json:"max_observed_actions,omitempty"`
	MaxObservedMemoryCards   int      `json:"max_observed_memory_cards,omitempty"`
	SchedulerTriggers        int      `json:"scheduler_triggers,omitempty"`
	SchedulerCooldownSkips   int      `json:"scheduler_cooldown_skips,omitempty"`
	SchedulerThresholdSkips  int      `json:"scheduler_threshold_skips,omitempty"`
	SchedulerTypeSkips       int      `json:"scheduler_type_skips,omitempty"`
	SchedulerExistingSkips   int      `json:"scheduler_existing_task_skips,omitempty"`
	SchedulerBusySkips       int      `json:"scheduler_runtime_busy_skips,omitempty"`
	MemoryFailures           int      `json:"memory_failures,omitempty"`
	AverageDurationMS        int64    `json:"average_duration_ms"`
	LatestRunDir             string   `json:"latest_run_dir,omitempty"`
	LatestError              string   `json:"latest_error,omitempty"`
	TopFailureMessages       []string `json:"top_failure_messages,omitempty"`
}

func BuildRollup(root string) (Rollup, error) {
	return BuildRollupWithScope(root, nil, "")
}

func BuildRollupWithTags(root string, tags []string) (Rollup, error) {
	return BuildRollupWithScope(root, tags, "")
}

func BuildRollupWithScope(root string, tags []string, lane string) (Rollup, error) {
	reports, err := CollectRunReportsWithScope(root, tags, lane)
	if err != nil {
		return Rollup{}, err
	}
	rollup := Rollup{
		RootDir:         root,
		GeneratedAt:     time.Now().UTC(),
		RunCount:        len(reports),
		ScenarioResults: make([]ScenarioStats, 0),
	}
	if len(reports) == 0 {
		return rollup, nil
	}

	type accum struct {
		stats         ScenarioStats
		totalDuration time.Duration
		failures      map[string]int
	}
	byScenario := map[string]*accum{}

	for _, report := range reports {
		if report.Passed {
			rollup.PassedCount++
		} else {
			rollup.FailedCount++
		}
		name := firstNonEmpty(report.ScenarioName, "unknown")
		entry := byScenario[name]
		if entry == nil {
			entry = &accum{
				stats:    ScenarioStats{ScenarioName: name},
				failures: map[string]int{},
			}
			byScenario[name] = entry
		}
		entry.stats.RunCount++
		if report.Passed {
			entry.stats.PassedCount++
		} else {
			entry.stats.FailedCount++
		}
		entry.stats.AssertionCount += len(report.AssertionResults)
		for _, assertion := range report.AssertionResults {
			switch {
			case isWorkingMemoryAssertion(assertion.Type):
				entry.stats.WorkingAssertions++
			case isProcedureMemoryAssertion(assertion.Type):
				entry.stats.ProcedureAssertions++
			case isDurableMemoryAssertion(assertion.Type):
				entry.stats.DurableAssertions++
			case isRecallMemoryAssertion(assertion.Type):
				entry.stats.RecallAssertions++
			}
			if !assertion.Passed {
				if isWorkingMemoryAssertion(assertion.Type) || isProcedureMemoryAssertion(assertion.Type) || isDurableMemoryAssertion(assertion.Type) || isRecallMemoryAssertion(assertion.Type) {
					entry.stats.MemoryFailures++
				}
				entry.stats.FailedAssertions++
				key := firstNonEmpty(assertion.Details, assertion.Description, assertion.Type)
				entry.failures[key]++
			}
		}
		for _, step := range report.StepReports {
			entry.stats.MemoryFeedbackUpdates += step.MemoryFeedbackUpdates
			entry.stats.ProcedureFeedbackUpdates += step.ProcedureFeedbackUpdates
			entry.stats.RetryAttempts += step.RetryAttempts
			if step.RetrySucceeded {
				entry.stats.RetrySuccesses++
			}
			if step.ActionReplayed {
				entry.stats.ActionReplays++
			}
			if step.Type == StepTypeFetchMetrics {
				entry.stats.MetricsSnapshots++
				if step.Metrics.TotalTasks > entry.stats.MaxObservedTasks {
					entry.stats.MaxObservedTasks = step.Metrics.TotalTasks
				}
				if step.Metrics.TotalActions > entry.stats.MaxObservedActions {
					entry.stats.MaxObservedActions = step.Metrics.TotalActions
				}
				if step.Metrics.TotalMemoryCards > entry.stats.MaxObservedMemoryCards {
					entry.stats.MaxObservedMemoryCards = step.Metrics.TotalMemoryCards
				}
			}
			if step.Type == StepTypeScheduleMemory {
				if step.SchedulerTriggered {
					entry.stats.SchedulerTriggers++
				}
				switch strings.TrimSpace(step.SchedulerSkipReason) {
				case "cooldown":
					entry.stats.SchedulerCooldownSkips++
				case "candidate_threshold":
					entry.stats.SchedulerThresholdSkips++
				case "no_eligible_candidates":
					entry.stats.SchedulerTypeSkips++
				case "existing_consolidation_task":
					entry.stats.SchedulerExistingSkips++
				case "runtime_busy":
					entry.stats.SchedulerBusySkips++
				}
			}
			if step.Type != StepTypeConsolidate || strings.TrimSpace(step.CardType) != "procedure" {
				continue
			}
			entry.stats.ProcedurePromotions += step.PromotedCount
			entry.stats.ProcedureSupersedes += step.SupersededCount
		}
		entry.totalDuration += report.FinishedAt.Sub(report.StartedAt)
		if report.StartedAt.After(parseLatestRunTime(entry.stats.LatestRunDir)) {
			entry.stats.LatestRunDir = report.RunDir
			entry.stats.LatestError = report.Error
		}
	}

	for _, entry := range byScenario {
		if entry.stats.RunCount > 0 {
			entry.stats.AverageDurationMS = entry.totalDuration.Milliseconds() / int64(entry.stats.RunCount)
		}
		entry.stats.TopFailureMessages = topFailureMessages(entry.failures, 3)
		rollup.ScenarioResults = append(rollup.ScenarioResults, entry.stats)
	}
	sort.Slice(rollup.ScenarioResults, func(i, j int) bool {
		return rollup.ScenarioResults[i].ScenarioName < rollup.ScenarioResults[j].ScenarioName
	})
	return rollup, nil
}

func CollectRunReports(root string, tags []string) ([]RunReport, error) {
	return CollectRunReportsWithScope(root, tags, "")
}

func CollectRunReportsWithScope(root string, tags []string, lane string) ([]RunReport, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, fmt.Errorf("run root is required")
	}
	tags = NormalizeTags(tags)
	lane = NormalizeLane(lane)
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	reports := make([]RunReport, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		report, _, err := LoadRunReport(filepath.Join(root, entry.Name()))
		if err != nil {
			return nil, err
		}
		if !reportMatchesTags(report, tags) {
			continue
		}
		if lane != "" && NormalizeLane(report.ScenarioLane) != lane {
			continue
		}
		reports = append(reports, report)
	}
	sort.Slice(reports, func(i, j int) bool {
		return reports[i].StartedAt.Before(reports[j].StartedAt)
	})
	return reports, nil
}

func SaveBaseline(srcRoot, baselineRoot string) ([]string, error) {
	return SaveBaselineWithScope(srcRoot, baselineRoot, nil, "")
}

func SaveBaselineWithTags(srcRoot, baselineRoot string, tags []string) ([]string, error) {
	return SaveBaselineWithScope(srcRoot, baselineRoot, tags, "")
}

func SaveBaselineWithScope(srcRoot, baselineRoot string, tags []string, lane string) ([]string, error) {
	reports, err := CollectRunReportsWithScope(srcRoot, tags, lane)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(baselineRoot, 0o755); err != nil {
		return nil, err
	}
	written := make([]string, 0, len(reports))
	for _, report := range reports {
		scenarioDir := filepath.Join(baselineRoot, slugify(report.ScenarioName))
		if err := os.MkdirAll(scenarioDir, 0o755); err != nil {
			return nil, err
		}
		baselineReport, err := snapshotBaselineReport(report, scenarioDir)
		if err != nil {
			return nil, err
		}
		path := filepath.Join(scenarioDir, "report.json")
		if err := writeJSON(path, baselineReport); err != nil {
			return nil, err
		}
		written = append(written, path)
	}
	return written, nil
}

type BaselineCheck struct {
	Compared int
	Passed   bool
	Lines    []string
}

func CheckBaseline(srcRoot, baselineRoot string) (BaselineCheck, error) {
	return CheckBaselineWithScope(srcRoot, baselineRoot, nil, "")
}

func CheckBaselineWithTags(srcRoot, baselineRoot string, tags []string) (BaselineCheck, error) {
	return CheckBaselineWithScope(srcRoot, baselineRoot, tags, "")
}

func CheckBaselineWithScope(srcRoot, baselineRoot string, tags []string, lane string) (BaselineCheck, error) {
	reports, err := CollectRunReportsWithScope(srcRoot, tags, lane)
	if err != nil {
		return BaselineCheck{}, err
	}
	out := BaselineCheck{Passed: true, Lines: make([]string, 0)}
	for _, report := range reports {
		baselinePath := filepath.Join(baselineRoot, slugify(report.ScenarioName), "report.json")
		baseline, _, err := LoadRunReport(baselinePath)
		if err != nil {
			out.Passed = false
			out.Lines = append(out.Lines, fmt.Sprintf("%s: missing baseline (%v)", report.ScenarioName, err))
			continue
		}
		diff := DiffReports(baseline, baselinePath, report, report.RunDir)
		out.Compared++
		if diff.HasDiff {
			out.Passed = false
			out.Lines = append(out.Lines, fmt.Sprintf("%s: changed", report.ScenarioName))
			for _, line := range diff.Lines {
				out.Lines = append(out.Lines, "  "+line)
			}
		} else {
			out.Lines = append(out.Lines, fmt.Sprintf("%s: matches baseline", report.ScenarioName))
		}
	}
	return out, nil
}

func RenderRollupText(rollup Rollup) string {
	lines := []string{
		fmt.Sprintf("Runs: %d", rollup.RunCount),
		fmt.Sprintf("Passed: %d", rollup.PassedCount),
		fmt.Sprintf("Failed: %d", rollup.FailedCount),
	}
	for _, scenario := range rollup.ScenarioResults {
		lines = append(lines,
			fmt.Sprintf("%s: runs=%d pass=%d fail=%d avg=%dms failed_assertions=%d working=%d durable=%d procedure=%d recall=%d memory_failures=%d",
				scenario.ScenarioName,
				scenario.RunCount,
				scenario.PassedCount,
				scenario.FailedCount,
				scenario.AverageDurationMS,
				scenario.FailedAssertions,
				scenario.WorkingAssertions,
				scenario.DurableAssertions,
				scenario.ProcedureAssertions,
				scenario.RecallAssertions,
				scenario.MemoryFailures,
			),
		)
		if scenario.MemoryFeedbackUpdates > 0 || scenario.ProcedureFeedbackUpdates > 0 {
			lines = append(lines,
				fmt.Sprintf("  memory_feedback_updates=%d procedure_feedback_updates=%d",
					scenario.MemoryFeedbackUpdates,
					scenario.ProcedureFeedbackUpdates,
				),
			)
		}
		if scenario.RetryAttempts > 0 || scenario.RetrySuccesses > 0 {
			lines = append(lines,
				fmt.Sprintf("  retry_attempts=%d retry_successes=%d",
					scenario.RetryAttempts,
					scenario.RetrySuccesses,
				),
			)
		}
		if scenario.ActionReplays > 0 {
			lines = append(lines, fmt.Sprintf("  action_replays=%d", scenario.ActionReplays))
		}
		if scenario.MetricsSnapshots > 0 {
			lines = append(lines,
				fmt.Sprintf("  metrics_snapshots=%d max_observed_tasks=%d max_observed_actions=%d max_observed_memory_cards=%d",
					scenario.MetricsSnapshots,
					scenario.MaxObservedTasks,
					scenario.MaxObservedActions,
					scenario.MaxObservedMemoryCards,
				),
			)
		}
		if scenario.SchedulerTriggers > 0 || scenario.SchedulerCooldownSkips > 0 || scenario.SchedulerThresholdSkips > 0 || scenario.SchedulerTypeSkips > 0 || scenario.SchedulerExistingSkips > 0 || scenario.SchedulerBusySkips > 0 {
			lines = append(lines,
				fmt.Sprintf("  scheduler_triggers=%d scheduler_cooldown_skips=%d scheduler_threshold_skips=%d scheduler_type_skips=%d scheduler_existing_task_skips=%d scheduler_runtime_busy_skips=%d",
					scenario.SchedulerTriggers,
					scenario.SchedulerCooldownSkips,
					scenario.SchedulerThresholdSkips,
					scenario.SchedulerTypeSkips,
					scenario.SchedulerExistingSkips,
					scenario.SchedulerBusySkips,
				),
			)
		}
		if scenario.ProcedurePromotions > 0 || scenario.ProcedureSupersedes > 0 {
			lines = append(lines,
				fmt.Sprintf("  procedure_promotions=%d procedure_supersessions=%d",
					scenario.ProcedurePromotions,
					scenario.ProcedureSupersedes,
				),
			)
		}
		for _, failure := range scenario.TopFailureMessages {
			lines = append(lines, "  - "+failure)
		}
	}
	return strings.Join(lines, "\n")
}

func isWorkingMemoryAssertion(kind string) bool {
	switch kind {
	case AssertSessionStateContain,
		AssertWorkingTopicContains,
		AssertWorkingFocusTaskEquals,
		AssertWorkingPendingQuestionContains,
		AssertWorkingPendingActionContains:
		return true
	default:
		return false
	}
}

func isDurableMemoryAssertion(kind string) bool {
	switch kind {
	case AssertMemoryCardCount,
		AssertMemoryCardContains,
		AssertEdgeCount,
		AssertDurableCardCount,
		AssertDurableCardContains,
		AssertDurableCardStatus,
		AssertDurableCardConfidenceRange,
		AssertDurableCardScope,
		AssertDurableCardSupersedes,
		AssertDurableCardVersionEquals,
		AssertDurableCardVersionAtLeast,
		AssertDurableCardActivationRange,
		AssertEdgeExists:
		return true
	default:
		return false
	}
}

func isProcedureMemoryAssertion(kind string) bool {
	switch kind {
	case AssertProcedureCount,
		AssertProcedureContains,
		AssertProcedureStepContains:
		return true
	default:
		return false
	}
}

func isRecallMemoryAssertion(kind string) bool {
	switch kind {
	case AssertRecallContains, AssertRecallNotContains:
		return true
	default:
		return false
	}
}

func topFailureMessages(failures map[string]int, limit int) []string {
	if len(failures) == 0 || limit <= 0 {
		return nil
	}
	type item struct {
		text  string
		count int
	}
	items := make([]item, 0, len(failures))
	for text, count := range failures {
		items = append(items, item{text: text, count: count})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].count == items[j].count {
			return items[i].text < items[j].text
		}
		return items[i].count > items[j].count
	})
	if len(items) > limit {
		items = items[:limit]
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, fmt.Sprintf("%s (x%d)", item.text, item.count))
	}
	return out
}

func parseLatestRunTime(runDir string) time.Time {
	if strings.TrimSpace(runDir) == "" {
		return time.Time{}
	}
	base := filepath.Base(runDir)
	if len(base) < len("20060102T150405Z") {
		return time.Time{}
	}
	ts := base[:len("20060102T150405Z")]
	parsed, err := time.Parse("20060102T150405Z", ts)
	if err != nil {
		return time.Time{}
	}
	return parsed
}

func SaveRollup(path string, rollup Rollup) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return writeJSON(path, rollup)
}

func snapshotBaselineReport(report RunReport, scenarioDir string) (RunReport, error) {
	cloned := report
	cloned.StepReports = append([]StepReport(nil), report.StepReports...)
	for i := range cloned.StepReports {
		artifacts, err := snapshotStepFiles(scenarioDir, "artifacts", cloned.StepReports[i].ID, cloned.StepReports[i].ArtifactPaths)
		if err != nil {
			return RunReport{}, err
		}
		observations, err := snapshotStepFiles(scenarioDir, "observations", cloned.StepReports[i].ID, cloned.StepReports[i].ObservationPaths)
		if err != nil {
			return RunReport{}, err
		}
		cloned.StepReports[i].ArtifactPaths = artifacts
		cloned.StepReports[i].ObservationPaths = observations
	}
	return cloned, nil
}

func snapshotStepFiles(scenarioDir, kind, stepID string, paths []string) ([]string, error) {
	if len(paths) == 0 {
		return nil, nil
	}
	destRoot := filepath.Join(scenarioDir, kind)
	if err := os.MkdirAll(destRoot, 0o755); err != nil {
		return nil, err
	}
	slug := slugify(firstNonEmpty(stepID, "step"))
	out := make([]string, 0, len(paths))
	for i, src := range paths {
		info, err := os.Stat(src)
		if err != nil {
			if os.IsNotExist(err) {
				out = append(out, src)
				continue
			}
			return nil, err
		}
		if info.IsDir() {
			out = append(out, src)
			continue
		}
		raw, err := os.ReadFile(src)
		if err != nil {
			return nil, err
		}
		base := filepath.Base(src)
		dest := filepath.Join(destRoot, fmt.Sprintf("%s-%02d-%s", slug, i, base))
		if err := os.WriteFile(dest, raw, 0o644); err != nil {
			return nil, err
		}
		if rel, err := filepath.Rel(scenarioDir, dest); err == nil {
			out = append(out, filepath.ToSlash(rel))
		} else {
			out = append(out, dest)
		}
	}
	return out, nil
}

func reportMatchesTags(report RunReport, tags []string) bool {
	tags = NormalizeTags(tags)
	if len(tags) == 0 {
		return true
	}
	reportTags := NormalizeTags(report.ScenarioTags)
	if len(reportTags) == 0 && strings.TrimSpace(report.ScenarioPath) != "" {
		if scenario, err := LoadScenario(report.ScenarioPath); err == nil {
			reportTags = NormalizeTags(scenario.Tags)
		}
	}
	if len(reportTags) == 0 {
		return false
	}
	set := make(map[string]struct{}, len(reportTags))
	for _, tag := range reportTags {
		set[tag] = struct{}{}
	}
	for _, tag := range tags {
		if _, ok := set[tag]; ok {
			return true
		}
	}
	return false
}

func normalizeArtifactContent(raw []byte) (string, string, bool) {
	if len(raw) == 0 {
		return "empty", "", true
	}
	var v any
	if err := json.Unmarshal(raw, &v); err == nil {
		canonical, marshalErr := json.Marshal(v)
		if marshalErr == nil {
			return "json", string(canonical), true
		}
	}
	text := string(raw)
	text = strings.ReplaceAll(text, "\r\n", "\n")
	lines := strings.Split(text, "\n")
	for i := range lines {
		lines[i] = strings.Join(strings.Fields(lines[i]), " ")
	}
	normalized := make([]string, 0, len(lines))
	blank := false
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			if blank {
				continue
			}
			blank = true
			normalized = append(normalized, "")
			continue
		}
		blank = false
		normalized = append(normalized, line)
	}
	return "text", strings.TrimSpace(strings.Join(normalized, "\n")), true
}
