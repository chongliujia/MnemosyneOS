package memory

import "time"

const (
	ScopeSession = "session"
	ScopeUser    = "user"
	ScopeProject = "project"
	ScopeArchive = "archive"

	CardStatusCandidate  = "candidate"
	CardStatusActive     = "active"
	CardStatusStale      = "stale"
	CardStatusSuperseded = "superseded"
	CardStatusArchived   = "archived"
)

type Card struct {
	CardID       string          `json:"card_id"`
	CardType     string          `json:"card_type"`
	Scope        string          `json:"scope,omitempty"`
	CreatedAt    time.Time       `json:"created_at"`
	ValidFrom    *time.Time      `json:"valid_from,omitempty"`
	ValidTo      *time.Time      `json:"valid_to,omitempty"`
	Version      int             `json:"version"`
	PrevVersion  string          `json:"prev_version_id,omitempty"`
	Status       string          `json:"status"`
	Supersedes   string          `json:"supersedes,omitempty"`
	Content      map[string]any  `json:"content"`
	EvidenceRefs []EvidenceRef   `json:"evidence_refs,omitempty"`
	Provenance   Provenance      `json:"provenance"`
	Activation   ActivationState `json:"activation_state"`
}

type EvidenceRef struct {
	CardID  string `json:"card_id"`
	Snippet string `json:"snippet,omitempty"`
	Hash    string `json:"hash,omitempty"`
}

type Provenance struct {
	AgentID    string  `json:"agent_id,omitempty"`
	Source     string  `json:"source,omitempty"`
	Confidence float64 `json:"confidence,omitempty"`
}

type ActivationState struct {
	Score           float64    `json:"score"`
	LastAccessAt    *time.Time `json:"last_access_at,omitempty"`
	LastEvaluatedAt *time.Time `json:"last_evaluated_at,omitempty"`
	DecayPolicy     string     `json:"decay_policy,omitempty"`
}

type Edge struct {
	EdgeID       string        `json:"edge_id"`
	FromCardID   string        `json:"from_card_id"`
	ToCardID     string        `json:"to_card_id"`
	EdgeType     string        `json:"edge_type"`
	Weight       float64       `json:"weight,omitempty"`
	Confidence   float64       `json:"confidence,omitempty"`
	ValidFrom    *time.Time    `json:"valid_from,omitempty"`
	ValidTo      *time.Time    `json:"valid_to,omitempty"`
	EvidenceRefs []EvidenceRef `json:"evidence_refs,omitempty"`
	CreatedAt    time.Time     `json:"created_at"`
}

type CreateCardRequest struct {
	CardID       string           `json:"card_id"`
	CardType     string           `json:"card_type"`
	Scope        string           `json:"scope,omitempty"`
	Status       string           `json:"status,omitempty"`
	Supersedes   string           `json:"supersedes,omitempty"`
	ValidFrom    *time.Time       `json:"valid_from,omitempty"`
	ValidTo      *time.Time       `json:"valid_to,omitempty"`
	Activation   *ActivationState `json:"activation,omitempty"`
	Content      map[string]any   `json:"content"`
	EvidenceRefs []EvidenceRef    `json:"evidence_refs,omitempty"`
	Provenance   Provenance       `json:"provenance"`
}

type UpdateCardRequest struct {
	Content      map[string]any `json:"content"`
	Scope        string         `json:"scope,omitempty"`
	ValidFrom    *time.Time     `json:"valid_from,omitempty"`
	ValidTo      *time.Time     `json:"valid_to,omitempty"`
	Status       string         `json:"status,omitempty"`
	Supersedes   string         `json:"supersedes,omitempty"`
	EvidenceRefs []EvidenceRef  `json:"evidence_refs,omitempty"`
	Provenance   Provenance     `json:"provenance"`
}

type CreateEdgeRequest struct {
	EdgeID       string        `json:"edge_id"`
	FromCardID   string        `json:"from_card_id"`
	ToCardID     string        `json:"to_card_id"`
	EdgeType     string        `json:"edge_type"`
	Weight       float64       `json:"weight,omitempty"`
	Confidence   float64       `json:"confidence,omitempty"`
	ValidFrom    *time.Time    `json:"valid_from,omitempty"`
	ValidTo      *time.Time    `json:"valid_to,omitempty"`
	EvidenceRefs []EvidenceRef `json:"evidence_refs,omitempty"`
}

type QueryRequest struct {
	CardID   string     `json:"card_id,omitempty"`
	CardType string     `json:"card_type,omitempty"`
	Scope    string     `json:"scope,omitempty"`
	Status   string     `json:"status,omitempty"`
	AsOf     *time.Time `json:"as_of,omitempty"`
}

type QueryResponse struct {
	Cards []Card `json:"cards"`
	Edges []Edge `json:"edges,omitempty"`
}

func NormalizeCardStatus(status string) string {
	switch status {
	case CardStatusCandidate, CardStatusActive, CardStatusStale, CardStatusSuperseded, CardStatusArchived:
		return status
	case "":
		return CardStatusActive
	default:
		return ""
	}
}

func IsRecallEligibleStatus(status string) bool {
	return NormalizeCardStatus(status) == CardStatusActive
}
