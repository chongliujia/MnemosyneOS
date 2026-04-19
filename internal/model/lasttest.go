package model

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// LastTestResult captures the outcome of the most recent "Test Connection"
// action triggered from /ui/models/test.json. It is persisted next to the
// active model config so the /ui/models status banner can show whether the
// configured model is actually reachable across daemon restarts.
type LastTestResult struct {
	Provider           string    `json:"provider,omitempty"`
	Model              string    `json:"model,omitempty"`
	Ok                 bool      `json:"ok"`
	AtUTC              time.Time `json:"at_utc"`
	LatencyMs          int64     `json:"latency_ms,omitempty"`
	ReplyPreview       string    `json:"reply_preview,omitempty"`
	ToolCallsChecked   bool      `json:"tool_calls_checked"`
	ToolCallsSupported bool      `json:"tool_calls_supported"`
	ToolCallsLatencyMs int64     `json:"tool_calls_latency_ms,omitempty"`
	ToolCallsDetail    string    `json:"tool_calls_detail,omitempty"`
	Error              string    `json:"error,omitempty"`
}

// LastTestStore persists the most recent model test result to a JSON file.
// Failures to read/write are non-fatal: callers treat a missing file as
// "never tested".
type LastTestStore struct {
	path string
	mu   sync.RWMutex
	data *LastTestResult
}

// NewLastTestStore returns a store that reads/writes from the given path.
// The directory is created on demand. An existing file is loaded; I/O errors
// fall back to an empty state rather than refusing to start.
func NewLastTestStore(path string) *LastTestStore {
	store := &LastTestStore{path: path}
	if path == "" {
		return store
	}
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	if raw, err := os.ReadFile(path); err == nil {
		var res LastTestResult
		if err := json.Unmarshal(raw, &res); err == nil {
			store.data = &res
		}
	}
	return store
}

// NewLastTestStoreBesideConfig opens a last-test store using the sibling
// path last-test.json next to the given model config path. If configPath
// is empty, the store is valid but read/write becomes a no-op.
func NewLastTestStoreBesideConfig(configPath string) *LastTestStore {
	configPath = filepath.Clean(configPath)
	if configPath == "" || configPath == "." {
		return NewLastTestStore("")
	}
	return NewLastTestStore(filepath.Join(filepath.Dir(configPath), "last-test.json"))
}

// Get returns a copy of the most recent result, or nil if none is stored.
func (s *LastTestStore) Get() *LastTestResult {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.data == nil {
		return nil
	}
	copied := *s.data
	return &copied
}

// Save writes the result both in-memory and to disk. A zero AtUTC is
// populated with the current time.
func (s *LastTestStore) Save(res LastTestResult) error {
	if s == nil {
		return nil
	}
	if res.AtUTC.IsZero() {
		res.AtUTC = time.Now().UTC()
	}
	s.mu.Lock()
	copied := res
	s.data = &copied
	path := s.path
	s.mu.Unlock()
	if path == "" {
		return nil
	}
	raw, err := json.MarshalIndent(res, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	return os.WriteFile(path, raw, 0o600)
}

// Path returns the filesystem location of the backing JSON file, or "" if
// the store was created without a backing path.
func (s *LastTestStore) Path() string {
	if s == nil {
		return ""
	}
	return s.path
}
