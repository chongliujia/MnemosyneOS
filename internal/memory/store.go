package memory

import (
	"errors"
	"fmt"
	"sort"
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
}

func NewStore() *Store {
	return &Store{
		cards:       make(map[string][]Card),
		edges:       make(map[string]Edge),
		edgesByCard: make(map[string][]string),
	}
}

func (s *Store) CreateCard(req CreateCardRequest) (Card, error) {
	if req.CardID == "" || req.CardType == "" {
		return Card{}, fmt.Errorf("%w: card_id and card_type are required", ErrInvalidArgument)
	}

	now := time.Now().UTC()
	card := Card{
		CardID:       req.CardID,
		CardType:     req.CardType,
		CreatedAt:    now,
		ValidFrom:    req.ValidFrom,
		ValidTo:      req.ValidTo,
		Version:      1,
		Status:       "active",
		Content:      req.Content,
		EvidenceRefs: req.EvidenceRefs,
		Provenance:   req.Provenance,
		Activation: ActivationState{
			Score: 1.0,
		},
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.cards[req.CardID]; ok {
		return Card{}, ErrAlreadyExists
	}

	s.cards[req.CardID] = []Card{card}
	return card, nil
}

func (s *Store) UpdateCard(cardID string, req UpdateCardRequest) (Card, error) {
	if cardID == "" {
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
	updated := Card{
		CardID:       latest.CardID,
		CardType:     latest.CardType,
		CreatedAt:    latest.CreatedAt,
		ValidFrom:    req.ValidFrom,
		ValidTo:      req.ValidTo,
		Version:      latest.Version + 1,
		PrevVersion:  latest.CardID + "#v" + fmt.Sprintf("%d", latest.Version),
		Status:       latest.Status,
		Content:      latest.Content,
		EvidenceRefs: latest.EvidenceRefs,
		Provenance:   latest.Provenance,
		Activation: ActivationState{
			Score:        latest.Activation.Score,
			LastAccessAt: &now,
			DecayPolicy:  latest.Activation.DecayPolicy,
		},
	}

	if req.Content != nil {
		updated.Content = req.Content
	}
	if req.Status != "" {
		updated.Status = req.Status
	}
	if req.EvidenceRefs != nil {
		updated.EvidenceRefs = req.EvidenceRefs
	}
	if req.Provenance.AgentID != "" || req.Provenance.Source != "" || req.Provenance.Confidence != 0 {
		updated.Provenance = req.Provenance
	}

	versions = append(versions, updated)
	s.cards[cardID] = versions
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
