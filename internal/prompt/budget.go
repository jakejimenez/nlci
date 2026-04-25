package prompt

import "strings"

// EstimateTokens gives a rough token count for a string.
// Approximation: 1 token ≈ 4 characters (conservative for English CLI text).
func EstimateTokens(s string) int {
	return len(s)/4 + 1
}

// EstimateRequestTokens estimates the total token usage for a prompt pair.
func EstimateRequestTokens(system, user string) int {
	return EstimateTokens(system) + EstimateTokens(user)
}

// FitsInBudget returns true if the estimated token count is within the safe ceiling.
func FitsInBudget(system, user string) bool {
	return EstimateRequestTokens(system, user) <= SafeTokenCeiling
}

// TrimSchema reduces the schema portion of a system prompt when token budget is exceeded.
// It keeps the system prompt instructions intact and trims low-relevance commands.
func TrimSchema(system string, intent string, maxTokens int) string {
	// Split system prompt into instructions + schema sections
	parts := strings.SplitN(system, "Available commands:", 2)
	if len(parts) < 2 {
		return system
	}

	instructions := parts[0]
	schemaSection := "Available commands:" + parts[1]

	// Score each command line by keyword overlap with intent
	intentWords := tokenize(intent)
	lines := strings.Split(schemaSection, "\n")

	type scoredLine struct {
		line  string
		score int
	}

	var header string
	var scored []scoredLine
	for _, line := range lines {
		if strings.HasPrefix(line, "Available") {
			header = line
			continue
		}
		if strings.TrimSpace(line) == "" {
			continue
		}
		score := 0
		lower := strings.ToLower(line)
		for _, word := range intentWords {
			if strings.Contains(lower, word) {
				score++
			}
		}
		scored = append(scored, scoredLine{line, score})
	}

	// Rebuild schema keeping highest-scoring lines within budget
	var kept []string
	kept = append(kept, instructions, header)
	for _, s := range scored {
		candidate := strings.Join(kept, "\n") + "\n" + s.line
		if EstimateTokens(candidate) < maxTokens-200 { // 200 token reserve for user prompt
			kept = append(kept, s.line)
		}
	}

	return strings.Join(kept, "\n")
}

// tokenize splits a string into lowercase words for keyword matching.
func tokenize(s string) []string {
	words := strings.Fields(strings.ToLower(s))
	var result []string
	for _, w := range words {
		// Strip punctuation
		w = strings.Trim(w, ".,!?\"'();:")
		if len(w) > 2 { // skip very short words
			result = append(result, w)
		}
	}
	return result
}
