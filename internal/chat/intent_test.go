package chat

import (
	"context"
	"testing"

	"mnemosyneos/internal/model"
)

func TestIntentAgentGreetingIsDirectReply(t *testing.T) {
	agent := NewIntentAgent(nil)
	decision := agent.Decide("你好")
	if decision.Kind != IntentKindDirect {
		t.Fatalf("expected direct reply intent, got %#v", decision)
	}
}

func TestIntentAgentOperationalRequestCreatesTask(t *testing.T) {
	agent := NewIntentAgent(nil)
	decision := agent.Decide("帮我搜索 AgentOS 定位")
	if decision.Kind != IntentKindTask {
		t.Fatalf("expected task request intent, got %#v", decision)
	}
}

func TestIntentAgentPrefersModelWhenAvailable(t *testing.T) {
	agent := NewIntentAgent(fakeIntentModel{
		resp: model.TextResponse{
			Text: `{"kind":"direct_reply","reason":"model classified greeting","confidence":0.88}`,
		},
	})
	decision := agent.Decide("帮我搜索 AgentOS 定位")
	if decision.Kind != IntentKindDirect {
		t.Fatalf("expected model-prioritized direct reply, got %#v", decision)
	}
	if decision.Reason != "model classified greeting" {
		t.Fatalf("expected model reason, got %#v", decision)
	}
}

func TestHeuristicReadOnlyFilesystemLookupIsDirect(t *testing.T) {
	t.Parallel()
	decision, ok := heuristicIntentDecision("查看一个叫lab的目录位置", "")
	if !ok {
		t.Fatal("expected heuristic to match")
	}
	if decision.Kind != IntentKindDirect {
		t.Fatalf("expected direct_reply, got %#v", decision)
	}
}

func TestPathPickerFollowupIsReadOnlyLookup(t *testing.T) {
	t.Parallel()
	if !looksLikeReadOnlyFilesystemLookup("/Users/jiachongliu/lab 这个") {
		t.Fatal("expected path-picker follow-up to count as read-only filesystem lookup")
	}
	decision, ok := heuristicIntentDecision("/Users/jiachongliu/lab 这个", "")
	if !ok || decision.Kind != IntentKindDirect {
		t.Fatalf("expected direct_reply, ok=%v decision=%#v", ok, decision)
	}
}

func TestListChoiceWithEnumeratedContextIsDirectIntent(t *testing.T) {
	t.Parallel()
	ctx := "assistant: | 目录名 | 完整路径 |\n| AI_lab | /path/a |\n你想查看的是哪一个？"
	decision, ok := heuristicIntentDecision("2吧", ctx)
	if !ok || decision.Kind != IntentKindDirect {
		t.Fatalf("expected direct_reply, ok=%v decision=%#v", ok, decision)
	}
}

func TestIntentAgentQuestionAboutProjectIsDirectReply(t *testing.T) {
	agent := NewIntentAgent(nil)

	questions := []string{
		"你能告訴我，我的MnemosyneOS這個項目的位置嗎？",
		"这个项目是什么？",
		"文件在哪里？",
		"我的任务有哪些？",
		"what is this project?",
		"where are my files?",
		"how does the memory system work?",
	}
	for _, q := range questions {
		decision := agent.Decide(q)
		if decision.Kind != IntentKindDirect {
			t.Errorf("expected direct_reply for question %q, got %s (reason: %s)", q, decision.Kind, decision.Reason)
		}
	}
}

func TestIntentAgentImperativeCommandIsTask(t *testing.T) {
	agent := NewIntentAgent(nil)

	commands := []string{
		"帮我搜索一下最新的AI新闻",
		"帮我查一下GitHub上的issue",
		"搜索 OpenClaw memory 设计",
		"执行这个shell命令",
	}
	for _, cmd := range commands {
		decision := agent.Decide(cmd)
		if decision.Kind != IntentKindTask {
			t.Errorf("expected task_request for command %q, got %s (reason: %s)", cmd, decision.Kind, decision.Reason)
		}
	}
}

func TestIntentAgentTreatsShortAffirmativeAsDirectReplyWhenAssistantOfferedContinuation(t *testing.T) {
	agent := NewIntentAgent(nil)
	decision := agent.DecideWithContext("需要", "assistant: 需要我帮你总结这些资料的核心内容吗？")
	if decision.Kind != IntentKindDirect {
		t.Fatalf("expected direct reply follow-up intent, got %#v", decision)
	}
}

// TestIntentAgentFallbackPrefersDirectReply pins down the new default: when
// neither the LLM nor the heuristic can classify a message confidently, we
// now prefer direct_reply (conversation) over task_request. Silently
// spinning up an approval-gated task because the classifier shrugged is a
// worse UX than asking one clarifying question.
func TestIntentAgentFallbackPrefersDirectReply(t *testing.T) {
	agent := NewIntentAgent(nil)
	// A statement-shaped message with no action verb, no topic noun, no
	// question marker, no filesystem path, and long enough to miss the
	// "very short" heuristic — this falls through to the final default.
	decision := agent.Decide("天气真好啊今天外面阳光明媚我想散散步")
	if decision.Kind != IntentKindDirect {
		t.Fatalf("expected direct_reply as the final fallback, got %#v", decision)
	}
}

// TestIntentAgentLLMCanOverrideHeuristicForAmbiguousTarget verifies that
// when the model classifies a "帮我…" message as direct_reply because the
// target is ambiguous, the agent respects the model even though the
// heuristic would have routed it to task_request on the "帮我" marker. This
// is the exact case that previously sent "帮我查看 la b" into task-plan mode
// with no chat reply visible.
func TestIntentAgentLLMCanOverrideHeuristicForAmbiguousTarget(t *testing.T) {
	agent := NewIntentAgent(fakeIntentModel{
		resp: model.TextResponse{
			Text: `{"kind":"direct_reply","reason":"target name is ambiguous","confidence":0.9}`,
		},
	})
	decision := agent.Decide("你能帮我查看一个名叫 la b 的目录")
	if decision.Kind != IntentKindDirect {
		t.Fatalf("expected LLM direct_reply to win over heuristic, got %#v", decision)
	}
}

type fakeIntentModel struct {
	resp model.TextResponse
	err  error
}

func (f fakeIntentModel) GenerateText(_ context.Context, _ model.TextRequest) (model.TextResponse, error) {
	return f.resp, f.err
}

func (f fakeIntentModel) StreamText(_ context.Context, _ model.TextRequest, _ func(model.TextDelta) error) (model.TextResponse, error) {
	return f.resp, f.err
}
