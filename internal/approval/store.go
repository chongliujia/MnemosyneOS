package approval

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

var (
	ErrApprovalNotFound     = errors.New("approval not found")
	ErrApprovalInvalidInput = errors.New("invalid approval input")
	ErrApprovalDenied       = errors.New("approval denied")
)

type Store struct {
	mu         sync.Mutex
	rootDir    string
	defaultTTL time.Duration
}

func NewStore(rootDir string, defaultTTL time.Duration) *Store {
	if defaultTTL <= 0 {
		defaultTTL = 10 * time.Minute
	}
	return &Store{
		rootDir:    rootDir,
		defaultTTL: defaultTTL,
	}
}

func (s *Store) Create(req CreateRequest) (Request, error) {
	if strings.TrimSpace(req.ExecutionProfile) == "" || strings.TrimSpace(req.ActionKind) == "" || strings.TrimSpace(req.Summary) == "" {
		return Request{}, fmt.Errorf("%w: execution_profile, action_kind, and summary are required", ErrApprovalInvalidInput)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	record := Request{
		ApprovalID:       fmt.Sprintf("approval-%d", now.UnixNano()),
		TaskID:           strings.TrimSpace(req.TaskID),
		ExecutionProfile: strings.TrimSpace(req.ExecutionProfile),
		ActionKind:       strings.TrimSpace(req.ActionKind),
		Summary:          strings.TrimSpace(req.Summary),
		RequestedBy:      strings.TrimSpace(req.RequestedBy),
		Status:           StatusPending,
		Metadata:         cloneMetadata(req.Metadata),
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	return record, s.writeLocked(record)
}

func (s *Store) Get(approvalID string) (Request, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, _, err := s.findLocked(approvalID)
	return record, err
}

func (s *Store) List(status string) ([]Request, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	statuses := []string{StatusPending, StatusApproved, StatusDenied, StatusConsumed}
	if strings.TrimSpace(status) != "" {
		statuses = []string{strings.TrimSpace(status)}
	}

	records := make([]Request, 0)
	for _, name := range statuses {
		dir := filepath.Join(s.rootDir, "approvals", name)
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
			var record Request
			if err := s.readJSON(filepath.Join(dir, entry.Name()), &record); err != nil {
				return nil, err
			}
			records = append(records, record)
		}
	}

	sort.Slice(records, func(i, j int) bool {
		return records[i].CreatedAt.After(records[j].CreatedAt)
	})
	return records, nil
}

func (s *Store) Approve(approvalID, approvedBy string) (Request, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	record, path, err := s.findLocked(approvalID)
	if err != nil {
		return Request{}, err
	}
	if record.Status != StatusPending {
		return Request{}, fmt.Errorf("%w: approval %s is not pending", ErrApprovalDenied, approvalID)
	}

	token, err := randomToken(24)
	if err != nil {
		return Request{}, err
	}
	now := time.Now().UTC()
	expiresAt := now.Add(s.defaultTTL)
	record.Status = StatusApproved
	record.ApprovalToken = token
	record.ApprovedBy = strings.TrimSpace(approvedBy)
	record.UpdatedAt = now
	record.ExpiresAt = &expiresAt

	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return Request{}, err
	}
	return record, s.writeLocked(record)
}

func (s *Store) Deny(approvalID, deniedBy, reason string) (Request, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	record, path, err := s.findLocked(approvalID)
	if err != nil {
		return Request{}, err
	}
	if record.Status != StatusPending && record.Status != StatusApproved {
		return Request{}, fmt.Errorf("%w: approval %s is not pending or approved", ErrApprovalDenied, approvalID)
	}

	now := time.Now().UTC()
	record.Status = StatusDenied
	record.ApprovalToken = ""
	record.ApprovedBy = ""
	record.DeniedBy = strings.TrimSpace(deniedBy)
	record.DeniedReason = strings.TrimSpace(reason)
	record.UpdatedAt = now
	record.ExpiresAt = nil

	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return Request{}, err
	}
	return record, s.writeLocked(record)
}

func (s *Store) Consume(profile, token string) (Request, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if strings.TrimSpace(profile) == "" || strings.TrimSpace(token) == "" {
		return Request{}, fmt.Errorf("%w: profile and token are required", ErrApprovalInvalidInput)
	}

	dir := filepath.Join(s.rootDir, "approvals", StatusApproved)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Request{}, fmt.Errorf("%w: no approved requests available", ErrApprovalDenied)
		}
		return Request{}, err
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		var record Request
		if err := s.readJSON(path, &record); err != nil {
			return Request{}, err
		}
		if record.ExecutionProfile != strings.TrimSpace(profile) {
			continue
		}
		if subtle.ConstantTimeCompare([]byte(strings.TrimSpace(token)), []byte(record.ApprovalToken)) != 1 {
			continue
		}
		if record.ExpiresAt != nil && time.Now().UTC().After(*record.ExpiresAt) {
			return Request{}, fmt.Errorf("%w: approval token expired", ErrApprovalDenied)
		}

		now := time.Now().UTC()
		record.Status = StatusConsumed
		record.ApprovalToken = ""
		record.UsedAt = &now
		record.UpdatedAt = now
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return Request{}, err
		}
		return record, s.writeLocked(record)
	}

	return Request{}, fmt.Errorf("%w: approval token is invalid", ErrApprovalDenied)
}

func (s *Store) findLocked(approvalID string) (Request, string, error) {
	statuses := []string{StatusPending, StatusApproved, StatusDenied, StatusConsumed}
	for _, status := range statuses {
		path := filepath.Join(s.rootDir, "approvals", status, approvalID+".json")
		if _, err := os.Stat(path); err == nil {
			var record Request
			if err := s.readJSON(path, &record); err != nil {
				return Request{}, "", err
			}
			return record, path, nil
		}
	}
	return Request{}, "", ErrApprovalNotFound
}

func (s *Store) writeLocked(record Request) error {
	path := filepath.Join(s.rootDir, "approvals", record.Status, record.ApprovalID+".json")
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

func (s *Store) readJSON(path string, target any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, target)
}

func cloneMetadata(input map[string]string) map[string]string {
	if len(input) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(input))
	for k, v := range input {
		out[k] = v
	}
	return out
}

func randomToken(bytesLen int) (string, error) {
	buf := make([]byte, bytesLen)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
