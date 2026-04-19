package harness

import "time"

const (
	StepTypeSubmitTask     = "submit_task"
	StepTypeRunTask        = "run_task"
	StepTypeApprovePending = "approve_pending"
	StepTypeSendChat       = "send_chat"
	StepTypeRestartRuntime = "restart_runtime"
	StepTypeConsolidate    = "consolidate_memory"
	StepTypeSeedMemoryCard = "seed_memory_card"
	StepTypeRequeueTask    = "requeue_task"
	StepTypeScheduleMemory = "schedule_memory_consolidation"
	StepTypeFetchMetrics   = "fetch_metrics"

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
	AssertFileAbsent          = "file_absent"

	AssertWorkingTopicContains           = "working_topic_contains"
	AssertWorkingFocusTaskEquals         = "working_focus_task_equals"
	AssertWorkingPendingQuestionContains = "working_pending_question_contains"
	AssertWorkingPendingActionContains   = "working_pending_action_contains"
	AssertDurableCardCount               = "durable_card_count"
	AssertDurableCardContains            = "durable_card_contains"
	AssertDurableCardStatus              = "durable_card_status"
	AssertDurableCardConfidenceRange     = "durable_card_confidence_range"
	AssertDurableCardScope               = "durable_card_scope"
	AssertDurableCardSupersedes          = "durable_card_supersedes"
	AssertDurableCardVersionEquals       = "durable_card_version_equals"
	AssertDurableCardVersionAtLeast      = "durable_card_version_at_least"
	AssertDurableCardActivationRange     = "durable_card_activation_score_range"
	AssertEdgeExists                     = "edge_exists"
	AssertRecallNotContains              = "recall_not_contains"
	AssertProcedureCount                 = "procedure_count"
	AssertProcedureContains              = "procedure_contains"
	AssertProcedureStepContains          = "procedure_step_contains"
	AssertActionAttemptCount             = "action_attempt_count"
	AssertActionFailureCategory          = "action_failure_category"
	AssertActionReplayed                 = "action_replayed"
	AssertRetrySucceeded                 = "retry_succeeded"
	AssertSchedulerTriggered             = "scheduler_triggered"
	AssertSchedulerSkipReason            = "scheduler_skip_reason"
	AssertMetricsTotalTasksAtLeast       = "metrics_total_tasks_at_least"
	AssertMetricsTotalActionsAtLeast     = "metrics_total_actions_at_least"
	AssertMetricsTotalMemoryAtLeast      = "metrics_total_memory_cards_at_least"
	AssertMetricsActiveSkillsAtLeast     = "metrics_active_skills_at_least"
	AssertMetricsTaskStateAtLeast        = "metrics_task_state_at_least"
	AssertMetricsActionStatusAtLeast     = "metrics_action_status_at_least"
	AssertMetricsMemoryStatusAtLeast     = "metrics_memory_status_at_least"
)

type Scenario struct {
	Name        string      `json:"name"`
	Description string      `json:"description,omitempty"`
	Lane        string      `json:"lane,omitempty"`
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
	// ConfirmFirst controls whether a send_chat step exercises the chat
	// service's always-confirm gate. By default (false) the harness injects
	// Source="intent-confirmation" so the service bypasses the preview and
	// runs the task in a single turn — matches pre-gate scenario expectations.
	// Set to true in scenarios that specifically test the confirmation UX.
	ConfirmFirst bool `json:"confirm_first,omitempty"`
}

type Assertion struct {
	Type          string  `json:"type"`
	Step          string  `json:"step,omitempty"`
	SessionID     string  `json:"session_id,omitempty"`
	Field         string  `json:"field,omitempty"`
	Status        string  `json:"status,omitempty"`
	Path          string  `json:"path,omitempty"`
	Equals        string  `json:"equals,omitempty"`
	Contains      string  `json:"contains,omitempty"`
	Query         string  `json:"query,omitempty"`
	Source        string  `json:"source,omitempty"`
	Expected      int     `json:"expected,omitempty"`
	Min           int     `json:"min,omitempty"`
	MinConfidence float64 `json:"min_confidence,omitempty"`
	MaxConfidence float64 `json:"max_confidence,omitempty"`
}

type RunReport struct {
	ScenarioName        string            `json:"scenario_name"`
	ScenarioDescription string            `json:"scenario_description,omitempty"`
	ScenarioLane        string            `json:"scenario_lane,omitempty"`
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
	ID                       string          `json:"id"`
	Type                     string          `json:"type"`
	SessionID                string          `json:"session_id,omitempty"`
	TaskID                   string          `json:"task_id,omitempty"`
	TaskState                string          `json:"task_state,omitempty"`
	SelectedSkill            string          `json:"selected_skill,omitempty"`
	ActionID                 string          `json:"action_id,omitempty"`
	ActionStatus             string          `json:"action_status,omitempty"`
	ActionFailureCategory    string          `json:"action_failure_category,omitempty"`
	ActionReplayed           bool            `json:"action_replayed,omitempty"`
	ReplayOfActionID         string          `json:"replay_of_action_id,omitempty"`
	ActionAttempts           int             `json:"action_attempts,omitempty"`
	RetryAttempts            int             `json:"retry_attempts,omitempty"`
	RetrySucceeded           bool            `json:"retry_succeeded,omitempty"`
	CardType                 string          `json:"card_type,omitempty"`
	SchedulerTriggered       bool            `json:"scheduler_triggered,omitempty"`
	SchedulerSkipReason      string          `json:"scheduler_skip_reason,omitempty"`
	SchedulerCandidateCount  int             `json:"scheduler_candidate_count,omitempty"`
	Metrics                  MetricsSnapshot `json:"metrics,omitempty"`
	ApprovalID               string          `json:"approval_id,omitempty"`
	UserContent              string          `json:"user_content,omitempty"`
	AssistantContent         string          `json:"assistant_content,omitempty"`
	ArtifactPaths            []string        `json:"artifact_paths,omitempty"`
	ObservationPaths         []string        `json:"observation_paths,omitempty"`
	PromotedCount            int             `json:"promoted_count,omitempty"`
	SupersededCount          int             `json:"superseded_count,omitempty"`
	ArchivedCount            int             `json:"archived_count,omitempty"`
	MemoryFeedbackUpdates    int             `json:"memory_feedback_updates,omitempty"`
	ProcedureFeedbackUpdates int             `json:"procedure_feedback_updates,omitempty"`
	Progress                 []StepProgress  `json:"progress,omitempty"`
	StartedAt                time.Time       `json:"started_at"`
	FinishedAt               time.Time       `json:"finished_at"`
	Error                    string          `json:"error,omitempty"`
}

type MetricsSnapshot struct {
	TotalTasks               int            `json:"total_tasks,omitempty"`
	TasksByState             map[string]int `json:"tasks_by_state,omitempty"`
	TotalActions             int            `json:"total_actions,omitempty"`
	ActionsByStatus          map[string]int `json:"actions_by_status,omitempty"`
	ActionsByFailureCategory map[string]int `json:"actions_by_failure_category,omitempty"`
	TotalMemoryCards         int            `json:"total_memory_cards,omitempty"`
	MemoryByStatus           map[string]int `json:"memory_by_status,omitempty"`
	ActiveSkills             int            `json:"active_skills,omitempty"`
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
