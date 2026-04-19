package memory

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

var (
	ErrNotFound        = errors.New("not found")
	ErrAlreadyExists   = errors.New("already exists")
	ErrInvalidArgument = errors.New("invalid argument")
)

type Store struct {
	mu          sync.RWMutex
	cards       map[string][]Card
	edges       map[string]Edge
	edgesByCard map[string][]string
	rootDir     string // empty = in-memory only
	journal     *Journal
}

// NewStore returns an in-memory-only store (for tests and harness).
func NewStore() *Store {
	return &Store{
		cards:       make(map[string][]Card),
		edges:       make(map[string]Edge),
		edgesByCard: make(map[string][]string),
	}
}

// NewPersistentStore returns a store backed by JSON files under rootDir.
// It loads existing cards/edges from disk and opens an append-only journal.
func NewPersistentStore(rootDir string) (*Store, error) {
	s := &Store{
		cards:       make(map[string][]Card),
		edges:       make(map[string]Edge),
		edgesByCard: make(map[string][]string),
		rootDir:     rootDir,
	}
	for _, sub := range []string{"cards", "edges"} {
		if err := os.MkdirAll(filepath.Join(rootDir, sub), 0o755); err != nil {
			return nil, fmt.Errorf("create memory dir %s: %w", sub, err)
		}
	}
	journal, err := OpenJournal(filepath.Join(rootDir, "journal.jsonl"))
	if err != nil {
		return nil, fmt.Errorf("open memory journal: %w", err)
	}
	s.journal = journal

	if err := s.loadFromDisk(); err != nil {
		return nil, fmt.Errorf("load memory from disk: %w", err)
	}
	return s, nil
}

// Close flushes the journal. Safe to call on in-memory stores.
func (s *Store) Close() error {
	if s.journal != nil {
		return s.journal.Close()
	}
	return nil
}

func (s *Store) persistent() bool {
	return s.rootDir != ""
}

