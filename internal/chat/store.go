package chat

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type Store struct {
	mu      sync.Mutex
	rootDir string
}

type transcript struct {
	SessionID  string     `json:"session_id"`
	Title      string     `json:"title"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
	ArchivedAt *time.Time `json:"archived_at,omitempty"`
	Messages   []Message  `json:"messages"`
}

type SessionSummary struct {
	SessionID    string    `json:"session_id"`
	Title        string    `json:"title"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	MessageCount int       `json:"message_count"`
}

func NewStore(rootDir string) *Store {
	return &Store{rootDir: rootDir}
}

func (s *Store) Append(sessionID string, message Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := s.loadLocked(sessionID)
	if err != nil {
		return err
	}
	if data.SessionID == "" {
		data.SessionID = sessionID
		data.Title = deriveSessionTitle(message.Content)
		data.CreatedAt = time.Now().UTC()
	}
	if strings.TrimSpace(data.Title) == "" {
		data.Title = deriveSessionTitle(message.Content)
	}
	data.Messages = append(data.Messages, message)
	data.UpdatedAt = time.Now().UTC()
	return s.saveLocked(data)
}

func (s *Store) Upsert(sessionID string, message Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := s.loadLocked(sessionID)
	if err != nil {
		return err
	}
	if data.SessionID == "" {
		data.SessionID = sessionID
		data.Title = deriveSessionTitle(message.Content)
		data.CreatedAt = time.Now().UTC()
	}
	replaced := false
	for i := range data.Messages {
		if data.Messages[i].MessageID == message.MessageID {
			data.Messages[i] = message
			replaced = true
			break
		}
	}
	if !replaced {
		data.Messages = append(data.Messages, message)
	}
	data.UpdatedAt = time.Now().UTC()
	return s.saveLocked(data)
}

func (s *Store) List(sessionID string, limit int) ([]Message, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := s.loadLocked(sessionID)
	if err != nil {
		return nil, err
	}
	if limit > 0 && len(data.Messages) > limit {
		return append([]Message(nil), data.Messages[len(data.Messages)-limit:]...), nil
	}
	return append([]Message(nil), data.Messages...), nil
}

func (s *Store) Sessions(limit int) ([]SessionSummary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.listSessionsLocked(filepath.Join(s.rootDir, "sessions", "history"), limit)
}

func (s *Store) ArchivedSessions(limit int) ([]SessionSummary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.listSessionsLocked(filepath.Join(s.rootDir, "sessions", "archive"), limit)
}

func (s *Store) EnsureSession(sessionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.sessionHistoryExistsLocked(sessionID) {
		return nil
	}
	now := time.Now().UTC()
	data := transcript{
		SessionID: sessionID,
		Title:     "New Session",
		CreatedAt: now,
		UpdatedAt: now,
		Messages:  []Message{},
	}
	return s.saveLocked(data)
}

func (s *Store) RenameSession(sessionID, title string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.sessionHistoryExistsLocked(sessionID) {
		return os.ErrNotExist
	}
	data, err := s.loadLocked(sessionID)
	if err != nil {
		return err
	}
	title = strings.TrimSpace(title)
	if title == "" {
		title = "New Session"
	}
	data.Title = title
	data.UpdatedAt = time.Now().UTC()
	return s.saveLocked(data)
}

func (s *Store) ArchiveSession(sessionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.sessionHistoryExistsLocked(sessionID) {
		return os.ErrNotExist
	}
	data, err := s.loadLocked(sessionID)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	data.ArchivedAt = &now
	data.UpdatedAt = now

	target := s.archivedSessionPath(sessionID)
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	if err := os.WriteFile(target, raw, 0o644); err != nil {
		return err
	}
	_ = os.Remove(s.sessionStatePath(sessionID))
	return os.Remove(s.sessionPath(sessionID))
}

func (s *Store) RestoreSession(sessionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	path := s.archivedSessionPath(sessionID)
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return os.ErrNotExist
		}
		return err
	}
	var data transcript
	if err := json.Unmarshal(raw, &data); err != nil {
		return err
	}
	now := time.Now().UTC()
	data.ArchivedAt = nil
	data.UpdatedAt = now
	if err := s.saveLocked(data); err != nil {
		return err
	}
	return os.Remove(path)
}

