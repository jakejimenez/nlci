package router

import "strings"

// Confidence represents how certain the keyword matcher is about a subcommand match.
type Confidence int

const (
	ConfidenceHigh   Confidence = 3 // exact subcommand name found in intent
	ConfidenceMedium Confidence = 2 // synonym dictionary match
	ConfidenceLow    Confidence = 1 // no match — fall through to routing inference
	ConfidenceNone   Confidence = 0
)

// Match is the result of the keyword routing stage.
type Match struct {
	Subcommand string
	Confidence Confidence
}

// KeywordMatch scores subcommand names against the user's intent using direct
// token overlap and the synonym dictionary.
func KeywordMatch(intent string, subcommands []string, synonyms map[string][]string) Match {
	tokens := tokenize(intent)

	// Stage 1: ALL subcommand tokens must be present in the intent tokens.
	// Requiring full coverage prevents "ps" from matching "system prune"
	// just because "prune" shares a token with "pruning".
	for _, sub := range subcommands {
		subTokens := tokenize(sub)
		if len(subTokens) == 0 {
			continue
		}
		allFound := true
		for _, st := range subTokens {
			found := false
			for _, it := range tokens {
				if st == it {
					found = true
					break
				}
			}
			if !found {
				allFound = false
				break
			}
		}
		if allFound {
			return Match{Subcommand: sub, Confidence: ConfidenceHigh}
		}
	}

	// Stage 2: synonym dictionary — check if any intent token maps to a subcommand
	builtinSynonyms := builtinSynonymMap()

	// Merge user-provided synonyms (user takes priority)
	merged := make(map[string][]string)
	for k, v := range builtinSynonyms {
		merged[k] = v
	}
	for k, v := range synonyms {
		merged[k] = v
	}

	for _, token := range tokens {
		if targets, ok := merged[token]; ok {
			for _, target := range targets {
				for _, sub := range subcommands {
					if strings.EqualFold(sub, target) || strings.Contains(sub, target) {
						return Match{Subcommand: sub, Confidence: ConfidenceMedium}
					}
				}
			}
		}
	}

	return Match{Confidence: ConfidenceLow}
}

// tokenize splits a string into lowercase words.
func tokenize(s string) []string {
	words := strings.Fields(strings.ToLower(s))
	var result []string
	for _, w := range words {
		w = strings.Trim(w, ".,!?\"'();:")
		if len(w) > 1 {
			result = append(result, w)
		}
	}
	return result
}
