package airuntime

import "time"

const (
	TaskStateInbox            = "inbox"
	TaskStatePlanned          = "planned"
	TaskStateActive           = "active"
	TaskStateBlocked          = "blocked"
	TaskStateAwaitingApproval = "awaiting_approval"
	TaskStateDone             = "done"
	TaskStateFailed           = "failed"
	TaskStateArchived         = "archived"
)

type RuntimeState struct {
	RuntimeID        string    `json:"runtime_id"`
	ActiveUserID     string    `json:"active_user_id"`
	SessionID        *string   `json:"session_id,omitempty"`
	Status           string    `json:"status"`
	ActiveTaskID     *string   `json:"active_task_id,omitempty"`
	ExecutionProfile string    `json:"execution_profile"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type Task struct {
	TaskID           string            `json:"task_id"`
	Title            string            `json:"title"`
	Goal             string            `json:"goal"`
	State            string            `json:"state"`
	CreatedAt        time.Time         `json:"created_at"`
	UpdatedAt        time.Time         `json:"updated_at"`
	RequestedBy      string            `json:"requested_by,omitempty"`
	Source           string            `json:"source,omitempty"`
	ExecutionProfile string            `json:"execution_profile"`
	RequiresApproval bool              `json:"requires_approval"`
	SelectedSkill    string            `json:"selected_skill,omitempty"`
	NextAction       string            `json:"next_action,omitempty"`
	FailureReason    string            `json:"failure_reason,omitempty"`
	Metadata         map[string]string `json:"metadata,omitempty"`
}

type CreateTaskRequest struct {
	Title            string            `json:"title"`
	Goal             string            `json:"goal"`
	RequestedBy      string            `json:"requested_by,omitempty"`
	Source           string            `json:"source,omitempty"`
	ExecutionProfile string            `json:"execution_profile,omitempty"`
	RequiresApproval bool              `json:"requires_approval,omitempty"`
	SelectedSkill    string            `json:"selected_skill,omitempty"`
	Metadata         map[string]string `json:"metadata,omitempty"`
}

type ApproveTaskRequest struct {
	ApprovedBy string `json:"approved_by,omitempty"`
}

type DenyTaskRequest struct {
	DeniedBy string `json:"denied_by,omitempty"`
	Reason   string `json:"reason,omitempty"`
}
