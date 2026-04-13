package harness

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func LoadScenario(path string) (Scenario, error) {
	resolved, err := resolveScenarioFile(path)
	if err != nil {
		return Scenario{}, err
	}
	raw, err := os.ReadFile(resolved)
	if err != nil {
		return Scenario{}, err
	}
	var scenario Scenario
	if err := json.Unmarshal(raw, &scenario); err != nil {
		return Scenario{}, err
	}
	scenario.Dir = filepath.Dir(resolved)
	if strings.TrimSpace(scenario.Name) == "" {
		scenario.Name = filepath.Base(scenario.Dir)
	}
	scenario.Lane = NormalizeLane(scenario.Lane)
	if len(scenario.Steps) == 0 {
		return Scenario{}, fmt.Errorf("scenario %q has no steps", scenario.Name)
	}
	if len(scenario.Tags) > 0 {
		tagSet := make(map[string]struct{}, len(scenario.Tags))
		normalized := make([]string, 0, len(scenario.Tags))
		for _, tag := range scenario.Tags {
			tag = strings.ToLower(strings.TrimSpace(tag))
			if tag == "" {
				continue
			}
			if _, ok := tagSet[tag]; ok {
				continue
			}
			tagSet[tag] = struct{}{}
			normalized = append(normalized, tag)
		}
		sort.Strings(normalized)
		scenario.Tags = normalized
	}
	seen := map[string]struct{}{}
	for i := range scenario.Steps {
		if strings.TrimSpace(scenario.Steps[i].ID) == "" {
			scenario.Steps[i].ID = fmt.Sprintf("step-%02d", i+1)
		}
		if _, ok := seen[scenario.Steps[i].ID]; ok {
			return Scenario{}, fmt.Errorf("scenario %q has duplicate step id %q", scenario.Name, scenario.Steps[i].ID)
		}
		seen[scenario.Steps[i].ID] = struct{}{}
	}
	return scenario, nil
}

func NormalizeLane(lane string) string {
	lane = strings.ToLower(strings.TrimSpace(lane))
	switch lane {
	case "", "smoke", "regression", "soak":
		return lane
	default:
		return lane
	}
}

func resolveScenarioFile(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("scenario path is required")
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return filepath.Join(path, "scenario.json"), nil
	}
	return path, nil
}

func NormalizeTags(tags []string) []string {
	if len(tags) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(tags))
	for _, tag := range tags {
		tag = strings.ToLower(strings.TrimSpace(tag))
		if tag == "" {
			continue
		}
		if _, ok := seen[tag]; ok {
			continue
		}
		seen[tag] = struct{}{}
		out = append(out, tag)
	}
	sort.Strings(out)
	return out
}

func ScenarioMatchesTags(scenario Scenario, tags []string) bool {
	tags = NormalizeTags(tags)
	if len(tags) == 0 {
		return true
	}
	if len(scenario.Tags) == 0 {
		return false
	}
	tagSet := make(map[string]struct{}, len(scenario.Tags))
	for _, tag := range scenario.Tags {
		tagSet[strings.ToLower(strings.TrimSpace(tag))] = struct{}{}
	}
	for _, tag := range tags {
		if _, ok := tagSet[tag]; ok {
			return true
		}
	}
	return false
}

func FilterScenarioPathsByTags(paths []string, tags []string) ([]string, error) {
	tags = NormalizeTags(tags)
	if len(tags) == 0 {
		return paths, nil
	}
	filtered := make([]string, 0, len(paths))
	for _, path := range paths {
		scenario, err := LoadScenario(path)
		if err != nil {
			return nil, err
		}
		if ScenarioMatchesTags(scenario, tags) {
			filtered = append(filtered, path)
		}
	}
	if len(filtered) == 0 {
		return nil, fmt.Errorf("no scenarios matched tags %v", tags)
	}
	return filtered, nil
}

func FilterScenarioPathsByLane(paths []string, lane string) ([]string, error) {
	lane = NormalizeLane(lane)
	if lane == "" {
		return paths, nil
	}
	filtered := make([]string, 0, len(paths))
	for _, path := range paths {
		scenario, err := LoadScenario(path)
		if err != nil {
			return nil, err
		}
		if scenario.Lane == lane {
			filtered = append(filtered, path)
		}
	}
	if len(filtered) == 0 {
		return nil, fmt.Errorf("no scenarios matched lane %q", lane)
	}
	return filtered, nil
}
