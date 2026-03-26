package recall

import (
	"fmt"
	"sort"
	"strings"

	"mnemosyneos/internal/memory"
)

type Request struct {
	Query   string
	Sources []string
	Limit   int
}

type Hit struct {
	Source        string      `json:"source"`
	CardType      string      `json:"card_type"`
	CardID        string      `json:"card_id"`
	Score         float64     `json:"score"`
	Snippet       string      `json:"snippet,omitempty"`
	MatchedFields []string    `json:"matched_fields,omitempty"`
	Card          memory.Card `json:"card"`
}

type Response struct {
	Query string `json:"query"`
	Hits  []Hit  `json:"hits"`
}

type Service struct {
	store *memory.Store
}

func NewService(store *memory.Store) *Service {
	return &Service{store: store}
}

func (s *Service) Recall(req Request) Response {
	if s == nil || s.store == nil {
		return Response{Query: req.Query, Hits: []Hit{}}
	}

	limit := req.Limit
	if limit <= 0 {
		limit = 10
	}
	query := strings.TrimSpace(req.Query)
	queryTokens := splitQueryTokens(query)
	sourceFilter := normalizeSources(req.Sources)

	hits := make([]Hit, 0)
	for _, card := range s.store.LatestCards() {
		source, ok := sourceForCardType(card.CardType)
		if !ok {
			continue
		}
		if len(sourceFilter) > 0 {
			if _, allowed := sourceFilter[source]; !allowed {
				continue
			}
		}
		score, matchedFields, snippet := scoreCard(card, query, queryTokens)
		if query != "" && score == 0 {
			continue
		}
		hits = append(hits, Hit{
			Source:        source,
			CardType:      card.CardType,
			CardID:        card.CardID,
			Score:         score,
			Snippet:       snippet,
			MatchedFields: matchedFields,
			Card:          card,
		})
	}

	sort.Slice(hits, func(i, j int) bool {
		if hits[i].Score == hits[j].Score {
			if hits[i].Card.CreatedAt.Equal(hits[j].Card.CreatedAt) {
				return hits[i].CardID < hits[j].CardID
			}
			return hits[i].Card.CreatedAt.After(hits[j].Card.CreatedAt)
		}
		return hits[i].Score > hits[j].Score
	})

	if len(hits) > limit {
		hits = hits[:limit]
	}
	return Response{
		Query: query,
		Hits:  hits,
	}
}

func sourceForCardType(cardType string) (string, bool) {
	switch cardType {
	case "web_search", "search_summary", "web_result":
		return "web", true
	case "email_inbox", "email_summary", "email_thread", "email_message":
		return "email", true
	case "github_issue_search", "github_issue_summary", "github_issue":
		return "github", true
	default:
		return "", false
	}
}

func scoreCard(card memory.Card, query string, tokens []string) (float64, []string, string) {
	base := baseWeight(card.CardType)
	if strings.TrimSpace(query) == "" {
		return base, nil, previewCard(card)
	}

	score := 0.0
	matchedFields := make([]string, 0)
	bestSnippet := ""
	for field, value := range flattenContent(card.Content) {
		lower := strings.ToLower(value)
		fieldMatched := false
		for _, token := range tokens {
			if token == "" {
				continue
			}
			if strings.Contains(lower, token) {
				score += 1
				fieldMatched = true
				if bestSnippet == "" {
					bestSnippet = snippetAroundMatch(value, token, 180)
				}
			}
		}
		if fieldMatched {
			matchedFields = append(matchedFields, field)
		}
	}
	if len(matchedFields) == 0 {
		return 0, nil, ""
	}
	return base + score, matchedFields, firstNonEmpty(bestSnippet, previewCard(card))
}

func baseWeight(cardType string) float64 {
	switch cardType {
	case "search_summary", "email_summary", "github_issue_summary":
		return 4
	case "email_thread":
		return 3.5
	case "web_result", "email_message", "github_issue":
		return 3
	case "web_search", "email_inbox", "github_issue_search":
		return 2
	default:
		return 1
	}
}

func flattenContent(content map[string]any) map[string]string {
	out := make(map[string]string)
	for key, value := range content {
		switch typed := value.(type) {
		case string:
			out[key] = typed
		case []string:
			out[key] = strings.Join(typed, " ")
		case []any:
			parts := make([]string, 0, len(typed))
			for _, item := range typed {
				if str, ok := item.(string); ok {
					parts = append(parts, str)
				}
			}
			if len(parts) > 0 {
				out[key] = strings.Join(parts, " ")
			}
		default:
			out[key] = strings.TrimSpace(strings.ReplaceAll(fmt.Sprintf("%v", value), "\n", " "))
		}
	}
	return out
}

func previewCard(card memory.Card) string {
	for _, field := range []string{"summary", "snippet", "subject", "title", "body", "goal", "query"} {
		if value, ok := card.Content[field].(string); ok && strings.TrimSpace(value) != "" {
			return snippetAroundMatch(value, "", 180)
		}
	}
	return card.CardID
}

func splitQueryTokens(query string) []string {
	fields := strings.Fields(strings.ToLower(query))
	out := make([]string, 0, len(fields)+1)
	if query = strings.TrimSpace(strings.ToLower(query)); query != "" {
		out = append(out, query)
	}
	for _, field := range fields {
		if field != "" && !contains(out, field) {
			out = append(out, field)
		}
	}
	return out
}

func normalizeSources(sources []string) map[string]struct{} {
	if len(sources) == 0 {
		return nil
	}
	out := make(map[string]struct{}, len(sources))
	for _, source := range sources {
		source = strings.TrimSpace(strings.ToLower(source))
		if source != "" {
			out[source] = struct{}{}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func snippetAroundMatch(text, token string, max int) string {
	text = strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", " "), "\n", " "))
	if len(text) <= max {
		return text
	}
	if token == "" {
		if max <= 3 {
			return text[:max]
		}
		return text[:max-3] + "..."
	}
	lower := strings.ToLower(text)
	idx := strings.Index(lower, strings.ToLower(token))
	if idx == -1 {
		if max <= 3 {
			return text[:max]
		}
		return text[:max-3] + "..."
	}
	start := idx - max/3
	if start < 0 {
		start = 0
	}
	end := start + max
	if end > len(text) {
		end = len(text)
	}
	snippet := text[start:end]
	if start > 0 {
		snippet = "..." + snippet
	}
	if end < len(text) {
		snippet += "..."
	}
	return snippet
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
