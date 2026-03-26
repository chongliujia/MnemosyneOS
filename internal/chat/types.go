package chat

import "time"

type Message struct {
	MessageID        string    `json:"message_id"`
	SessionID        string    `json:"session_id,omitempty"`
	Role             string    `json:"role"`
	Content          string    `json:"content"`
	DialogueAct      string    `json:"dialogue_act,omitempty"`
	IntentKind       string    `json:"intent_kind,omitempty"`
	IntentReason     string    `json:"intent_reason,omitempty"`
	IntentConfidence float64   `json:"intent_confidence,omitempty"`
	Stage            string    `json:"stage,omitempty"`
	TaskID           string    `json:"task_id,omitempty"`
	TaskState        string    `json:"task_state,omitempty"`
	SelectedSkill    string    `json:"selected_skill,omitempty"`
	ExecutionProfile string    `json:"execution_profile,omitempty"`
	Links            []Link    `json:"links,omitempty"`
	Actions          []Action  `json:"actions,omitempty"`
	Context          *Context  `json:"context,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
}

type Link struct {
	Label string `json:"label"`
	Href  string `json:"href"`
}

type Action struct {
	Label  string `json:"label"`
	Href   string `json:"href"`
	Method string `json:"method,omitempty"`
}

type Context struct {
	RecentTasks []TaskRef   `json:"recent_tasks,omitempty"`
	RecallHits  []RecallRef `json:"recall_hits,omitempty"`
}

type TaskResultEnvelope struct {
	Outcome          string   `json:"outcome,omitempty"`
	Headline         string   `json:"headline,omitempty"`
	NextAction       string   `json:"next_action,omitempty"`
	FailureReason    string   `json:"failure_reason,omitempty"`
	ArtifactPaths    []string `json:"artifact_paths,omitempty"`
	ObservationPaths []string `json:"observation_paths,omitempty"`
}

type TaskRef struct {
	TaskID        string `json:"task_id"`
	Title         string `json:"title"`
	State         string `json:"state"`
	SelectedSkill string `json:"selected_skill,omitempty"`
}

type RecallRef struct {
	Source   string `json:"source"`
	CardID   string `json:"card_id"`
	CardType string `json:"card_type"`
	Snippet  string `json:"snippet,omitempty"`
}

type SendRequest struct {
	SessionID        string `json:"session_id,omitempty"`
	Message          string `json:"message"`
	RequestedBy      string `json:"requested_by,omitempty"`
	Source           string `json:"source,omitempty"`
	ExecutionProfile string `json:"execution_profile,omitempty"`
	Async            bool   `json:"async,omitempty"`
}

type SendResponse struct {
	UserMessage      Message `json:"user_message"`
	AssistantMessage Message `json:"assistant_message"`
}

type SessionState struct {
	SessionID        string         `json:"session_id"`
	Topic            string         `json:"topic,omitempty"`
	FocusTaskID      string         `json:"focus_task_id,omitempty"`
	PendingQuestion  string         `json:"pending_question,omitempty"`
	PendingAction    string         `json:"pending_action,omitempty"`
	LastUserAct      string         `json:"last_user_act,omitempty"`
	LastAssistantAct string         `json:"last_assistant_act,omitempty"`
	WorkingSet       SessionWorkset `json:"working_set,omitempty"`
	UpdatedAt        time.Time      `json:"updated_at"`
}

type SessionWorkset struct {
	ArtifactPaths []string `json:"artifact_paths,omitempty"`
	RecallCardIDs []string `json:"recall_card_ids,omitempty"`
	SourceRefs    []string `json:"source_refs,omitempty"`
}
