package chat

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"mnemosyneos/internal/model"
)

const routingSystemPrompt = `Route the user message for an AgentOS chat turn.

Return strict JSON only:
{"dialogue_act":"<act>","intent_kind":"<kind>","skill":"<skill>","reason":"<reason>","confidence":<0.0-1.0>,"target_scope":"<scope>","risk_level":"<risk>","needs_confirmation":<true|false>,"candidate_skills":["<skill>"]}

dialogue_act: chat | new_task | answer_question | confirm | deny | continue_task | clarify | switch_topic
intent_kind: direct_reply | task_request
skill (only when intent_kind=task_request): task-plan | web-search | github-issue-search | email-inbox | memory-consolidate | file-read | file-edit | shell-command
target_scope: session | focused_task | new_task
risk_level: low | medium | high

CRITICAL RULES for intent_kind:

direct_reply — use this for:
- Greetings, small talk, social messages
- Questions ABOUT the system (e.g. "where is my project?", "what can you do?", "how does memory work?")
- Questions asking for information, explanations, or opinions
- Follow-ups confirming or clarifying within a conversation thread
- Short affirmative replies like "需要", "继续", "好的" when responding to assistant questions
- ANY message phrased as a question (contains ?, 吗, 嗎, 呢, what, where, how, why, 什么, 哪里, 怎么, etc.) UNLESS it explicitly asks you to perform an action

task_request — use this ONLY for:
- Explicit commands to DO something: "search for X", "run this command", "create a file", "check my email"
- Messages with clear action verbs directed at the system: "帮我搜索", "执行", "编辑文件", "发邮件"
- Requests that require the system to use external tools or modify state

Keep direct_reply (not task_request) for read-only workspace questions such as "where is folder X", "查看一个叫 Y 的目录位置", "which path is Z" — the chat agent answers those with list/search tools without a task plan.

When uncertain, prefer direct_reply. The cost of incorrectly creating a task is much higher than answering directly.`

type RouteDecision struct {
	DialogueAct       string   `json:"dialogue_act"`
	IntentKind        string   `json:"intent_kind"`
	Skill             string   `json:"skill,omitempty"`
	Reason            string   `json:"reason,omitempty"`
	Confidence        float64  `json:"confidence,omitempty"`
	TargetScope       string   `json:"target_scope,omitempty"`
	RiskLevel         string   `json:"risk_level,omitempty"`
	NeedsConfirmation bool     `json:"needs_confirmation,omitempty"`
	CandidateSkills   []string `json:"candidate_skills,omitempty"`
}

type RouteAgent struct {
	model model.TextGateway
}

func NewRouteAgent(textModel model.TextGateway) *RouteAgent {
	return &RouteAgent{model: textModel}
}

func (a *RouteAgent) Decide(message, conversationContext string, state SessionState) RouteDecision {
	message = strings.TrimSpace(message)
	if message == "" {
		return RouteDecision{
			DialogueAct: DialogueActChat,
			IntentKind:  IntentKindDirect,
			Reason:      "empty message",
			Confidence:  1,
			TargetScope: "session",
			RiskLevel:   "low",
		}
	}

	heuristic := heuristicRouteDecision(message, conversationContext, state)

	if a == nil || a.model == nil {
		return heuristic
	}

	modelResult, err := a.modelDecision(message, conversationContext, state)
	if err != nil || modelResult.DialogueAct == "" || modelResult.IntentKind == "" {
		return heuristic
	}

	// When the heuristic is very confident this is a direct reply (e.g. a
	// question), only let the model override if IT is also confident about
	// task routing. This prevents the model from aggressively turning
	// informational questions into tasks.
	if heuristic.IntentKind == IntentKindDirect && heuristic.Confidence >= 0.85 &&
		modelResult.IntentKind == IntentKindTask && modelResult.Confidence < 0.85 {
		return heuristic
	}
	if heuristic.TargetScope == "focused_task" &&
		(heuristic.DialogueAct == DialogueActContinueTask || heuristic.DialogueAct == DialogueActAnswer || heuristic.DialogueAct == DialogueActConfirm) &&
		modelResult.TargetScope != "focused_task" {
		return heuristic
	}

	// Do not let routing LLM turn “where is this directory?” into a task plan;
	// the user expects immediate paths from list_directory / search_files.
	if looksLikeReadOnlyFilesystemLookup(message) &&
		heuristic.IntentKind == IntentKindDirect &&
		modelResult.IntentKind == IntentKindTask {
		return heuristic
	}

	if looksLikeListChoiceReply(message) && assistantOfferedEnumeratedChoice(conversationContext) &&
		heuristic.IntentKind == IntentKindDirect &&
		modelResult.IntentKind == IntentKindTask {
		return heuristic
	}

	if looksLikeExplicitConcreteTask(message) && inferRiskLevel(modelResult.Skill, modelResult.IntentKind) != "high" {
		modelResult.NeedsConfirmation = false
	}

	return modelResult
}

