package recipe

import (
	"sort"
	"strings"
	"unicode"
)

// NormalizeDietaryTerms normalizes a list of raw dietary term candidates into
// canonical terms used for user preferences and filtering.
func NormalizeDietaryTerms(inputs []string) []string {
	seen := make(map[string]struct{})
	out := make([]string, 0, len(inputs))
	for _, value := range inputs {
		parts := strings.FieldsFunc(strings.ToLower(value), func(r rune) bool {
			return strings.ContainsRune("、，,；;/（）()：:", r) || unicode.IsSpace(r)
		})
		for _, part := range parts {
			term := strings.Trim(part, "-*。.")
			if term == "" || isQuantity(term) {
				continue
			}
			if _, ok := seen[term]; ok {
				continue
			}
			seen[term] = struct{}{}
			out = append(out, term)
		}
	}
	sort.Strings(out)
	return out
}

func isQuantity(value string) bool {
	for _, r := range value {
		if !unicode.IsDigit(r) && !strings.ContainsRune(".-~克斤个只毫升mlgkg", r) {
			return false
		}
	}
	return true
}
