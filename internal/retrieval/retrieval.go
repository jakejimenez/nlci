package retrieval

import (
	"sort"
	"strings"

	"github.com/jakejimenez/nlci/internal/definition"
)

// CommandDocument holds a command with pre-tokenized fields for fast lexical retrieval.
type CommandDocument struct {
	Command       definition.Command
	pathTokens    []string   // tokens from the command name (e.g. ["pr", "create"])
	descTokens    []string   // tokens from the description
	nlTokens      [][]string // tokens from each example's NL field
	synonymTokens []string   // synonym words that map to this command (inverted from def.Synonyms)
}

// Index is the retrieval index built from a CLIDefinition.
type Index struct {
	docs   []CommandDocument
	binary string
}

// Candidate is a retrieval result paired with its relevance score.
// BestExampleIdx is the index of the most NL-relevant example in
// Command.Examples (-1 when no examples exist).
type Candidate struct {
	Command        definition.Command
	Score          float64
	BestExampleIdx int
}

// Build constructs an Index from a CLIDefinition.
// Synonym terms from def.Synonyms are indexed into each matching command's document
// so that e.g. "follow" retrieves "logs" and "shell" retrieves "exec".
func Build(def *definition.CLIDefinition) *Index {
	// Invert the synonym map: command name → synonym words that reference it.
	synonymsForCmd := make(map[string][]string, len(def.Synonyms)*2)
	for word, cmds := range def.Synonyms {
		for _, cmdName := range cmds {
			synonymsForCmd[cmdName] = append(synonymsForCmd[cmdName], word)
		}
	}

	idx := &Index{binary: def.Binary}
	for _, cmd := range def.Commands {
		doc := CommandDocument{
			Command:       cmd,
			pathTokens:    tokenize(cmd.Name),
			descTokens:    tokenize(cmd.Description),
			synonymTokens: synonymsForCmd[cmd.Name],
		}
		for _, ex := range cmd.Examples {
			doc.nlTokens = append(doc.nlTokens, tokenize(ex.NL))
		}
		idx.docs = append(idx.docs, doc)
	}
	return idx
}

// Retrieve returns the top-k candidates for the given intent, sorted by score descending.
// If k <= 0 or no documents exist it returns nil.
func (idx *Index) Retrieve(intent string, k int) []Candidate {
	if k <= 0 || len(idx.docs) == 0 {
		return nil
	}

	intentTokens := tokenize(intent)
	intentLower := strings.ToLower(intent)

	type scored struct {
		doc            CommandDocument
		score          float64
		bestExampleIdx int
	}

	results := make([]scored, 0, len(idx.docs))

	for _, doc := range idx.docs {
		score := 0.0
		bestExampleIdx := -1

		// Exact full-path phrase bonus — strongest signal.
		if strings.Contains(intentLower, strings.ToLower(doc.Command.Name)) {
			score += 10.0
		}

		// Path token overlap — high weight; key for flat commands like "ps", "logs".
		score += float64(tokenOverlap(intentTokens, doc.pathTokens)) * 3.0

		// Description token overlap.
		score += float64(tokenOverlap(intentTokens, doc.descTokens)) * 1.5

		// Synonym token overlap — medium-high weight so "follow" → logs, "shell" → exec.
		score += float64(tokenOverlap(intentTokens, doc.synonymTokens)) * 2.0

		// Example NL overlap — raised to 2.5 (was 1.0) so the closest example
		// dominates over generic description words; track which example was best.
		bestNL := 0
		for i, nlToks := range doc.nlTokens {
			if ov := tokenOverlap(intentTokens, nlToks); ov > bestNL {
				bestNL = ov
				bestExampleIdx = i
			}
		}
		score += float64(bestNL) * 2.5

		// Exact / near-exact NL phrase bonus: if an example NL is a substring of
		// the intent or vice-versa, this is a very strong relevance signal.
		for i, ex := range doc.Command.Examples {
			exLower := strings.ToLower(ex.NL)
			if strings.Contains(intentLower, exLower) || strings.Contains(exLower, intentLower) {
				score += 5.0
				bestExampleIdx = i
				break
			}
		}

		results = append(results, scored{doc, score, bestExampleIdx})
	}

	// Sort by score descending; break ties by command name for stability.
	sort.SliceStable(results, func(i, j int) bool {
		if results[i].score != results[j].score {
			return results[i].score > results[j].score
		}
		return results[i].doc.Command.Name < results[j].doc.Command.Name
	})

	if k > len(results) {
		k = len(results)
	}

	candidates := make([]Candidate, k)
	for i := 0; i < k; i++ {
		candidates[i] = Candidate{
			Command:        results[i].doc.Command,
			Score:          results[i].score,
			BestExampleIdx: results[i].bestExampleIdx,
		}
	}
	return candidates
}

// tokenize splits s into lowercase word tokens, stripping punctuation.
// Tokens shorter than 2 characters are discarded to avoid noise.
func tokenize(s string) []string {
	words := strings.Fields(strings.ToLower(s))
	result := make([]string, 0, len(words))
	for _, w := range words {
		w = strings.Trim(w, ".,!?\"'();:-_/")
		if len(w) >= 2 {
			result = append(result, w)
		}
	}
	return result
}

// tokenOverlap counts distinct query tokens that appear in the target token set.
func tokenOverlap(query, target []string) int {
	if len(target) == 0 {
		return 0
	}
	set := make(map[string]struct{}, len(target))
	for _, t := range target {
		set[t] = struct{}{}
	}
	count := 0
	seen := make(map[string]struct{}, len(query))
	for _, q := range query {
		if _, already := seen[q]; already {
			continue
		}
		seen[q] = struct{}{}
		if _, ok := set[q]; ok {
			count++
		}
	}
	return count
}
