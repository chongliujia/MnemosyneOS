package execution

import "time"

const (
	ActionKindShell     = "shell"
	ActionKindFileRead  = "file-read"
	ActionKindFileWrite = "file-write"

	ActionStatusPending   = "pending"
	ActionStatusRunning   = "running"
	ActionStatusCompleted = "completed"
	ActionStatusFailed    = "failed"

	ActionFailureTimeout     = "timeout"
	ActionFailureProcessExit = "process_exit"
	ActionFailureIO          = "io_error"
	ActionFailureExecution   = "execution_error"
)

type ActionRecord struct {
	ActionID         string            `json:"action_id"`
	TaskID           string            `json:"task_id,omitempty"`
	Kind             string            `json:"kind"`
	Status           string            `json:"status"`
	ExecutionProfile string            `json:"execution_profile"`
	Command          string            `json:"command,omitempty"`
	Args             []string          `json:"args,omitempty"`
	Path             string            `json:"path,omitempty"`
	Workdir          string            `json:"workdir,omitempty"`
	ChangedFiles     []string          `json:"changed_files,omitempty"`
	Metadata         map[string]string `json:"metadata,omitempty"`
	Attempt          int               `json:"attempt,omitempty"`
	IdempotencyKey   string            `json:"idempotency_key,omitempty"`
	Retryable        bool              `json:"retryable,omitempty"`
	FailureCategory  string            `json:"failure_category,omitempty"`
	Stdout           string            `json:"stdout,omitempty"`
	Stderr           string            `json:"stderr,omitempty"`
	ExitCode         int               `json:"exit_code,omitempty"`
	Error            string            `json:"error,omitempty"`
	StartedAt        time.Time         `json:"started_at"`
	FinishedAt       *time.Time        `json:"finished_at,omitempty"`
}

type ShellActionRequest struct {
	TaskID           string            `json:"task_id,omitempty"`
	Command          string            `json:"command"`
	Args             []string          `json:"args,omitempty"`
	Workdir          string            `json:"workdir,omitempty"`
	TimeoutMS        int               `json:"timeout_ms,omitempty"`
	Attempt          int               `json:"attempt,omitempty"`
	IdempotencyKey   string            `json:"idempotency_key,omitempty"`
	ExecutionProfile string            `json:"execution_profile,omitempty"`
	ApprovalToken    string            `json:"approval_token,omitempty"`
	Metadata         map[string]string `json:"metadata,omitempty"`
}

type FileReadActionRequest struct {
	TaskID           string            `json:"task_id,omitempty"`
	Path             string            `json:"path"`
	Attempt          int               `json:"attempt,omitempty"`
	IdempotencyKey   string            `json:"idempotency_key,omitempty"`
	ExecutionProfile string            `json:"execution_profile,omitempty"`
	ApprovalToken    string            `json:"approval_token,omitempty"`
	Metadata         map[string]string `json:"metadata,omitempty"`
}

type FileWriteActionRequest struct {
	TaskID           string            `json:"task_id,omitempty"`
	Path             string            `json:"path"`
	Content          string            `json:"content"`
	CreateParents    bool              `json:"create_parents,omitempty"`
	Attempt          int               `json:"attempt,omitempty"`
	IdempotencyKey   string            `json:"idempotency_key,omitempty"`
	ExecutionProfile string            `json:"execution_profile,omitempty"`
	ApprovalToken    string            `json:"approval_token,omitempty"`
	Metadata         map[string]string `json:"metadata,omitempty"`
}
