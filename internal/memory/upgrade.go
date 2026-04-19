package memory

import (
	"crypto/sha1"
	"fmt"
	"sort"
	"strings"
)

// UpgradeRequest configures how event cards are consolidated into fact cards.
type UpgradeRequest struct {
	Scope           string
	MinOccurrences  int     // minimum event cards needed to warrant a fact (default 2)
	MinConfidence   float64 // minimum average confidence to upgrade (default 0.5)
	ArchiveSources  bool    // archive source event cards after upgrade
}

// UpgradeResult reports what the upgrade engine produced.
type UpgradeResult struct {
	EventsExamined  int      `json:"events_examined"`
	ClustersFound   int      `json:"clusters_found"`
	FactsCreated    int      `json:"facts_created"`
	EventsArchived  int      `json:"events_archived"`
	FactCardIDs     []string `json:"fact_card_ids,omitempty"`
}

// UpgradeEventsToFacts scans active "event" cards, clusters them by topic
// (content key overlap), and creates "fact" candidate cards when enough events
// converge on the same conclusion. This is the episodic → semantic bridge.
func (s *Store) UpgradeEventsToFacts(req UpgradeRequest) (UpgradeResult, error) {
	minOcc := req.MinOccurrences
	if minOcc <= 0 {
		minOcc = 2
	}
	minConf := req.MinConfidence
	if minConf <= 0 {
		minConf = 0.5
	}
	scope := strings.TrimSpace(req.Scope)
	if scope == "" {
		scope = ScopeProject
	}

	resp := s.Query(QueryRequest{
		CardType: "event",
		Status:   CardStatusActive,
		Scope:    scope,
	})

	result := UpgradeResult{EventsExamined: len(resp.Cards)}
	if len(resp.Cards) < minOcc {
		return result, nil
	}

	clusters := clusterEventCards(resp.Cards)
	result.ClustersFound = len(clusters)

	for topic, cards := range clusters {
		if len(cards) < minOcc {
			continue
		}

		avgConf := averageConfidence(cards)
		if avgConf < minConf {
			continue
		}

		factCardID := factCardIDFromTopic(topic)
		if existing := s.Query(QueryRequest{CardID: factCardID}); len(existing.Cards) > 0 {
			continue
		}

		evidenceRefs := make([]EvidenceRef, 0, len(cards))
		for _, c := range cards {
			evidenceRefs = append(evidenceRefs, EvidenceRef{
				CardID:  c.CardID,
				Snippet: extractSnippet(c),
			})
		}

		factContent := synthesizeFactContent(topic, cards)
		_, err := s.CreateCard(CreateCardRequest{
			CardID:       factCardID,
			CardType:     "fact",
			Scope:        scope,
			Status:       CardStatusCandidate,
			Content:      factContent,
			EvidenceRefs: evidenceRefs,
			Provenance: Provenance{
				Source:     "event-fact-upgrade",
				Confidence: avgConf,
			},
		})
		if err != nil {
			return result, fmt.Errorf("create fact card %s: %w", factCardID, err)
		}
		result.FactsCreated++
		result.FactCardIDs = append(result.FactCardIDs, factCardID)

		// Link fact to source events via edges
		for _, c := range cards {
			edgeID := fmt.Sprintf("edge:evidence:%s->%s", c.CardID, factCardID)
			_, _ = s.CreateEdge(CreateEdgeRequest{
				EdgeID:     edgeID,
				FromCardID: c.CardID,
				ToCardID:   factCardID,
				EdgeType:   "evidence",
				Weight:     1.0,
				Confidence: c.Provenance.Confidence,
			})
		}

		if req.ArchiveSources {
			for _, c := range cards {
				if _, err := s.UpdateCard(c.CardID, UpdateCardRequest{
					Status: CardStatusArchived,
				}); err == nil {
					result.EventsArchived++
				}
			}
		}
	}

	return result, nil
}

// clusterEventCards groups event cards by topic signature.
// Topic is derived from content keys: "subject", "topic", "summary", "query".
func clusterEventCards(cards []Card) map[string][]Card {
	clusters := make(map[string][]Card)
	for _, card := range cards {
		topic := extractTopic(card)
		if topic == "" {
			continue
		}
		clusters[topic] = append(clusters[topic], card)
	}
	return clusters
}

// extractTopic derives a normalized topic key from a card's content.
func extractTopic(card Card) string {
	for _, key := range []string{"topic", "subject", "summary", "query", "claim"} {
		if val, ok := card.Content[key].(string); ok {
			normalized := normalizeTopic(val)
			if normalized != "" {
				return normalized
			}
		}
	}
	return ""
}

func normalizeTopic(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	words := strings.Fields(s)
	sort.Strings(words)
	return strings.Join(words, " ")
}

func extractSnippet(card Card) string {
	for _, key := range []string{"summary", "snippet", "body", "claim", "query"} {
		if val, ok := card.Content[key].(string); ok && strings.TrimSpace(val) != "" {
			if len(val) > 200 {
				return val[:197] + "..."
			}
			return val
		}
	}
	return card.CardID
}

func averageConfidence(cards []Card) float64 {
	if len(cards) == 0 {
		return 0
	}
	sum := 0.0
	count := 0
	for _, c := range cards {
		if c.Provenance.Confidence > 0 {
			sum += c.Provenance.Confidence
			count++
		}
	}
	if count == 0 {
		return 0.7 // default if no confidence values set
	}
	return sum / float64(count)
}

func factCardIDFromTopic(topic string) string {
	h := sha1.Sum([]byte(topic))
	return fmt.Sprintf("fact:upgraded:%x", h[:8])
}

// synthesizeFactContent merges the most common assertions from source events.
func synthesizeFactContent(topic string, cards []Card) map[string]any {
	content := map[string]any{
		"topic":          topic,
		"source_count":   len(cards),
		"derived_from":   "event-fact-upgrade",
	}

	for _, key := range []string{"summary", "claim", "body", "answer"} {
		for _, card := range cards {
			if val, ok := card.Content[key].(string); ok && strings.TrimSpace(val) != "" {
				content[key] = val
				break
			}
		}
	}

	sourceIDs := make([]any, 0, len(cards))
	for _, c := range cards {
		sourceIDs = append(sourceIDs, c.CardID)
	}
	content["source_event_ids"] = sourceIDs

	return content
}
