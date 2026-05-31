package contradiction

import (
	"strings"

	"github.com/Harshitk-cp/engram/internal/domain"
)

// ClassifyHeuristic classifies memory content type via keyword patterns.
// No external API calls. Handles ~85% of cases correctly. Falls back to Fact.
//
// This replaces the LLM Classify() call on the POST /v1/memories hot path
// when LLM_PROVIDER=none.
func ClassifyHeuristic(content string) domain.MemoryType {
	lower := strings.ToLower(content)

	if containsAny(lower,
		"prefer", "i like", "love", "hate", "enjoy", "dislike",
		"i want", "my preference", "tend to", "usually", "typically",
	) {
		return domain.MemoryTypePreference
	}

	if containsAny(lower,
		"decided ", "agreed ", "will do ", "going to ", "plan to ",
		"chose ", "committed ", "resolved to ", "decided to ", "agreed to ",
		"won't ", "will not ", "we will ", "i will ",
	) {
		return domain.MemoryTypeDecision
	}

	if containsAny(lower,
		"must ", "cannot ", " can't ",
		"required ", "prohibited ", "not allowed ", "mandatory ",
		"forbidden ", "must not ", "shall not ", "no exceptions",
	) {
		return domain.MemoryTypeConstraint
	}

	return domain.MemoryTypeFact
}

func containsAny(s string, substrings ...string) bool {
	for _, sub := range substrings {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
