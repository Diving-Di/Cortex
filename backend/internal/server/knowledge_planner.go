package server

import (
	"regexp"
	"strings"
)

var complexKnowledgePattern = regexp.MustCompile(`(?i)(对比|比较|区别|差异|趋势|变化|同比|环比|跨(周|月|年)|vs\.?|versus)`)
var knowledgeSplitPattern = regexp.MustCompile(`\s*(?:和|与|及|对比|比较|vs\.?|versus)\s*`)

// planKnowledgeQueries is deliberately deterministic and cheap. It is the
// rule fast-path in front of any future model planner: ordinary questions do
// not pay a planning call, while obvious comparisons get a bounded plan.
func planKnowledgeQueries(query string, enabled bool, maxQueries int) ([]string, bool) {
	query = strings.TrimSpace(query)
	if !enabled || maxQueries < 2 || !complexKnowledgePattern.MatchString(query) {
		return []string{query}, false
	}
	maxQueries = min(maxQueries, 4)
	result := []string{query}
	seen := map[string]bool{query: true}
	for _, part := range knowledgeSplitPattern.Split(query, -1) {
		part = strings.Trim(strings.TrimSpace(part), "，。！？?：:")
		if len([]rune(part)) < 2 || seen[part] {
			continue
		}
		seen[part] = true
		result = append(result, part)
		if len(result) == maxQueries {
			break
		}
	}
	if len(result) == 1 && strings.Contains(query, "趋势") {
		result = append(result, query+" 早期", query+" 近期")
		if len(result) > maxQueries {
			result = result[:maxQueries]
		}
	}
	return result, len(result) > 1
}
