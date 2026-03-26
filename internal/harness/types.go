package harness

import "time"

const (
	StepTypeSubmitTask     = "submit_task"
	StepTypeRunTask        = "run_task"
	StepTypeApprovePending = "approve_pending"
	StepTypeSendChat       = "send_chat"

	AssertTaskState           = "task_state"
	AssertSelectedSkill       = "selected_skill"
	AssertApprovalCount       = "approval_count"
	AssertArtifactCount       = "artifact_count"
	AssertObservationCount    = "observation_count"
	AssertArtifactContains    = "artifact_contains"
	AssertAssistantContains   = "assistant_contains"
	AssertFileContains        = "file_contains"
	AssertSessionStateContain = "session_state_contains"
	AssertMemoryCardCount     = "memory_card_count"
	AssertMemoryCardContains  = "memory_card_contains"
	AssertEdgeCount           = "edge_count"
	AssertRecallContains      = "recall_contains"
)

type Scenario struct {
	Name        string      `json:"name"`
	Description string      `json:"description,omitempty"`
	Tags        []string    `json:"tags,omitempty"`
	Fixtures    Fixtures    `json:"fixtures,omitempty"`
	Steps       []Step      `json:"steps"`
	Assertions  []Assertion `json:"assertions,omitempty"`
	Dir         string      `json:"-"`
}

type Fixtures struct {
	SearchResponseFile string `json:"search_response_file,omitempty"`
	GitHubResponseFile string `json:"github_response_file,omitempty"`
	EmailResponseFile  string `json:"email_response_file,omitempty"`
}

type Step struct {
	ID               string            `json:"id"`
	Type             string            `json:"type"`
	SessionID        string            `json:"session_id,omitempty"`
	Message          string            `json:"message,omitempty"`
	Title            string            `json:"title,omitempty"`
	Goal             string            `json:"goal,omitempty"`
	RequestedBy      string            `json:"requested_by,omitempty"`
	Source           string            `json:"source,omitempty"`
	ExecutionProfile string            `json:"execution_profile,omitempty"`
	RequiresApproval bool              `json:"requires_approval,omitempty"`
	SelectedSkill    string            `json:"selected_skill,omitempty"`
	Metadata         map[string]string `json:"metadata,omitempty"`
	TaskRef          string            `json:"task_ref,omitempty"`
	ApprovalRef      string            `json:"approval_ref,omitempty"`
	ApprovedBy       string            `json:"approved_by,omitempty"`
}

type Assertion struct {
	Type      string `json:"type"`
	Step      string `json:"step,omitempty"`
	SessionID string `json:"session_id,omitempty"`
	Field     string `json:"field,omitempty"`
	Status    string `json:"status,omitempty"`
	Path      string `json:"path,omitempty"`
	Equals    string `json:"equals,omitempty"`
	Contains  string `json:"contains,omitempty"`
	Query     string `json:"query,omitempty"`
	Source    string `json:"source,omitempty"`
	Expected  int    `json:"expected,omitempty"`
	Min       int    `json:"min,omitempty"`
}

type RunReport struct {
	ScenarioName        string            `json:"scenario_name"`
	ScenarioDescription string            `json:"scenario_description,omitempty"`
	ScenarioTags        []string          `json:"scenario_tags,omitempty"`
	ScenarioPath        string            `json:"scenario_path"`
	RunDir              string            `json:"run_dir"`
	RuntimeRoot         string            `json:"runtime_root"`
	StartedAt           time.Time         `json:"started_at"`
	FinishedAt          time.Time         `json:"finished_at"`
	Passed              bool              `json:"passed"`
	StepReports         []StepReport      `json:"step_reports"`
	AssertionResults    []AssertionResult `json:"assertion_results,omitempty"`
	Error               string            `json:"error,omitempty"`
}

type StepReport struct {
	ID               string         `json:"id"`
	Type             string         `json:"type"`
	SessionID        string         `json:"session_id,omitempty"`
	TaskID           string         `json:"task_id,omitempty"`
	TaskState        string         `json:"task_state,omitempty"`
	SelectedSkill    string         `json:"selected_skill,omitempty"`
	ApprovalID       string         `json:"approval_id,omitempty"`
	UserContent      string         `json:"user_content,omitempty"`
	AssistantContent string         `json:"assistant_content,omitempty"`
	ArtifactPaths    []string       `json:"artifact_paths,omitempty"`
	ObservationPaths []string       `json:"observation_paths,omitempty"`
	Progress         []StepProgress `json:"progress,omitempty"`
	StartedAt        time.Time      `json:"started_at"`
	FinishedAt       time.Time      `json:"finished_at"`
	Error            string         `json:"error,omitempty"`
}

type StepProgress struct {
	Stage   string `json:"stage"`
	Message string `json:"message"`
}

type AssertionResult struct {
	Type        string `json:"type"`
	Step        string `json:"step,omitempty"`
	Passed      bool   `json:"passed"`
	Description string `json:"description"`
	Details     string `json:"details,omitempty"`
}
