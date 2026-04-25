package prompt

import (
	"fmt"
	"strings"

	"github.com/jakejimenez/nlci/internal/definition"
	"github.com/jakejimenez/nlci/internal/retrieval"
)

const (
	// MaxTokens is the hard ceiling for Apple Intelligence context window.
	MaxTokens = 4096
	// SafeTokenCeiling leaves a buffer below the hard limit.
	SafeTokenCeiling = 3500
	// MaxExamples is the maximum number of few-shot examples to include.
	MaxExamples = 3
)

// Request carries all the inputs needed to build an inference prompt.
type Request struct {
	Def          *definition.CLIDefinition
	Intent       string
	Candidates   []retrieval.Candidate // retrieved top-K candidates; nil = show all commands
	ErrorContext string                // optional: injected during agentic loop retries
}

// Build assembles the full inference prompt from a Request.
func Build(r Request) (system string, userPrompt string) {
	system = buildSystem(r.Def, r.Candidates)
	userPrompt = buildUser(r)
	return system, userPrompt
}

func buildSystem(def *definition.CLIDefinition, candidates []retrieval.Candidate) string {
	var b strings.Builder

	// Base instructions: YAML-provided or default.
	if def.SystemPrompt != "" {
		b.WriteString(strings.TrimSpace(def.SystemPrompt))
	} else {
		b.WriteString(DefaultSystemPrompt(def.Name, def.Binary))
	}

	// Always append anti-hallucination constraints so they apply to every
	// definition, whether hand-written or YAML-provided.
	b.WriteString("\n")
	b.WriteString(extraConstraints())

	b.WriteString("\n\n")
	b.WriteString(buildSchema(def, candidates))

	return b.String()
}

func buildUser(r Request) string {
	var b strings.Builder

	// Few-shot examples — top candidate first, best-matching example first within
	// each candidate so the model sees the most relevant pattern at the top.
	examples := collectCandidateExamples(r.Candidates, MaxExamples)
	if len(examples) > 0 {
		b.WriteString("Examples:\n")
		for _, ex := range examples {
			b.WriteString(fmt.Sprintf("  User: %s\n  Command: %s\n", ex[0], ex[1]))
		}
		b.WriteString("\n")
	}

	// NO first-pass live --help injection.
	// Raw help output was found to mislead the model (e.g. docker ps -f json from
	// seeing "-f" and "json" in the same --format flag description).
	// Structured flag hints are only injected on retry via the error context below.

	// Error context injected by the agentic retry loop.
	if r.ErrorContext != "" {
		b.WriteString(r.ErrorContext)
		b.WriteString("\n\n")
	}

	b.WriteString(fmt.Sprintf("User: %s\nCommand:", r.Intent))

	return b.String()
}

// buildSchema lists the candidate commands (or all commands when no candidates).
// Showing only candidates focuses the model on the retrieved subcommand space and
// cuts token usage significantly for large definitions.
func buildSchema(def *definition.CLIDefinition, candidates []retrieval.Candidate) string {
	if def.Mode == "flag_driven" {
		return buildFlagDrivenSchema(def, candidates)
	}

	var b strings.Builder
	b.WriteString("Available commands:\n")

	if len(candidates) > 0 {
		for _, c := range candidates {
			if c.Command.Description != "" {
				b.WriteString(fmt.Sprintf("  %s: %s\n", c.Command.Name, c.Command.Description))
			} else {
				b.WriteString(fmt.Sprintf("  %s\n", c.Command.Name))
			}
		}
	} else {
		for _, cmd := range def.Commands {
			if cmd.Description != "" {
				b.WriteString(fmt.Sprintf("  %s: %s\n", cmd.Name, cmd.Description))
			} else {
				b.WriteString(fmt.Sprintf("  %s\n", cmd.Name))
			}
		}
	}

	return b.String()
}

func buildFlagDrivenSchema(def *definition.CLIDefinition, candidates []retrieval.Candidate) string {
	var b strings.Builder
	b.WriteString("This CLI is flag-driven: compose commands from root flags and positional arguments, not subcommands.\n")
	b.WriteString("Available capabilities:\n")

	capSeen := make(map[string]bool)
	if len(candidates) > 0 {
		for _, c := range candidates {
			if c.Capability.Name == "" || capSeen[c.Capability.Name] {
				continue
			}
			capSeen[c.Capability.Name] = true
			if c.Capability.Description != "" {
				b.WriteString(fmt.Sprintf("  %s: %s\n", c.Capability.Name, c.Capability.Description))
			} else {
				b.WriteString(fmt.Sprintf("  %s\n", c.Capability.Name))
			}
			for _, flagName := range c.Capability.Flags {
				if f := findRootFlag(def.RootFlags, flagName); f != nil {
					b.WriteString(fmt.Sprintf("    --%s", f.Name))
					if f.Short != "" {
						b.WriteString(fmt.Sprintf(" (-%s)", f.Short))
					}
					if f.Description != "" {
						b.WriteString(fmt.Sprintf(": %s", f.Description))
					}
					b.WriteString("\n")
				}
			}
		}
	} else {
		for _, cap := range def.Capabilities {
			if cap.Description != "" {
				b.WriteString(fmt.Sprintf("  %s: %s\n", cap.Name, cap.Description))
			} else {
				b.WriteString(fmt.Sprintf("  %s\n", cap.Name))
			}
		}
	}

	b.WriteString("\nCommon root flags:\n")
	rootFlagCount := 0
	for _, f := range def.RootFlags {
		b.WriteString(fmt.Sprintf("  --%s", f.Name))
		if f.Short != "" {
			b.WriteString(fmt.Sprintf(" (-%s)", f.Short))
		}
		if f.Description != "" {
			b.WriteString(fmt.Sprintf(": %s", f.Description))
		}
		b.WriteString("\n")
		rootFlagCount++
		if rootFlagCount >= 20 {
			break
		}
	}

	return b.String()
}

// collectCandidateExamples gathers up to n few-shot examples, drawing from
// candidates in rank order. Within each candidate the best-matching example
// (as determined by retrieval scoring) is placed first.
func collectCandidateExamples(candidates []retrieval.Candidate, n int) [][2]string {
	var examples [][2]string
	for _, c := range candidates {
		exampleSet := c.Command.Examples
		if c.Capability.Name != "" {
			exampleSet = c.Capability.Examples
		}
		if len(exampleSet) == 0 {
			continue
		}
		for _, ex := range orderedExamples(exampleSet, c.BestExampleIdx) {
			examples = append(examples, [2]string{ex.NL, ex.Cmd})
			if len(examples) >= n {
				return examples
			}
		}
	}
	return examples
}

func findRootFlag(flags []definition.Flag, name string) *definition.Flag {
	for i := range flags {
		if flags[i].Name == name {
			return &flags[i]
		}
	}
	return nil
}

// orderedExamples returns the examples slice with bestIdx moved to position 0.
// This ensures the most relevant example is seen first by the model.
func orderedExamples(exs []definition.Example, bestIdx int) []definition.Example {
	if bestIdx <= 0 || bestIdx >= len(exs) {
		return exs
	}
	result := make([]definition.Example, 0, len(exs))
	result = append(result, exs[bestIdx])
	for i, ex := range exs {
		if i != bestIdx {
			result = append(result, ex)
		}
	}
	return result
}
