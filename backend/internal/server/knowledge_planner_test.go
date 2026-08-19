package server

import "testing"

func TestKnowledgePlannerFastPathAndBudget(t *testing.T) {
	queries, planned := planKnowledgeQueries("项目结论是什么", true, 4)
	if planned || len(queries) != 1 {
		t.Fatalf("simple query entered planner: %#v", queries)
	}
	queries, planned = planKnowledgeQueries("比较方案 A 与方案 B 和方案 C 的差异", true, 3)
	if !planned || len(queries) > 3 || len(queries) < 2 {
		t.Fatalf("complex query ignored budget: %#v", queries)
	}
	queries, planned = planKnowledgeQueries("比较方案 A 与方案 B", false, 4)
	if planned || len(queries) != 1 {
		t.Fatalf("disabled planner changed query: %#v", queries)
	}
}
