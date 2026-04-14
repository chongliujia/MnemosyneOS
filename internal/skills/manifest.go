package skills

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type Manifest struct {
	Version           int                `json:"version,omitempty"`
	Name              string             `json:"name"`
	Description       string             `json:"description,omitempty"`
	Uses              string             `json:"uses"`
	Enabled           *bool              `json:"enabled,omitempty"`
	DefaultMetadata   map[string]string  `json:"default_metadata,omitempty"`
	ExecutionProfile  string             `json:"execution_profile,omitempty"`
	MaintenancePolicy *MaintenancePolicy `json:"maintenance_policy,omitempty"`
	External          *ExternalConfig    `json:"external,omitempty"`
}

type State struct {
	Enabled map[string]bool `json:"enabled,omitempty"`
}

type ManifestStatus struct {
	Version      int    `json:"version,omitempty"`
	Name         string `json:"name,omitempty"`
	Path         string `json:"path"`
	Loaded       bool   `json:"loaded"`
	Source       string `json:"source,omitempty"`
	Uses         string `json:"uses,omitempty"`
	ExternalKind string `json:"external_kind,omitempty"`
	Error        string `json:"error,omitempty"`
}

var skillNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

func normalizeManifest(manifest Manifest) Manifest {
	if manifest.Version == 0 {
		manifest.Version = 1
	}
	return manifest
}

func (r *Runner) LoadSkillManifests(dir string) error {
	if r == nil {
		return fmt.Errorf("runner is nil")
	}
	r.manifests = nil
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if strings.HasSuffix(strings.ToLower(entry.Name()), ".json") {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	var failures []string
	for _, name := range names {
		path := filepath.Join(dir, name)
		status, err := r.loadSkillManifest(path)
		r.manifests = append(r.manifests, status)
		if err != nil {
			failures = append(failures, err.Error())
		}
	}
	if len(failures) > 0 {
		return fmt.Errorf("skill manifest load failures: %s", strings.Join(failures, "; "))
	}
	return nil
}

func (r *Runner) loadSkillManifest(path string) (ManifestStatus, error) {
	status := ManifestStatus{Path: path}
	raw, err := os.ReadFile(path)
	if err != nil {
		status.Error = err.Error()
		return status, err
	}
	var manifest Manifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		err = fmt.Errorf("decode skill manifest %s: %w", path, err)
		status.Error = err.Error()
		return status, err
	}
	manifest = normalizeManifest(manifest)
	status.Version = manifest.Version
	status.Name = strings.TrimSpace(manifest.Name)
	status.Uses = strings.TrimSpace(manifest.Uses)
	if manifest.External != nil {
		status.ExternalKind = strings.TrimSpace(manifest.External.Kind)
	}
	if err := validateManifest(manifest); err != nil {
		err = fmt.Errorf("validate skill manifest %s: %w", path, err)
		status.Error = err.Error()
		return status, err
	}
	name := strings.TrimSpace(manifest.Name)
	uses := strings.TrimSpace(manifest.Uses)
	enabled := true
	if manifest.Enabled != nil {
		enabled = *manifest.Enabled
	}
	var def Definition
	if manifest.External != nil {
		def = Definition{
			Name:              name,
			Description:       strings.TrimSpace(manifest.Description),
			Source:            "external",
			Enabled:           enabled,
			DefaultMetadata:   copyMetadata(manifest.DefaultMetadata),
			ExecutionProfile:  strings.TrimSpace(manifest.ExecutionProfile),
			MaintenancePolicy: manifest.MaintenancePolicy,
			External:          copyExternalConfig(manifest.External),
			Handler:           r.externalHandler(path, manifest),
		}
		status.Source = "external"
	} else {
		base, ok := r.registry.Resolve(uses)
		if !ok {
			err := fmt.Errorf("skill manifest %s references unknown handler %s", path, uses)
			status.Error = err.Error()
			return status, err
		}
		def = base
		def.Name = name
		if strings.TrimSpace(manifest.Description) != "" {
			def.Description = strings.TrimSpace(manifest.Description)
		}
		def.Source = "manifest"
		def.Uses = uses
		def.Enabled = enabled
		def.DefaultMetadata = copyMetadata(manifest.DefaultMetadata)
		def.ExecutionProfile = strings.TrimSpace(manifest.ExecutionProfile)
		if manifest.MaintenancePolicy != nil {
			def.MaintenancePolicy = manifest.MaintenancePolicy
		}
		status.Source = "manifest"
	}
	if def.Description == "" {
		def.Description = name
	}
	if err := r.RegisterSkill(def); err != nil {
		err = fmt.Errorf("register skill manifest %s: %w", path, err)
		status.Error = err.Error()
		return status, err
	}
	status.Loaded = true
	return status, nil
}

func (r *Runner) loadSkillState() (State, error) {
	path := r.skillStatePath()
	if strings.TrimSpace(path) == "" {
		return State{}, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return State{Enabled: map[string]bool{}}, nil
		}
		return State{}, err
	}
	var state State
	if err := json.Unmarshal(raw, &state); err != nil {
		return State{}, err
	}
	if state.Enabled == nil {
		state.Enabled = map[string]bool{}
	}
	return state, nil
}