func (s *Store) CreateCard(req CreateCardRequest) (Card, error) {
	if req.CardID == "" || req.CardType == "" {
		return Card{}, fmt.Errorf("%w: card_id and card_type are required", ErrInvalidArgument)
	}
	status := NormalizeCardStatus(req.Status)
	if status == "" {
		return Card{}, fmt.Errorf("%w: unsupported card status %q", ErrInvalidArgument, req.Status)
	}

	now := time.Now().UTC()
	card := Card{
		CardID:       req.CardID,
		CardType:     req.CardType,
		Scope:        req.Scope,
		CreatedAt:    now,
		ValidFrom:    req.ValidFrom,
		ValidTo:      req.ValidTo,
		Version:      1,
		Status:       status,
		Supersedes:   req.Supersedes,
		Content:      req.Content,
		EvidenceRefs: req.EvidenceRefs,
		Provenance:   req.Provenance,
		Activation: ActivationState{
			Score: 1.0,
		},
	}
	if req.Activation != nil {
		card.Activation.Score = clampUnit(req.Activation.Score)
		card.Activation.LastAccessAt = req.Activation.LastAccessAt
		card.Activation.LastEvaluatedAt = req.Activation.LastEvaluatedAt
		card.Activation.DecayPolicy = req.Activation.DecayPolicy
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.cards[req.CardID]; ok {
		return Card{}, ErrAlreadyExists
	}

	s.cards[req.CardID] = []Card{card}
	if s.persistent() {
		if err := s.persistCard(req.CardID); err != nil {
			return Card{}, err
		}
		if err := s.appendCardJournalEvent("card_created", req.CardID, card.CardType, card.Version); err != nil {
			return Card{}, err
		}
	}
	return card, nil
}

func (s *Store) UpdateCard(cardID string, req UpdateCardRequest) (Card, error) {
	if cardID == "" {
		return Card{}, fmt.Errorf("%w: card_id is required", ErrInvalidArgument)
	}
	if req.Status != "" && NormalizeCardStatus(req.Status) == "" {
		return Card{}, fmt.Errorf("%w: unsupported card status %q", ErrInvalidArgument, req.Status)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	versions, ok := s.cards[cardID]
	if !ok || len(versions) == 0 {
		return Card{}, ErrNotFound
	}

	latest := versions[len(versions)-1]
	now := time.Now().UTC()
	updated := Card{
		CardID:       latest.CardID,
		CardType:     latest.CardType,
		Scope:        latest.Scope,
		CreatedAt:    latest.CreatedAt,
		ValidFrom:    req.ValidFrom,
		ValidTo:      req.ValidTo,
		Version:      latest.Version + 1,
		PrevVersion:  latest.CardID + "#v" + fmt.Sprintf("%d", latest.Version),
		Status:       latest.Status,
		Supersedes:   latest.Supersedes,
		Content:      latest.Content,
		EvidenceRefs: latest.EvidenceRefs,
		Provenance:   latest.Provenance,
		Activation: ActivationState{
			Score:           latest.Activation.Score,
			LastAccessAt:    &now,
			LastEvaluatedAt: latest.Activation.LastEvaluatedAt,
			DecayPolicy:     latest.Activation.DecayPolicy,
		},
	}

	if req.Content != nil {
		updated.Content = req.Content
	}
	if req.Status != "" {
		updated.Status = NormalizeCardStatus(req.Status)
	}
	if req.Supersedes != "" {
		updated.Supersedes = req.Supersedes
	}
	if req.Scope != "" {
		updated.Scope = req.Scope
	}
	if req.EvidenceRefs != nil {
		updated.EvidenceRefs = req.EvidenceRefs
	}
	if req.Provenance.AgentID != "" || req.Provenance.Source != "" || req.Provenance.Confidence != 0 {
		updated.Provenance = req.Provenance
	}

	versions = append(versions, updated)
	s.cards[cardID] = versions
	if s.persistent() {
		if err := s.persistCard(cardID); err != nil {
			return Card{}, err
		}
		if err := s.appendCardJournalEvent("card_updated", cardID, updated.CardType, updated.Version); err != nil {
			return Card{}, err
		}
	}
	return updated, nil
}

func (s *Store) CreateEdge(req CreateEdgeRequest) (Edge, error) {
	if req.EdgeID == "" || req.FromCardID == "" || req.ToCardID == "" || req.EdgeType == "" {
		return Edge{}, fmt.Errorf("%w: edge_id/from_card_id/to_card_id/edge_type are required", ErrInvalidArgument)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.edges[req.EdgeID]; ok {
		return Edge{}, ErrAlreadyExists
	}
	if _, ok := s.cards[req.FromCardID]; !ok {
		return Edge{}, fmt.Errorf("%w: from_card_id does not exist", ErrInvalidArgument)
	}
	if _, ok := s.cards[req.ToCardID]; !ok {
		return Edge{}, fmt.Errorf("%w: to_card_id does not exist", ErrInvalidArgument)
	}

	edge := Edge{
		EdgeID:       req.EdgeID,
		FromCardID:   req.FromCardID,
		ToCardID:     req.ToCardID,
		EdgeType:     req.EdgeType,
		Weight:       req.Weight,
		Confidence:   req.Confidence,
		ValidFrom:    req.ValidFrom,
		ValidTo:      req.ValidTo,
		EvidenceRefs: req.EvidenceRefs,
		CreatedAt:    time.Now().UTC(),
	}

	s.edges[edge.EdgeID] = edge
	s.edgesByCard[edge.FromCardID] = append(s.edgesByCard[edge.FromCardID], edge.EdgeID)
	s.edgesByCard[edge.ToCardID] = append(s.edgesByCard[edge.ToCardID], edge.EdgeID)
	if s.persistent() {
		if err := s.persistEdge(edge.EdgeID); err != nil {
			return Edge{}, err
		}
		if err := s.appendEdgeJournalEvent("edge_created", edge); err != nil {
			return Edge{}, err
		}
	}
	return edge, nil
}

func (s *Store) Query(req QueryRequest) QueryResponse {
	s.mu.RLock()
	defer s.mu.RUnlock()

	resp := QueryResponse{
		Cards: make([]Card, 0),
	}

	if req.CardID != "" {
		versions := s.cards[req.CardID]
		if len(versions) == 0 {
			return resp
		}
		card := resolveAsOf(versions, req.AsOf)
		if card != nil {
			resp.Cards = append(resp.Cards, *card)
			resp.Edges = s.collectEdgesForCards([]string{req.CardID})
		}
		return resp
	}

	for _, versions := range s.cards {
		card := resolveAsOf(versions, req.AsOf)
		if card == nil {
			continue
		}
		if req.CardType != "" && card.CardType != req.CardType {
			continue
		}
		if req.Scope != "" && card.Scope != req.Scope {
			continue
		}
		if req.Status != "" && card.Status != req.Status {
			continue
		}
		resp.Cards = append(resp.Cards, *card)
	}

	cardIDs := make([]string, 0, len(resp.Cards))
	for _, card := range resp.Cards {
		cardIDs = append(cardIDs, card.CardID)
	}
	resp.Edges = s.collectEdgesForCards(cardIDs)
	return resp
}

func (s *Store) LatestCards() []Card {
	s.mu.RLock()
	defer s.mu.RUnlock()

	cards := make([]Card, 0, len(s.cards))
	for _, versions := range s.cards {
		card := resolveAsOf(versions, nil)
		if card == nil {
			continue
		}
		cards = append(cards, *card)
	}

	sort.Slice(cards, func(i, j int) bool {
		if cards[i].CreatedAt.Equal(cards[j].CreatedAt) {
			return cards[i].CardID < cards[j].CardID
		}
		return cards[i].CreatedAt.After(cards[j].CreatedAt)
	})
	return cards
}

func (s *Store) TouchCard(cardID string, activationDelta, confidenceDelta float64) (Card, error) {
	if strings.TrimSpace(cardID) == "" {
		return Card{}, fmt.Errorf("%w: card_id is required", ErrInvalidArgument)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	versions, ok := s.cards[cardID]
	if !ok || len(versions) == 0 {
		return Card{}, ErrNotFound
	}

	latest := versions[len(versions)-1]
	now := time.Now().UTC()
	updated := latest
	updated.Version = latest.Version + 1
	updated.PrevVersion = latest.CardID + "#v" + fmt.Sprintf("%d", latest.Version)
	updated.Activation = latest.Activation
	updated.Activation.LastAccessAt = &now
	updated.Activation.Score = clampUnit(latest.Activation.Score + activationDelta)
	updated.Provenance = latest.Provenance
	updated.Provenance.Confidence = clampUnit(latest.Provenance.Confidence + confidenceDelta)

	versions = append(versions, updated)
	s.cards[cardID] = versions
	if s.persistent() {
		if err := s.persistCard(cardID); err != nil {
			return Card{}, err
		}
		if err := s.appendCardJournalEvent("card_touched", cardID, updated.CardType, updated.Version); err != nil {
			return Card{}, err
		}
	}
	return updated, nil
}

func resolveAsOf(versions []Card, asOf *time.Time) *Card {
	if len(versions) == 0 {
		return nil
	}
	if asOf == nil {
		last := versions[len(versions)-1]
		return &last
	}

	// Latest version whose validity window contains as_of.
	for i := len(versions) - 1; i >= 0; i-- {
		card := versions[i]
		if card.ValidFrom != nil && asOf.Before(*card.ValidFrom) {
			continue
		}
		if card.ValidTo != nil && asOf.After(*card.ValidTo) {
			continue
		}
		return &card
	}
	return nil
}

func clampUnit(value float64) float64 {
	return math.Max(0, math.Min(1, value))
}

func (s *Store) collectEdgesForCards(cardIDs []string) []Edge {
	seen := make(map[string]struct{})
	edges := make([]Edge, 0)
	for _, cardID := range cardIDs {
		for _, edgeID := range s.edgesByCard[cardID] {
			if _, ok := seen[edgeID]; ok {
				continue
			}
			edge := s.edges[edgeID]
			edges = append(edges, edge)
			seen[edgeID] = struct{}{}
		}
	}
	return edges
}

// RebuildProjections wipes all card/edge snapshot files, replays the journal
// from scratch, replaces the in-memory state, and rewrites clean snapshots.
// This is the "nuclear option" for recovering from corrupted snapshot files.
func (s *Store) RebuildProjections() error {
	if !s.persistent() {
		return nil
	}
	journalPath := filepath.Join(s.rootDir, "journal.jsonl")
	replayed, err := ReplayFromJournal(journalPath)
	if err != nil {
		return fmt.Errorf("replay journal: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Wipe existing snapshot dirs
	for _, sub := range []string{"cards", "edges"} {
		dir := filepath.Join(s.rootDir, sub)
		entries, _ := os.ReadDir(dir)
		for _, entry := range entries {
			if !entry.IsDir() && filepath.Ext(entry.Name()) == ".json" {
				_ = os.Remove(filepath.Join(dir, entry.Name()))
			}
		}
	}

	// Replace in-memory state
	s.cards = replayed.cards
	s.edges = replayed.edges
	s.edgesByCard = replayed.edgesByCard

	// Rewrite all snapshots
	for cardID := range s.cards {
		if err := s.persistCard(cardID); err != nil {
			return fmt.Errorf("rewrite card %s: %w", cardID, err)
		}
	}
	for edgeID := range s.edges {
		if err := s.persistEdge(edgeID); err != nil {
			return fmt.Errorf("rewrite edge %s: %w", edgeID, err)
		}
	}
	return nil
}

// --- Disk persistence helpers (only active when rootDir != "") ---

// cardFileName maps a card ID to a collision-free filesystem name.
func cardFileName(cardID string) string {
	return hex.EncodeToString([]byte(cardID)) + ".json"
}

func edgeFileName(edgeID string) string {
	return hex.EncodeToString([]byte(edgeID)) + ".json"
}

func legacyMemoryFileName(id string) string {
	r := strings.NewReplacer("/", "__", ":", "_", " ", "_")
	return r.Replace(id) + ".json"
}

// persistCard writes all versions of a card to disk. Caller must hold mu.
func (s *Store) persistCard(cardID string) error {
	versions := s.cards[cardID]
	data, err := json.MarshalIndent(versions, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal card %s: %w", cardID, err)
	}
	path := filepath.Join(s.rootDir, "cards", cardFileName(cardID))
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return err
	}
	legacyPath := filepath.Join(s.rootDir, "cards", legacyMemoryFileName(cardID))
	if legacyPath != path {
		_ = os.Remove(legacyPath)
	}
	return nil
}

// persistEdge writes an edge to disk. Caller must hold mu.
func (s *Store) persistEdge(edgeID string) error {
	edge, ok := s.edges[edgeID]
	if !ok {
		return nil
	}
	data, err := json.MarshalIndent(edge, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal edge %s: %w", edgeID, err)
	}
	path := filepath.Join(s.rootDir, "edges", edgeFileName(edgeID))
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return err
	}
	legacyPath := filepath.Join(s.rootDir, "edges", legacyMemoryFileName(edgeID))
	if legacyPath != path {
		_ = os.Remove(legacyPath)
	}
	return nil
}

// loadFromDisk reads all card and edge JSON files into the in-memory maps.
func (s *Store) loadFromDisk() error {
	cardsDir := filepath.Join(s.rootDir, "cards")
	entries, err := os.ReadDir(cardsDir)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(cardsDir, entry.Name()))
		if err != nil {
			return err
		}
		var versions []Card
		if err := json.Unmarshal(data, &versions); err != nil {
			return fmt.Errorf("unmarshal card file %s: %w", entry.Name(), err)
		}
		if len(versions) == 0 {
			continue
		}
		cardID := versions[0].CardID
		s.cards[cardID] = versions
	}

	edgesDir := filepath.Join(s.rootDir, "edges")
	entries, err = os.ReadDir(edgesDir)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(edgesDir, entry.Name()))
		if err != nil {
			return err
		}
		var edge Edge
		if err := json.Unmarshal(data, &edge); err != nil {
			return fmt.Errorf("unmarshal edge file %s: %w", entry.Name(), err)
		}
		s.edges[edge.EdgeID] = edge
		s.edgesByCard[edge.FromCardID] = append(s.edgesByCard[edge.FromCardID], edge.EdgeID)
		s.edgesByCard[edge.ToCardID] = append(s.edgesByCard[edge.ToCardID], edge.EdgeID)
	}
	return nil
}

// appendCardJournalEvent writes a card event with full snapshot to the journal.
func (s *Store) appendCardJournalEvent(eventType, cardID, cardType string, version int) error {
	if s.journal == nil {
		return nil
	}
	return s.journal.Append(JournalEvent{
		Timestamp:    time.Now().UTC(),
		EventType:    eventType,
		EntityID:     cardID,
		EntityType:   cardType,
		Version:      version,
		CardSnapshot: marshalRaw(s.cards[cardID]),
	})
}

// appendEdgeJournalEvent writes an edge event with full snapshot to the journal.
func (s *Store) appendEdgeJournalEvent(eventType string, edge Edge) error {
	if s.journal == nil {
		return nil
	}
	return s.journal.Append(JournalEvent{
		Timestamp:    time.Now().UTC(),
		EventType:    eventType,
		EntityID:     edge.EdgeID,
		EntityType:   edge.EdgeType,
		EdgeSnapshot: marshalRaw(edge),
	})
}
