package memory

import (
	"strings"
)

// Conflict represents two cards that may contradict each other.
type Conflict struct {
	CardA       string  `json:"card_a"`
	CardB       string  `json:"card_b"`
	Topic       string  `json:"topic"`
	Reason      string  `json:"reason"`
	ScoreDelta  float64 `json:"score_delta"`
}

// ConflictDetectionRequest configures what to scan for conflicts.
type ConflictDetectionRequest struct {
	Scope    string
	CardType string // empty = all types
}

// ConflictDetectionResult holds the outcome.
type ConflictDetectionResult struct {
	Examined  int        `json:"examined"`
	Conflicts []Conflict `json:"conflicts"`
}

// DetectConflicts finds pairs of active/candidate fact cards with the same topic
// but different assertions. Two cards conflict when they share a topic key
// but have divergent content in key fields (answer, claim, summary).
func (s *Store) DetectConflicts(req ConflictDetectionRequest) ConflictDetectionResult {
	scope := strings.TrimSpace(req.Scope)
	cardType := strings.TrimSpace(req.CardType)
	if cardType == "" {
		cardType = "fact"
	}

	active := s.Query(QueryRequest{
		CardType: cardType,
		Scope:    scope,
		Status:   CardStatusActive,
	}).Cards
	candidates := s.Query(QueryRequest{
		CardType: cardType,
		Scope:    scope,
		Status:   CardStatusCandidate,
	}).Cards

	all := append(active, candidates...)
	result := ConflictDetectionResult{Examined: len(all)}

	type topicEntry struct {
		card  Card
		topic string
	}

	byTopic := make(map[string][]topicEntry)
	for _, card := range all {
		topic := extractTopic(card)
		if topic == "" {
			continue
		}
		byTopic[topic] = append(byTopic[topic], topicEntry{card: card, topic: topic})
	}

	seen := make(map[string]bool)
	for topic, entries := range byTopic {
		for i := 0; i < len(entries); i++ {
			for j := i + 1; j < len(entries); j++ {
				a := entries[i].card
				b := entries[j].card
				pairKey := a.CardID + "|" + b.CardID
				if seen[pairKey] {
					continue
				}

				if reason := contentConflict(a, b); reason != "" {
					seen[pairKey] = true
					result.Conflicts = append(result.Conflicts, Conflict{
						CardA:      a.CardID,
						CardB:      b.CardID,
						Topic:      topic,
						Reason:     reason,
						ScoreDelta: a.Activation.Score - b.Activation.Score,
					})
				}
			}
		}
	}

	return result
}

// contentConflict checks whether two cards on the same topic have divergent assertions.
func contentConflict(a, b Card) string {
	for _, key := range []string{"answer", "claim", "summary", "body"} {
		valA, okA := a.Content[key].(string)
		valB, okB := b.Content[key].(string)
		if !okA || !okB {
			continue
		}
		valA = strings.TrimSpace(strings.ToLower(valA))
		valB = strings.TrimSpace(strings.ToLower(valB))
		if valA == "" || valB == "" {
			continue
		}
		if valA != valB {
			return "divergent " + key + ": " + truncate(valA, 80) + " vs " + truncate(valB, 80)
		}
	}
	return ""
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}
