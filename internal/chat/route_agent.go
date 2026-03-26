package chat

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"mnemosyneos/internal/model"
)

type RouteDecision struct {
	DialogueAct string  `json:"dialogue_act"`
	IntentKind  string  `json:"intent_kind"`
	Skill       string  `json:"skill,omitempty"`
	Reason      string  `json:"reason,omitempty"`
	Confidence  float64 `json:"confidence,omitempty"`
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
		}
	}

	if a != nil && a.model != nil {
		if decision, err := a.modelDecision(message, conversationContext, state); err == nil && decision.DialogueAct != "" && decision.IntentKind != "" {
			return decision
		}
	}
	return heuristicRouteDecision(message, conversationContext, state)
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
		SystemPrompt: "Route the user message for an AgentOS chat turn. Return strict JSON only: {\"dialogue_act\":\"chat|new_task|answer_question|confirm|deny|continue_task|clarify|switch_topic\",\"intent_kind\":\"direct_reply|task_request\",\"skill\":\"task-plan|web-search|github-issue-search|email-inbox|memory-consolidate|file-read|file-edit|shell-command\",\"reason\":\"...\",\"confidence\":0.0-1.0}. For short follow-ups like '需要', '继续', '好的', use recent context and pending_question. Use direct_reply when the user is answering or confirming within the current thread instead of asking for a new task. Use web-search for external research requests. Use memory-consolidate only when the user explicitly asks to consolidate memory itself. If intent_kind is direct_reply, skill may be empty.",
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
	if decision.Skill != "" {
		if _, ok := validSkills[decision.Skill]; !ok {
			return RouteDecision{}, err
		}
	}
	return decision, nil
}

func heuristicRouteDecision(message, conversationContext string, state SessionState) RouteDecision {
	dialogue, _ := heuristicDialogueDecision(message, conversationContext, state)
	intent, _ := heuristicIntentDecision(message, conversationContext)
	skill, _ := heuristicSkillDecision(message, conversationContext)

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

	return RouteDecision{
		DialogueAct: dialogue.Act,
		IntentKind:  intent.Kind,
		Skill:       skill.Skill,
		Reason:      firstNonEmpty(dialogue.Reason, intent.Reason, skill.Reason),
		Confidence:  maxFloat(dialogue.Confidence, intent.Confidence, skill.Confidence),
	}
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