func (s *Store) DeleteSession(sessionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	paths := []string{
		s.sessionStatePath(sessionID),
		s.sessionPath(sessionID),
		s.archivedSessionPath(sessionID),
	}
	found := false
	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			found = true
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	if !found {
		return os.ErrNotExist
	}
	for _, path := range paths {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

func (s *Store) GetSessionState(sessionID string) (SessionState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadStateLocked(sessionID)
}

func (s *Store) SaveSessionState(state SessionState) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	state.SessionID = strings.TrimSpace(state.SessionID)
	if state.SessionID == "" {
		return os.ErrInvalid
	}
	state.UpdatedAt = time.Now().UTC()
	return s.saveStateLocked(state)
}

func (s *Store) loadLocked(sessionID string) (transcript, error) {
	path := s.sessionPath(sessionID)
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return transcript{SessionID: sessionID, Messages: []Message{}}, nil
		}
		return transcript{}, err
	}
	if len(raw) == 0 {
		return transcript{SessionID: sessionID, Messages: []Message{}}, nil
	}
	var data transcript
	if err := json.Unmarshal(raw, &data); err != nil {
		return transcript{}, err
	}
	if data.Messages == nil {
		data.Messages = []Message{}
	}
	if data.SessionID == "" {
		data.SessionID = sessionID
	}
	return data, nil
}

func (s *Store) saveLocked(data transcript) error {
	path := s.sessionPath(data.SessionID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	return os.WriteFile(path, raw, 0o644)
}

func (s *Store) sessionPath(sessionID string) string {
	return filepath.Join(s.rootDir, "sessions", "history", sessionID+".json")
}

func (s *Store) sessionStatePath(sessionID string) string {
	return filepath.Join(s.rootDir, "sessions", "state", sessionID+".json")
}

func (s *Store) loadStateLocked(sessionID string) (SessionState, error) {
	path := s.sessionStatePath(sessionID)
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return SessionState{SessionID: sessionID}, nil
		}
		return SessionState{}, err
	}
	if len(raw) == 0 {
		return SessionState{SessionID: sessionID}, nil
	}
	var state SessionState
	if err := json.Unmarshal(raw, &state); err != nil {
		return SessionState{}, err
	}
	if state.SessionID == "" {
		state.SessionID = sessionID
	}
	return state, nil
}

func (s *Store) saveStateLocked(state SessionState) error {
	path := s.sessionStatePath(state.SessionID)
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

func (s *Store) archivedSessionPath(sessionID string) string {
	return filepath.Join(s.rootDir, "sessions", "archive", sessionID+".json")
}

func (s *Store) sessionHistoryExistsLocked(sessionID string) bool {
	_, err := os.Stat(s.sessionPath(sessionID))
	return err == nil
}

func (s *Store) listSessionsLocked(dir string, limit int) ([]SessionSummary, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	summaries := make([]SessionSummary, 0)
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		var data transcript
		raw, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal(raw, &data); err != nil {
			return nil, err
		}
		summaries = append(summaries, SessionSummary{
			SessionID:    data.SessionID,
			Title:        firstNonEmpty(data.Title, data.SessionID),
			CreatedAt:    data.CreatedAt,
			UpdatedAt:    data.UpdatedAt,
			MessageCount: len(data.Messages),
		})
	}
	sort.Slice(summaries, func(i, j int) bool {
		if summaries[i].UpdatedAt.Equal(summaries[j].UpdatedAt) {
			return summaries[i].SessionID > summaries[j].SessionID
		}
		return summaries[i].UpdatedAt.After(summaries[j].UpdatedAt)
	})
	if limit > 0 && len(summaries) > limit {
		summaries = summaries[:limit]
	}
	return summaries, nil
}

func deriveSessionTitle(content string) string {
	content = strings.TrimSpace(strings.ReplaceAll(content, "\n", " "))
	if content == "" {
		return "New Session"
	}
	if len(content) > 56 {
		return content[:56] + "..."
	}
	return content
}
