package memory

import "time"

type Card struct {
	CardID       string          `json:"card_id"`
	CardType     string          `json:"card_type"`
	CreatedAt    time.Time       `json:"created_at"`
	ValidFrom    *time.Time      `json:"valid_from,omitempty"`
	ValidTo      *time.Time      `json:"valid_to,omitempty"`
	Version      int             `json:"version"`
	PrevVersion  string          `json:"prev_version_id,omitempty"`
	Status       string          `json:"status"`
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
	Score        float64    `json:"score"`
	LastAccessAt *time.Time `json:"last_access_at,omitempty"`
	DecayPolicy  string     `json:"decay_policy,omitempty"`
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
	CardID       string         `json:"card_id"`
	CardType     string         `json:"card_type"`
	ValidFrom    *time.Time     `json:"valid_from,omitempty"`
	ValidTo      *time.Time     `json:"valid_to,omitempty"`
	Content      map[string]any `json:"content"`
	EvidenceRefs []EvidenceRef  `json:"evidence_refs,omitempty"`
	Provenance   Provenance     `json:"provenance"`
}

type UpdateCardRequest struct {
	Content      map[string]any `json:"content"`
	ValidFrom    *time.Time     `json:"valid_from,omitempty"`
	ValidTo      *time.Time     `json:"valid_to,omitempty"`
	Status       string         `json:"status,omitempty"`
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
	AsOf     *time.Time `json:"as_of,omitempty"`
}

type QueryResponse struct {
	Cards []Card `json:"cards"`
	Edges []Edge `json:"edges,omitempty"`
}
