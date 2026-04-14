package execution

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

var ErrActionNotFound = errors.New("action not found")

type Store struct {
	mu      sync.Mutex
	rootDir string
}

func NewStore(rootDir string) *Store {
	return &Store{rootDir: rootDir}
}

func (s *Store) Save(record ActionRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.writeRecordLocked(record)
}

func (s *Store) Get(actionID string) (ActionRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.getLocked(actionID)
}

func (s *Store) FindCompletedByIdempotency(kind, executionProfile, idempotencyKey, fingerprint string) (ActionRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.findCompletedByIdempotencyLocked(kind, executionProfile, idempotencyKey, fingerprint)
}

func (s *Store) Move(record ActionRecord, nextStatus string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if existing, path, err := s.findLocked(record.ActionID); err == nil {
		record.StartedAt = existing.StartedAt
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	record.Status = nextStatus
	return s.writeRecordLocked(record)
}

func (s *Store) List(limit int) ([]ActionRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if limit <= 0 {
		limit = 20
	}
	statuses := []string{
		ActionStatusPending,
		ActionStatusRunning,
		ActionStatusCompleted,
		ActionStatusFailed,
	}
	records := make([]ActionRecord, 0)
	for _, status := range statuses {
		dir := filepath.Join(s.rootDir, "actions", status)
		entries, err := os.ReadDir(dir)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, err
		}
		for _, entry := range entries {
			if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
				continue
			}
			var record ActionRecord
			if err := s.readJSONFile(filepath.Join(dir, entry.Name()), &record); err != nil {
				return nil, err
			}
			records = append(records, record)
		}
	}

	sort.Slice(records, func(i, j int) bool {
		return records[i].StartedAt.After(records[j].StartedAt)
	})
	if len(records) > limit {
		records = records[:limit]
	}
	return records, nil
}

func (s *Store) getLocked(actionID string) (ActionRecord, error) {
	record, _, err := s.findLocked(actionID)
	return record, err
}

func (s *Store) findLocked(actionID string) (ActionRecord, string, error) {
	statuses := []string{
		ActionStatusPending,
		ActionStatusRunning,
		ActionStatusCompleted,
		ActionStatusFailed,
	}
	for _, status := range statuses {
		path := filepath.Join(s.rootDir, "actions", status, actionID+".json")
		if _, err := os.Stat(path); err == nil {
			var record ActionRecord
			if err := s.readJSONFile(path, &record); err != nil {
				return ActionRecord{}, "", err
			}
			return record, path, nil
		}
	}
	return ActionRecord{}, "", ErrActionNotFound
}

func (s *Store) findCompletedByIdempotencyLocked(kind, executionProfile, idempotencyKey, fingerprint string) (ActionRecord, error) {
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if idempotencyKey == "" {
		return ActionRecord{}, ErrActionNotFound
	}
	dir := filepath.Join(s.rootDir, "actions", ActionStatusCompleted)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ActionRecord{}, ErrActionNotFound
		}
		return ActionRecord{}, err
	}
	records := make([]ActionRecord, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		var record ActionRecord
		if err := s.readJSONFile(filepath.Join(dir, entry.Name()), &record); err != nil {
			return ActionRecord{}, err
		}
		if record.Kind != kind {
			continue
		}
		if record.ExecutionProfile != executionProfile {
			continue
		}
		if strings.TrimSpace(record.IdempotencyKey) != idempotencyKey {
			continue
		}
		if fingerprint != "" && strings.TrimSpace(record.Metadata["request_fingerprint"]) != fingerprint {
			continue
		}
		records = append(records, record)
	}
	if len(records) == 0 {
		return ActionRecord{}, ErrActionNotFound
	}
	sort.Slice(records, func(i, j int) bool {
		return records[i].StartedAt.After(records[j].StartedAt)
	})
	return records[0], nil
}

func (s *Store) writeRecordLocked(record ActionRecord) error {
	path := filepath.Join(s.rootDir, "actions", record.Status, record.ActionID+".json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func (s *Store) readJSONFile(path string, target any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, target)
}
