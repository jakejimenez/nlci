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

	if def.SystemPrompt != "" {
		b.WriteString(strings.TrimSpace(def.SystemPrompt))
	} else {
		b.WriteString(DefaultSystemPrompt(def.Name, def.Binary))
	}

	b.WriteString("\n\n")
	b.WriteString(buildSchema(def, candidates))

	return b.String()
}

func buildUser(r Request) string {
	var b strings.Builder

	// Few-shot examples — draw from candidates (top candidate first).
	examples := collectCandidateExamples(r.Candidates, MaxExamples)
	if len(examples) > 0 {
		b.WriteString("Examples:\n")
		for _, ex := range examples {
			b.WriteString(fmt.Sprintf("  User: %s\n  Command: %s\n", ex[0], ex[1]))
		}
		b.WriteString("\n")
	}

	// Live --help for the top candidate to expose exact flag names.
	if len(r.Candidates) > 0 && r.Def != nil {
		helpText := definition.SubcommandHelpText(r.Def.Binary, r.Candidates[0].Command.Name)
		if helpText != "" {
			b.WriteString("Relevant flags:\n")
			b.WriteString(helpText)
			b.WriteString("\n\n")
		}
	}

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

// collectCandidateExamples gathers up to n few-shot examples, drawing from
// candidates in rank order (top candidate's examples first).
func collectCandidateExamples(candidates []retrieval.Candidate, n int) [][2]string {
	var examples [][2]string
	for _, c := range candidates {
		for _, ex := range c.Command.Examples {
			examples = append(examples, [2]string{ex.NL, ex.Cmd})
			if len(examples) >= n {
				return examples
			}
		}
	}
	return examples
}
