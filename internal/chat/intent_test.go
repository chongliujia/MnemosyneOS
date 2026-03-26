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

func TestIntentAgentTreatsShortAffirmativeAsDirectReplyWhenAssistantOfferedContinuation(t *testing.T) {
	agent := NewIntentAgent(nil)
	decision := agent.DecideWithContext("需要", "assistant: 需要我帮你总结这些资料的核心内容吗？")
	if decision.Kind != IntentKindDirect {
		t.Fatalf("expected direct reply follow-up intent, got %#v", decision)
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
