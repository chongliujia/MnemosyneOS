package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"mnemosyneos/internal/harness"
)

func main() {
	var (
		scenarioPath  = flag.String("scenario", "", "path to a scenario directory or scenario.json")
		tagFilter     = flag.String("tags", "", "comma-separated scenario tags to run, for example chat,memory")
		outRoot       = flag.String("out", "runs", "directory to write harness runs into")
		reportA       = flag.String("report-a", "", "path to a run directory or report.json for diff left side")
		reportB       = flag.String("report-b", "", "path to a run directory or report.json for diff right side")
		rollupRoot    = flag.String("rollup", "", "path to a runs directory for rollup output")
		rollupJSON    = flag.String("rollup-json", "", "optional path to save rollup JSON")
		saveBaseline  = flag.String("save-baseline", "", "path to a runs directory to save as baseline")
		checkBaseline = flag.String("check-baseline", "", "path to a runs directory to compare against baseline")
		baselineDir   = flag.String("baseline-dir", "baselines/harness", "baseline directory root")
	)
	flag.Parse()

	tags := strings.Split(*tagFilter, ",")

	if strings.TrimSpace(*rollupRoot) != "" {
		rollup, err := harness.BuildRollupWithTags(*rollupRoot, tags)
		if err != nil {
			fmt.Fprintf(os.Stderr, "build rollup: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(harness.RenderRollupText(rollup))
		if strings.TrimSpace(*rollupJSON) != "" {
			if err := harness.SaveRollup(*rollupJSON, rollup); err != nil {
				fmt.Fprintf(os.Stderr, "save rollup: %v\n", err)
				os.Exit(1)
			}
		}
		return
	}

	if strings.TrimSpace(*saveBaseline) != "" {
		written, err := harness.SaveBaselineWithTags(*saveBaseline, *baselineDir, tags)
		if err != nil {
			fmt.Fprintf(os.Stderr, "save baseline: %v\n", err)
			os.Exit(1)
		}
		for _, path := range written {
			fmt.Printf("BASELINE  %s\n", path)
		}
		return
	}

	if strings.TrimSpace(*checkBaseline) != "" {
		result, err := harness.CheckBaselineWithTags(*checkBaseline, *baselineDir, tags)
		if err != nil {
			fmt.Fprintf(os.Stderr, "check baseline: %v\n", err)
			os.Exit(1)
		}
		for _, line := range result.Lines {
			fmt.Println(line)
		}
		if !result.Passed {
			os.Exit(1)
		}
		return
	}

	if *reportA != "" || *reportB != "" {
		if strings.TrimSpace(*reportA) == "" || strings.TrimSpace(*reportB) == "" {
			fmt.Fprintln(os.Stderr, "both -report-a and -report-b are required for diff")
			os.Exit(1)
		}
		left, leftPath, err := harness.LoadRunReport(*reportA)
		if err != nil {
			fmt.Fprintf(os.Stderr, "load left report: %v\n", err)
			os.Exit(1)
		}
		right, rightPath, err := harness.LoadRunReport(*reportB)
		if err != nil {
			fmt.Fprintf(os.Stderr, "load right report: %v\n", err)
			os.Exit(1)
		}
		diff := harness.DiffReports(left, leftPath, right, rightPath)
		fmt.Printf("Diff %s <-> %s\n", diff.LeftPath, diff.RightPath)
		for _, line := range diff.Lines {
			fmt.Printf("- %s\n", line)
		}
		if diff.HasDiff {
			os.Exit(1)
		}
		return
	}

	paths, err := collectScenarioPaths(*scenarioPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "collect scenarios: %v\n", err)
		os.Exit(1)
	}
	if strings.TrimSpace(*tagFilter) != "" {
		paths, err = harness.FilterScenarioPathsByTags(paths, tags)
		if err != nil {
			fmt.Fprintf(os.Stderr, "filter scenarios: %v\n", err)
			os.Exit(1)
		}
	}

	if err := os.MkdirAll(*outRoot, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "create output root: %v\n", err)
		os.Exit(1)
	}

	ctx := context.Background()
	var failed bool
	for _, path := range paths {
		scenario, err := harness.LoadScenario(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "load scenario %s: %v\n", path, err)
			failed = true
			continue
		}
		started := time.Now()
		report, err := harness.RunScenario(ctx, scenario, *outRoot)
		duration := time.Since(started).Round(time.Millisecond)
		if err != nil {
			fmt.Printf("FAIL  %-28s %s  %v\n", scenario.Name, duration, err)
			failed = true
			continue
		}
		fmt.Printf("PASS  %-28s %s  %s\n", scenario.Name, duration, report.RunDir)
	}

	if failed {
		os.Exit(1)
	}
}

func collectScenarioPaths(single string) ([]string, error) {
	if single != "" {
		return []string{single}, nil
	}
	entries, err := os.ReadDir("scenarios")
	if err != nil {
		return nil, err
	}
	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		paths = append(paths, filepath.Join("scenarios", entry.Name()))
	}
	sort.Strings(paths)
	if len(paths) == 0 {
		return nil, fmt.Errorf("no scenarios found under scenarios/")
	}
	return paths, nil
}
