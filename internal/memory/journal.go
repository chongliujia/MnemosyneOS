package memory

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// JournalEvent is a single append-only record of a mutation.
// CardSnapshot / EdgeSnapshot carry the full entity state at time of write,
// enabling complete replay without snapshot files.
type JournalEvent struct {
	Timestamp    time.Time        `json:"ts"`
	EventType    string           `json:"event"`
	EntityID     string           `json:"entity_id"`
	EntityType   string           `json:"entity_type,omitempty"`
	Version      int              `json:"version,omitempty"`
	CardSnapshot *json.RawMessage `json:"card_snapshot,omitempty"`
	EdgeSnapshot *json.RawMessage `json:"edge_snapshot,omitempty"`
}

// Journal writes append-only JSONL to a file for crash-recovery auditing.
type Journal struct {
	mu   sync.Mutex
	file *os.File
	enc  *json.Encoder
}

// OpenJournal creates or opens a journal file at the given path.
func OpenJournal(path string) (*Journal, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create journal dir: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}
	return &Journal{file: f, enc: json.NewEncoder(f)}, nil
}

// Append writes a single event to the journal.
func (j *Journal) Append(event JournalEvent) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	if err := j.enc.Encode(event); err != nil {
		return err
	}
	return j.file.Sync()
}

// Close flushes and closes the journal file.
func (j *Journal) Close() error {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.file.Close()
}

// ReadJournal reads all events from a journal file (for replay / diagnostics).
func ReadJournal(path string) ([]JournalEvent, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	var events []JournalEvent
	dec := json.NewDecoder(f)
	for {
		var e JournalEvent
		if err := dec.Decode(&e); err != nil {
			if err == io.EOF {
				break
			}
			return events, fmt.Errorf("decode journal event: %w", err)
		}
		events = append(events, e)
	}
	return events, nil
}

// ReplayFromJournal builds a fresh in-memory Store from a journal file alone,
// without needing snapshot files. Useful for disaster recovery or integrity checks.
func ReplayFromJournal(journalPath string) (*Store, error) {
	events, err := ReadJournal(journalPath)
	if err != nil {
		return nil, fmt.Errorf("read journal for replay: %w", err)
	}

	s := NewStore()
	for i, ev := range events {
		switch ev.EventType {
		case "card_created", "card_updated", "card_touched", "decay_applied":
			if ev.CardSnapshot == nil {
				continue
			}
			var versions []Card
			if err := json.Unmarshal(*ev.CardSnapshot, &versions); err != nil {
				return nil, fmt.Errorf("replay event %d (%s): unmarshal card snapshot: %w", i, ev.EventType, err)
			}
			if len(versions) > 0 {
				s.cards[versions[0].CardID] = versions
			}
		case "edge_created":
			if ev.EdgeSnapshot == nil {
				continue
			}
			var edge Edge
			if err := json.Unmarshal(*ev.EdgeSnapshot, &edge); err != nil {
				return nil, fmt.Errorf("replay event %d (%s): unmarshal edge snapshot: %w", i, ev.EventType, err)
			}
			if _, exists := s.edges[edge.EdgeID]; !exists {
				s.edgesByCard[edge.FromCardID] = append(s.edgesByCard[edge.FromCardID], edge.EdgeID)
				s.edgesByCard[edge.ToCardID] = append(s.edgesByCard[edge.ToCardID], edge.EdgeID)
			}
			s.edges[edge.EdgeID] = edge
		}
	}
	return s, nil
}

// VerifyIntegrity compares the current in-memory state with a journal-replayed
// store and returns mismatches. Useful for detecting corruption.
type IntegrityReport struct {
	OK            bool     `json:"ok"`
	MissingCards  []string `json:"missing_cards,omitempty"`
	ExtraCards    []string `json:"extra_cards,omitempty"`
	MismatchCards []string `json:"mismatch_cards,omitempty"`
	MissingEdges  []string `json:"missing_edges,omitempty"`
	ExtraEdges    []string `json:"extra_edges,omitempty"`
}

func (s *Store) VerifyIntegrity() (IntegrityReport, error) {
	if !s.persistent() {
		return IntegrityReport{OK: true}, nil
	}
	journalPath := filepath.Join(s.rootDir, "journal.jsonl")
	replayed, err := ReplayFromJournal(journalPath)
	if err != nil {
		return IntegrityReport{}, fmt.Errorf("replay for integrity check: %w", err)
	}

	report := IntegrityReport{OK: true}

	s.mu.RLock()
	defer s.mu.RUnlock()

	for cardID := range replayed.cards {
		if _, ok := s.cards[cardID]; !ok {
			report.MissingCards = append(report.MissingCards, cardID)
			report.OK = false
		}
	}
	for cardID := range s.cards {
		if _, ok := replayed.cards[cardID]; !ok {
			report.ExtraCards = append(report.ExtraCards, cardID)
			report.OK = false
		}
	}
	for cardID, replayedVersions := range replayed.cards {
		if liveVersions, ok := s.cards[cardID]; ok {
			if len(liveVersions) != len(replayedVersions) {
				report.MismatchCards = append(report.MismatchCards, cardID)
				report.OK = false
			}
		}
	}

	for edgeID := range replayed.edges {
		if _, ok := s.edges[edgeID]; !ok {
			report.MissingEdges = append(report.MissingEdges, edgeID)
			report.OK = false
		}
	}
	for edgeID := range s.edges {
		if _, ok := replayed.edges[edgeID]; !ok {
			report.ExtraEdges = append(report.ExtraEdges, edgeID)
			report.OK = false
		}
	}

	return report, nil
}

func marshalRaw(v any) *json.RawMessage {
	data, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	raw := json.RawMessage(data)
	return &raw
}
