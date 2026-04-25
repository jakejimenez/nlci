package retrieval

import (
	"sort"
	"strings"

	"github.com/jakejimenez/nlci/internal/definition"
)

// CommandDocument holds either a command-tree command or a flag-driven capability
// with pre-tokenized fields for fast lexical retrieval.
type CommandDocument struct {
	Command       definition.Command
	Capability    definition.Capability
	pathTokens    []string
	descTokens    []string
	nlTokens      [][]string
	synonymTokens []string
	flagTokens    []string
	isCapability  bool
}

// Index is the retrieval index built from a CLIDefinition.
type Index struct {
	docs   []CommandDocument
	binary string
	mode   string
}

// Candidate is a retrieval result paired with its relevance score.
// Exactly one of Command or Capability is populated.
type Candidate struct {
	Command        definition.Command
	Capability     definition.Capability
	Score          float64
	BestExampleIdx int
}

// Build constructs an Index from a CLIDefinition.
func Build(def *definition.CLIDefinition) *Index {
	synonymsForTarget := make(map[string][]string, len(def.Synonyms)*2)
	for word, targets := range def.Synonyms {
		for _, target := range targets {
			synonymsForTarget[target] = append(synonymsForTarget[target], word)
		}
	}

	idx := &Index{binary: def.Binary, mode: def.Mode}
	if def.Mode == "flag_driven" {
		flagByName := make(map[string]definition.Flag, len(def.RootFlags))
		for _, f := range def.RootFlags {
			flagByName[f.Name] = f
		}
		for _, cap := range def.Capabilities {
			doc := CommandDocument{
				Capability:    cap,
				pathTokens:    tokenize(cap.Name),
				descTokens:    tokenize(cap.Description),
				synonymTokens: synonymsForTarget[cap.Name],
				isCapability:  true,
			}
			for _, ex := range cap.Examples {
				doc.nlTokens = append(doc.nlTokens, tokenize(ex.NL))
			}
			for _, flagName := range cap.Flags {
				if f, ok := flagByName[flagName]; ok {
					doc.flagTokens = append(doc.flagTokens, tokenize(f.Name)...)
					doc.flagTokens = append(doc.flagTokens, tokenize(f.Description)...)
				}
			}
			idx.docs = append(idx.docs, doc)
		}
		return idx
	}

	for _, cmd := range def.Commands {
		doc := CommandDocument{
			Command:       cmd,
			pathTokens:    tokenize(cmd.Name),
			descTokens:    tokenize(cmd.Description),
			synonymTokens: synonymsForTarget[cmd.Name],
		}
		for _, ex := range cmd.Examples {
			doc.nlTokens = append(doc.nlTokens, tokenize(ex.NL))
		}
		idx.docs = append(idx.docs, doc)
	}
	return idx
}

// Retrieve returns the top-k candidates for the given intent, sorted by score descending.
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
		targetName := doc.Command.Name
		examples := doc.Command.Examples
		if doc.isCapability {
			targetName = doc.Capability.Name
			examples = doc.Capability.Examples
		}

		score := 0.0
		bestExampleIdx := -1

		if strings.Contains(intentLower, strings.ToLower(targetName)) {
			score += 10.0
		}
		score += float64(tokenOverlap(intentTokens, doc.pathTokens)) * 3.0
		score += float64(tokenOverlap(intentTokens, doc.descTokens)) * 1.5
		score += float64(tokenOverlap(intentTokens, doc.synonymTokens)) * 2.0
		if doc.isCapability {
			score += float64(tokenOverlap(intentTokens, doc.flagTokens)) * 1.5
		}

		bestNL := 0
		for i, nlToks := range doc.nlTokens {
			if ov := tokenOverlap(intentTokens, nlToks); ov > bestNL {
				bestNL = ov
				bestExampleIdx = i
			}
		}
		score += float64(bestNL) * 2.5

		for i, ex := range examples {
			exLower := strings.ToLower(ex.NL)
			if strings.Contains(intentLower, exLower) || strings.Contains(exLower, intentLower) {
				score += 5.0
				bestExampleIdx = i
				break
			}
		}

		results = append(results, scored{doc: doc, score: score, bestExampleIdx: bestExampleIdx})
	}

	sort.SliceStable(results, func(i, j int) bool {
		if results[i].score != results[j].score {
			return results[i].score > results[j].score
		}
		left := results[i].doc.Command.Name
		right := results[j].doc.Command.Name
		if results[i].doc.isCapability {
			left = results[i].doc.Capability.Name
		}
		if results[j].doc.isCapability {
			right = results[j].doc.Capability.Name
		}
		return left < right
	})

	if k > len(results) {
		k = len(results)
	}

	candidates := make([]Candidate, k)
	for i := 0; i < k; i++ {
		candidates[i] = Candidate{
			Command:        results[i].doc.Command,
			Capability:     results[i].doc.Capability,
			Score:          results[i].score,
			BestExampleIdx: results[i].bestExampleIdx,
		}
	}
	return candidates
}

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
