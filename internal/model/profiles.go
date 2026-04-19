package model

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// ProfileStore persists named model configurations alongside the active
// runtime/model/config.json so users can switch between multiple backends.
type ProfileStore struct {
	path string
	mu   sync.RWMutex
	data profilesFile
}

type profilesFile struct {
	Version  int               `json:"version"`
	Profiles map[string]Config `json:"profiles"`
}

// NewProfileStore opens or creates runtimeRoot/model/profiles.json.
func NewProfileStore(runtimeRoot string) (*ProfileStore, error) {
	path := filepath.Join(runtimeRoot, "model", "profiles.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	ps := &ProfileStore{
		path: path,
		data: profilesFile{Version: 1, Profiles: map[string]Config{}},
	}
	if raw, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(raw, &ps.data); err != nil {
			return nil, fmt.Errorf("parse model profiles: %w", err)
		}
		if ps.data.Profiles == nil {
			ps.data.Profiles = map[string]Config{}
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	return ps, nil
}

// ListNames returns sorted profile names.
func (s *ProfileStore) ListNames() []string {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]string, 0, len(s.data.Profiles))
	for name := range s.data.Profiles {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// Get returns a normalized copy of the named profile, if present.
func (s *ProfileStore) Get(name string) (Config, bool) {
	if s == nil {
		return Config{}, false
	}
	name, err := sanitizeProfileName(name)
	if err != nil {
		return Config{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	raw, ok := s.data.Profiles[name]
	if !ok {
		return Config{}, false
	}
	return normalizeConfig(raw), true
}

// Save upserts a profile after validation. The stored snapshot is normalized.
func (s *ProfileStore) Save(name string, cfg Config) error {
	if s == nil {
		return fmt.Errorf("model profile store is not configured")
	}
	name, err := sanitizeProfileName(name)
	if err != nil {
		return err
	}
	cfg = normalizeConfig(cfg)
	if err := validateConfig(cfg); err != nil {
		return err
	}
	if cfg.Provider == "none" {
		return fmt.Errorf("cannot save an inactive (none) provider as a profile")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.data.Profiles == nil {
		s.data.Profiles = map[string]Config{}
	}
	s.data.Version = 1
	s.data.Profiles[name] = cfg
	return s.persistLocked()
}

// Delete removes a named profile. Missing names are not an error.
func (s *ProfileStore) Delete(name string) error {
	if s == nil {
		return fmt.Errorf("model profile store is not configured")
	}
	name, err := sanitizeProfileName(name)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data.Profiles, name)
	return s.persistLocked()
}

func (s *ProfileStore) persistLocked() error {
	raw, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	return os.WriteFile(s.path, raw, 0o600)
}

func sanitizeProfileName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("profile name is required")
	}
	if len(name) > 64 {
		return "", fmt.Errorf("profile name is too long (max 64 characters)")
	}
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '-', r == '_', r == '.':
		default:
			return "", fmt.Errorf("profile name may only use letters, digits, dash, dot, and underscore")
		}
	}
	return name, nil
}
