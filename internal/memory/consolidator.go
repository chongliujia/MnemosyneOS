package memory

import (
	"sort"
	"strings"
)

type ConsolidateRequest struct {
	CardType         string
	Scope            string
	Limit            int
	ArchiveRemaining bool
}

type ConsolidateResult struct {
	Examined        int      `json:"examined"`
	Promoted        int      `json:"promoted"`
	Superseded      int      `json:"superseded"`
	Archived        int      `json:"archived"`
	PromotedCards   []string `json:"promoted_cards,omitempty"`
	SupersededCards []string `json:"superseded_cards,omitempty"`
	ArchivedCards   []string `json:"archived_cards,omitempty"`
}

type Consolidator struct {
	store *Store
}

func NewConsolidator(store *Store) *Consolidator {
	return &Consolidator{store: store}
}

func (c *Consolidator) PromoteCandidates(req ConsolidateRequest) (ConsolidateResult, error) {
	if c == nil || c.store == nil {
		return ConsolidateResult{}, nil
	}

	resp := c.store.Query(QueryRequest{
		CardType: strings.TrimSpace(req.CardType),
		Scope:    strings.TrimSpace(req.Scope),
		Status:   CardStatusCandidate,
	})
	cards := resp.Cards
	sort.Slice(cards, func(i, j int) bool {
		if cards[i].CreatedAt.Equal(cards[j].CreatedAt) {
			return cards[i].CardID < cards[j].CardID
		}
		return cards[i].CreatedAt.Before(cards[j].CreatedAt)
	})

	result := ConsolidateResult{Examined: len(cards)}
	limit := req.Limit
	if limit <= 0 || limit > len(cards) {
		limit = len(cards)
	}
	for i := 0; i < limit; i++ {
		card := cards[i]
		if _, err := c.store.UpdateCard(card.CardID, UpdateCardRequest{
			Status:       CardStatusActive,
			Scope:        card.Scope,
			Content:      card.Content,
			EvidenceRefs: card.EvidenceRefs,
			Provenance:   card.Provenance,
		}); err != nil {
			return result, err
		}
		result.Promoted++
		result.PromotedCards = append(result.PromotedCards, card.CardID)
		if target := strings.TrimSpace(card.Supersedes); target != "" {
			if _, err := c.store.UpdateCard(target, UpdateCardRequest{
				Status: CardStatusSuperseded,
			}); err == nil {
				result.Superseded++
				result.SupersededCards = append(result.SupersededCards, target)
			}
		}
	}
	if req.ArchiveRemaining && limit < len(cards) {
		for i := limit; i < len(cards); i++ {
			card := cards[i]
			if _, err := c.store.UpdateCard(card.CardID, UpdateCardRequest{
				Status: CardStatusArchived,
			}); err != nil {
				return result, err
			}
			result.Archived++
			result.ArchivedCards = append(result.ArchivedCards, card.CardID)
		}
	}
	return result, nil
}