func (a *RouteAgent) modelDecision(message, conversationContext string, state SessionState) (RouteDecision, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()

	userPrompt := "User message:\n" + message
	if strings.TrimSpace(conversationContext) != "" {
		userPrompt += "\n\nRecent conversation context:\n" + conversationContext
	}
	if summary := summarizeSessionState(state); strings.TrimSpace(summary) != "" {
		userPrompt += "\n\nSession state:\n" + summary
	}

	resp, err := a.model.GenerateText(ctx, model.TextRequest{
		SystemPrompt: routingSystemPrompt,
		UserPrompt:   userPrompt,
		MaxTokens:    140,
		Temperature:  0,
		Profile:      model.ProfileRouting,
	})
	if err != nil {
		return RouteDecision{}, err
	}

	var decision RouteDecision
	if err := json.Unmarshal([]byte(strings.TrimSpace(resp.Text)), &decision); err != nil {
		return RouteDecision{}, err
	}
	if _, ok := validDialogueActs[decision.DialogueAct]; !ok {
		return RouteDecision{}, err
	}
	if decision.IntentKind != IntentKindDirect && decision.IntentKind != IntentKindTask {
		return RouteDecision{}, err
	}
	if decision.TargetScope == "" {
		decision.TargetScope = "session"
	}
	if decision.RiskLevel == "" {
		decision.RiskLevel = "low"
	}
	if decision.Skill != "" {
		if _, ok := validSkills[decision.Skill]; !ok {
			return RouteDecision{}, err
		}
	}
	for _, skill := range decision.CandidateSkills {
		if _, ok := validSkills[skill]; !ok {
			return RouteDecision{}, err
		}
	}
	return decision, nil
}

func heuristicRouteDecision(message, conversationContext string, state SessionState) RouteDecision {
	dialogue, _ := heuristicDialogueDecision(message, conversationContext, state)
	intent, _ := heuristicIntentDecision(message, conversationContext)
	candidates := prefilterCandidateSkills(message, conversationContext, state)
	skill, _ := heuristicSkillDecisionWithCandidates(message, conversationContext, candidates)

	if dialogue.Act == "" {
		dialogue = DialogueDecision{Act: DialogueActNewTask, Reason: "default new task heuristic", Confidence: 0.55}
	}
	if intent.Kind == "" {
		if isFollowupDialogueAct(dialogue.Act) || dialogue.Act == DialogueActChat {
			intent = IntentDecision{Kind: IntentKindDirect, Reason: "dialogue act implies direct reply", Confidence: dialogue.Confidence}
		} else {
			intent = IntentDecision{Kind: IntentKindTask, Reason: "dialogue act implies task request", Confidence: dialogue.Confidence}
		}
	}
	if intent.Kind == IntentKindDirect {
		skill = SkillDecision{}
	}
	if intent.Kind == IntentKindTask && skill.Skill == "" {
		skill = SkillDecision{Skill: SkillTaskPlan, Reason: "default planning skill", Confidence: 0.55}
	}

	targetScope := inferTargetScope(dialogue.Act, intent.Kind, state)
	riskLevel := inferRiskLevel(skill.Skill, intent.Kind)
	needsConfirmation := inferNeedsConfirmation(message, dialogue.Act, intent.Kind, skill.Skill, riskLevel, state)

	return RouteDecision{
		DialogueAct:       dialogue.Act,
		IntentKind:        intent.Kind,
		Skill:             skill.Skill,
		Reason:            firstNonEmpty(dialogue.Reason, intent.Reason, skill.Reason),
		Confidence:        maxFloat(dialogue.Confidence, intent.Confidence, skill.Confidence),
		TargetScope:       targetScope,
		RiskLevel:         riskLevel,
		NeedsConfirmation: needsConfirmation,
		CandidateSkills:   candidates,
	}
}