func (r *Runner) saveSkillState(state State) error {
	path := r.skillStatePath()
	if strings.TrimSpace(path) == "" {
		return nil
	}
	if state.Enabled == nil {
		state.Enabled = map[string]bool{}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	return os.WriteFile(path, raw, 0o644)
}

func (r *Runner) applySkillStateOverrides() error {
	if r == nil || r.registry == nil {
		return nil
	}
	state, err := r.loadSkillState()
	if err != nil {
		return err
	}
	for name, enabled := range state.Enabled {
		if _, ok := r.registry.Resolve(name); !ok {
			continue
		}
		if err := r.registry.SetEnabled(name, enabled); err != nil {
			return err
		}
	}
	return nil
}

func (r *Runner) skillStatePath() string {
	if r == nil || r.runtimeStore == nil {
		return ""
	}
	return filepath.Join(r.runtimeStore.RootDir(), "state", "skills.json")
}

func (r *Runner) SkillsDir() string {
	if r == nil || r.runtimeStore == nil {
		return ""
	}
	return filepath.Join(r.runtimeStore.RootDir(), "skills")
}

func (r *Runner) SaveManifest(manifest Manifest) (string, error) {
	if r == nil {
		return "", fmt.Errorf("runner is nil")
	}
	manifest = normalizeManifest(manifest)
	if err := validateManifest(manifest); err != nil {
		return "", err
	}
	dir := r.SkillsDir()
	if strings.TrimSpace(dir) == "" {
		return "", fmt.Errorf("skills directory is not available")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, strings.TrimSpace(manifest.Name)+".json")
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return "", err
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", err
	}
	if err := r.ReloadSkills(); err != nil {
		return path, err
	}
	return path, nil
}

func (r *Runner) LoadManifestFile(name string) (string, error) {
	if r == nil {
		return "", fmt.Errorf("runner is nil")
	}
	name = strings.TrimSpace(name)
	if !skillNamePattern.MatchString(name) {
		return "", fmt.Errorf("invalid manifest name %q", name)
	}
	path := filepath.Join(r.SkillsDir(), name+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func copyMetadata(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func copyExternalConfig(in *ExternalConfig) *ExternalConfig {
	if in == nil {
		return nil
	}
	out := &ExternalConfig{
		Kind:            strings.TrimSpace(in.Kind),
		Command:         strings.TrimSpace(in.Command),
		Args:            append([]string(nil), in.Args...),
		WorkDir:         strings.TrimSpace(in.WorkDir),
		TimeoutMS:       in.TimeoutMS,
		AllowWriteRoot:  in.AllowWriteRoot,
		RequireApproval: in.RequireApproval,
	}
	if len(in.Env) > 0 {
		out.Env = make(map[string]string, len(in.Env))
		for key, value := range in.Env {
			out.Env[key] = value
		}
	}
	return out
}

func validateManifest(manifest Manifest) error {
	if manifest.Version < 0 {
		return fmt.Errorf("version must be >= 0")
	}
	if manifest.Version > 1 {
		return fmt.Errorf("unsupported manifest version %d", manifest.Version)
	}
	name := strings.TrimSpace(manifest.Name)
	if name == "" {
		return fmt.Errorf("missing name")
	}
	if !skillNamePattern.MatchString(name) {
		return fmt.Errorf("invalid name %q", name)
	}
	hasUses := strings.TrimSpace(manifest.Uses) != ""
	hasExternal := manifest.External != nil
	switch {
	case hasUses && hasExternal:
		return fmt.Errorf("uses and external are mutually exclusive")
	case !hasUses && !hasExternal:
		return fmt.Errorf("either uses or external is required")
	}
	if profile := strings.TrimSpace(manifest.ExecutionProfile); profile != "" && profile != "user" && profile != "root" {
		return fmt.Errorf("invalid execution_profile %q", profile)
	}
	if manifest.MaintenancePolicy != nil {
		policy := manifest.MaintenancePolicy
		scope := strings.TrimSpace(policy.Scope)
		if scope != "" && scope != memoryScopeProject && scope != memoryScopeUser {
			return fmt.Errorf("invalid maintenance_policy.scope %q", scope)
		}
		if policy.MinCandidates < 0 {
			return fmt.Errorf("maintenance_policy.min_candidates must be >= 0")
		}
		for _, cardType := range policy.AllowedCardTypes {
			if strings.TrimSpace(cardType) == "" {
				return fmt.Errorf("maintenance_policy.allowed_card_types contains empty value")
			}
		}
	}
	if manifest.External != nil {
		external := manifest.External
		if strings.TrimSpace(external.Kind) != "command" {
			return fmt.Errorf("unsupported external.kind %q", strings.TrimSpace(external.Kind))
		}
		if strings.TrimSpace(external.Command) == "" {
			return fmt.Errorf("external.command is required")
		}
		if filepath.IsAbs(strings.TrimSpace(external.Command)) {
			return fmt.Errorf("external.command must be relative to the manifest directory")
		}
		if external.TimeoutMS < 0 {
			return fmt.Errorf("external.timeout_ms must be >= 0")
		}
		if workDir := strings.TrimSpace(external.WorkDir); filepath.IsAbs(workDir) {
			return fmt.Errorf("external.workdir must be relative to the manifest directory")
		}
	}
	return nil
}
