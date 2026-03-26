package chat

import "testing"

func TestHeuristicSkillDecisionPrefersWebSearchForChineseResearchRequest(t *testing.T) {
	decision, ok := heuristicSkillDecision("帮我搜索一下 OpenClaw 的 memory 设计", "")
	if !ok {
		t.Fatalf("expected heuristic decision")
	}
	if decision.Skill != SkillWebSearch {
		t.Fatalf("expected %s, got %s", SkillWebSearch, decision.Skill)
	}
}
