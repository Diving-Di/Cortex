package server

import "strings"

type knowledgeEvidenceDecision string

const (
	knowledgeDecisionAbsent        knowledgeEvidenceDecision = "absent"
	knowledgeDecisionAmbiguous     knowledgeEvidenceDecision = "ambiguous"
	knowledgeDecisionScopeConflict knowledgeEvidenceDecision = "scope_conflict"
)

func decideWeakKnowledgeEvidence(question string, marginConflict bool) knowledgeEvidenceDecision {
	if marginConflict {
		return knowledgeDecisionScopeConflict
	}
	for _, marker := range []string{"这个", "那个", "它", "之前的", "上次", "最近那个", "哪一个", "哪份"} {
		if strings.Contains(question, marker) {
			return knowledgeDecisionAmbiguous
		}
	}
	return knowledgeDecisionAbsent
}

func clarificationPrompt(decision knowledgeEvidenceDecision) string {
	if decision == knowledgeDecisionScopeConflict {
		return "找到了多个可能的资料范围，请补充要参考的文档、主题或时间范围。"
	}
	return "问题中的指代或条件不够明确，请补充具体对象、主题或时间范围。"
}
