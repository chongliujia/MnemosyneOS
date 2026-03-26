package chat

import "testing"

func TestHeuristicRouteDecisionForChineseSearchRequest(t *testing.T) {
	decision := heuristicRouteDecision("帮我搜索一下 OpenClaw 的 memory 设计", "", SessionState{})
	if decision.IntentKind != IntentKindTask {
		t.Fatalf("expected task_request, got %#v", decision)
	}
	if decision.Skill != SkillWebSearch {
		t.Fatalf("expected %s, got %#v", SkillWebSearch, decision)
	}
}