func inferTargetScope(dialogueAct, intentKind string, state SessionState) string {
	switch dialogueAct {
	case DialogueActContinueTask, DialogueActConfirm, DialogueActAnswer:
		if strings.TrimSpace(state.FocusTaskID) != "" {
			return "focused_task"
		}
	}
	if intentKind == IntentKindDirect {
		return "session"
	}
	return "new_task"
}

func inferRiskLevel(skill, intentKind string) string {
	if intentKind != IntentKindTask {
		return "low"
	}
	switch strings.TrimSpace(skill) {
	case SkillFileEdit, SkillShellCommand:
		return "high"
	case SkillWebSearch, SkillGitHubIssueSearch, SkillEmailInbox, SkillFileRead:
		return "medium"
	default:
		return "low"
	}
}

// looksLikeExplicitConcreteTask is true when the user already stated a clear
// action + object (e.g. create a named file). Those should not be blocked by
// the generic "short task-plan → confirm" heuristic or by routing JSON alone.
func looksLikeExplicitConcreteTask(msg string) bool {
	msg = strings.TrimSpace(msg)
	if msg == "" {
		return false
	}
	lower := strings.ToLower(msg)

	switch {
	case strings.HasPrefix(lower, "touch ") && len(msg) > 6:
		return true
	case strings.HasPrefix(lower, "mkdir ") && len(msg) > 6:
		return true
	case (strings.Contains(lower, "create") || strings.Contains(lower, "make")) && strings.Contains(lower, "file"):
		return true
	case strings.Contains(lower, "write ") && strings.Contains(lower, " to "):
		return true
	}

	if strings.Contains(msg, "文件") {
		for _, verb := range []string{"创建", "新建", "生成", "写入", "删除", "删掉"} {
			if strings.Contains(msg, verb) {
				return true
			}
		}
		if strings.Contains(msg, "名叫") || strings.Contains(msg, "名为") || strings.Contains(msg, "叫做") {
			return true
		}
	}

	// Path-like or explicit shell one-liner (exclude bare path picks; those are
	// read-only lookups answered in chat, not concrete execution requests).
	if pathPickerFollowup(msg) {
		return false
	}
	if strings.Contains(msg, "/") && len([]rune(msg)) >= 4 {
		return true
	}

	return false
}

func inferNeedsConfirmation(message, dialogueAct, intentKind, skill, riskLevel string, state SessionState) bool {
	if intentKind != IntentKindTask {
		return false
	}
	switch dialogueAct {
	case DialogueActContinueTask, DialogueActConfirm, DialogueActAnswer:
		return false
	}
	if strings.TrimSpace(state.PendingAction) == "confirm_task_intent" {
		return false
	}
	if riskLevel == "high" {
		text := strings.TrimSpace(message)
		// Concrete file-create/edit goals already name the target; skip the
		// generic chat "confirm task intent" step — file-edit still hits the
		// execution plane and root approval when required.
		if strings.TrimSpace(skill) == SkillFileEdit && looksLikeExplicitConcreteTask(text) {
			// fall through
		} else {
			return true
		}
	}
	text := strings.TrimSpace(message)
	if looksLikeExplicitConcreteTask(text) {
		return false
	}
	if detectTurnLocale(text) == localeEN && len([]rune(text)) <= 16 && containsAny(strings.ToLower(text), "this", "that", "it", "something") {
		return true
	}
	// Very short task-plan turns are often vague ("run it", "do that"); longer
	// messages are usually already concrete — explicit cases are handled above.
	if strings.TrimSpace(skill) == SkillTaskPlan && len([]rune(text)) <= 10 {
		return true
	}
	return false
}

func maxFloat(values ...float64) float64 {
	var out float64
	for _, value := range values {
		if value > out {
			out = value
		}
	}
	return out
}
