package approval

import "time"

const (
	StatusPending  = "pending"
	StatusApproved = "approved"
	StatusDenied   = "denied"
	StatusConsumed = "consumed"
)

type Request struct {
	ApprovalID       string            `json:"approval_id"`
	TaskID           string            `json:"task_id,omitempty"`
	ExecutionProfile string            `json:"execution_profile"`
	ActionKind       string            `json:"action_kind"`
	Summary          string            `json:"summary"`
	RequestedBy      string            `json:"requested_by,omitempty"`
	ApprovedBy       string            `json:"approved_by,omitempty"`
	DeniedBy         string            `json:"denied_by,omitempty"`
	DeniedReason     string            `json:"denied_reason,omitempty"`
	Status           string            `json:"status"`
	ApprovalToken    string            `json:"approval_token,omitempty"`
	Metadata         map[string]string `json:"metadata,omitempty"`
	CreatedAt        time.Time         `json:"created_at"`
	UpdatedAt        time.Time         `json:"updated_at"`
	ExpiresAt        *time.Time        `json:"expires_at,omitempty"`
	UsedAt           *time.Time        `json:"used_at,omitempty"`
}

type CreateRequest struct {
	TaskID           string            `json:"task_id,omitempty"`
	ExecutionProfile string            `json:"execution_profile"`
	ActionKind       string            `json:"action_kind"`
	Summary          string            `json:"summary"`
	RequestedBy      string            `json:"requested_by,omitempty"`
	Metadata         map[string]string `json:"metadata,omitempty"`
}
