package chat

import "testing"

func TestDialogueAgentTreatsAffirmativeFollowupAsConfirm(t *testing.T) {
	agent := NewDialogueAgent(nil)
	decision := agent.Decide("需要", "assistant: 需要我帮你总结这些资料的核心内容吗？", SessionState{
		SessionID:       "default",
		PendingQuestion: "需要我帮你总结这些资料的核心内容吗？",
		PendingAction:   "summarize_focus_task",
		FocusTaskID:     "task-1",
	})
	if decision.Act != DialogueActConfirm {
		t.Fatalf("expected confirm act, got %#v", decision)
	}
}

func TestDialogueAgentTreatsListChoiceAsAnswerWhenAssistantEnumerated(t *testing.T) {
	agent := NewDialogueAgent(nil)
	ctx := "assistant: | 目录名 | 完整路径 |\n| AI_lab | /path/a |\n你想查看的是哪一个？"
	decision := agent.Decide("2吧", ctx, SessionState{})
	if decision.Act != DialogueActAnswer {
		t.Fatalf("expected answer_question, got %#v", decision)
	}
}
