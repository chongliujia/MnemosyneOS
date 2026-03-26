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
	ScenarioName       string   `json:"scenario_name"`
	RunCount           int      `json:"run_count"`
	PassedCount        int      `json:"passed_count"`
	FailedCount        int      `json:"failed_count"`
	AssertionCount     int      `json:"assertion_count"`
	FailedAssertions   int      `json:"failed_assertions"`
	AverageDurationMS  int64    `json:"average_duration_ms"`
	LatestRunDir       string   `json:"latest_run_dir,omitempty"`
	LatestError        string   `json:"latest_error,omitempty"`
	TopFailureMessages []string `json:"top_failure_messages,omitempty"`
}

func BuildRollup(root string) (Rollup, error) {
	return BuildRollupWithTags(root, nil)
}

func BuildRollupWithTags(root string, tags []string) (Rollup, error) {
	reports, err := CollectRunReports(root, tags)
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
			if !assertion.Passed {
				entry.stats.FailedAssertions++
				key := firstNonEmpty(assertion.Details, assertion.Description, assertion.Type)
				entry.failures[key]++
			}
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
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, fmt.Errorf("run root is required")
	}
	tags = NormalizeTags(tags)
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
		reports = append(reports, report)
	}
	sort.Slice(reports, func(i, j int) bool {
		return reports[i].StartedAt.Before(reports[j].StartedAt)
	})
	return reports, nil
}

func SaveBaseline(srcRoot, baselineRoot string) ([]string, error) {
	return SaveBaselineWithTags(srcRoot, baselineRoot, nil)
}

func SaveBaselineWithTags(srcRoot, baselineRoot string, tags []string) ([]string, error) {
	reports, err := CollectRunReports(srcRoot, tags)
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
		path := filepath.Join(scenarioDir, "report.json")
		if err := writeJSON(path, report); err != nil {
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
	return CheckBaselineWithTags(srcRoot, baselineRoot, nil)
}

func CheckBaselineWithTags(srcRoot, baselineRoot string, tags []string) (BaselineCheck, error) {
	reports, err := CollectRunReports(srcRoot, tags)
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
			fmt.Sprintf("%s: runs=%d pass=%d fail=%d avg=%dms failed_assertions=%d",
				scenario.ScenarioName,
				scenario.RunCount,
				scenario.PassedCount,
				scenario.FailedCount,
				scenario.AverageDurationMS,
				scenario.FailedAssertions,
			),
		)
		for _, failure := range scenario.TopFailureMessages {
			lines = append(lines, "  - "+failure)
		}
	}
	return strings.Join(lines, "\n")
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
