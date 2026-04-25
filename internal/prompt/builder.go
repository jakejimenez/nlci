package prompt

import (
	"fmt"
	"strings"

	"github.com/jakejimenez/nlci/internal/definition"
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
	Def           *definition.CLIDefinition
	Intent        string
	SubcommandHelp string // optional: targeted --help slice for the matched subcommand
	ErrorContext   string // optional: injected during agentic loop retries
}

// Build assembles the full inference prompt from a Request.
// It respects the token budget and trims if necessary.
func Build(r Request) (system string, userPrompt string) {
	system = buildSystem(r.Def)
	userPrompt = buildUser(r)
	return system, userPrompt
}

// BuildRouting builds a minimal routing prompt to identify the best subcommand.
// Used by the cascading router when keyword confidence is LOW.
func BuildRouting(subcommands []string, intent string) string {
	return fmt.Sprintf(
		"Subcommands: %s\nIntent: %s\nWhich single subcommand best matches this intent? Reply with one word only.",
		strings.Join(subcommands, ", "),
		intent,
	)
}

func buildSystem(def *definition.CLIDefinition) string {
	var b strings.Builder

	// Use provided system prompt or fall back to default template
	if def.SystemPrompt != "" {
		b.WriteString(strings.TrimSpace(def.SystemPrompt))
	} else {
		b.WriteString(DefaultSystemPrompt(def.Name, def.Binary))
	}

	b.WriteString("\n\n")
	b.WriteString(buildSchema(def))

	return b.String()
}

func buildUser(r Request) string {
	var b strings.Builder

	// Few-shot examples
	examples := collectExamples(r.Def, MaxExamples)
	if len(examples) > 0 {
		b.WriteString("Examples:\n")
		for _, ex := range examples {
			b.WriteString(fmt.Sprintf("  User: %s\n  Command: %s\n", ex[0], ex[1]))
		}
		b.WriteString("\n")
	}

	// Targeted subcommand --help slice
	if r.SubcommandHelp != "" {
		b.WriteString("Relevant flags:\n")
		b.WriteString(r.SubcommandHelp)
		b.WriteString("\n\n")
	}

	// Error context from agentic loop
	if r.ErrorContext != "" {
		b.WriteString(r.ErrorContext)
		b.WriteString("\n\n")
	}

	b.WriteString(fmt.Sprintf("User: %s\nCommand:", r.Intent))

	return b.String()
}

// buildSchema compresses the CLI definition into a token-efficient schema string.
func buildSchema(def *definition.CLIDefinition) string {
	var b strings.Builder
	b.WriteString("Available commands:\n")

	for _, cmd := range def.Commands {
		if cmd.Description != "" {
			b.WriteString(fmt.Sprintf("  %s: %s\n", cmd.Name, cmd.Description))
		} else {
			b.WriteString(fmt.Sprintf("  %s\n", cmd.Name))
		}
	}

	return b.String()
}

// collectExamples gathers up to n few-shot examples from across all commands.
func collectExamples(def *definition.CLIDefinition, n int) [][2]string {
	var examples [][2]string
	for _, cmd := range def.Commands {
		for _, ex := range cmd.Examples {
			examples = append(examples, [2]string{ex.NL, ex.Cmd})
			if len(examples) >= n {
				return examples
			}
		}
	}
	return examples
}
