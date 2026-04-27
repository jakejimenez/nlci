package router

import (
	"fmt"
	"strings"

	"github.com/jakejimenez/nlci/internal/prompt"
)

const routingInstructions = `You are a tool router. Given a user intent, pick the single best CLI tool to fulfill it.

Rules:
- Output ONLY the tool name, lowercase, on a single line.
- No explanation, no markdown, no quotes.
- Prefer a tool from the list below — its name, description, or synonyms.
- If NO listed tool fits the intent, output exactly: OTHER: <binary>
  where <binary> is a real CLI program (e.g., OTHER: curl, OTHER: kubectl).
  Use OTHER only when no listed tool is appropriate.`

// buildRoutingPrompt assembles the system and user prompts. If the resulting
// pair would exceed prompt.SafeTokenCeiling, metadata is dropped progressively
// (synonyms first, then descriptions truncated, then description-less tools
// dropped) until it fits.
func buildRoutingPrompt(tools []ToolMeta, intent, retryHint string) (system, user string) {
	user = buildUserPrompt(intent, retryHint)

	system = renderSystem(tools, renderOpts{})
	if prompt.FitsInBudget(system, user) {
		return system, user
	}

	system = renderSystem(tools, renderOpts{dropSynonyms: true})
	if prompt.FitsInBudget(system, user) {
		return system, user
	}

	system = renderSystem(tools, renderOpts{dropSynonyms: true, truncDescTo: 60})
	if prompt.FitsInBudget(system, user) {
		return system, user
	}

	// Last resort: drop tools without descriptions.
	filtered := tools[:0:0]
	for _, t := range tools {
		if t.Description != "" {
			filtered = append(filtered, t)
		}
	}
	if len(filtered) == 0 {
		filtered = tools
	}
	system = renderSystem(filtered, renderOpts{dropSynonyms: true, truncDescTo: 60})
	return system, user
}

func buildUserPrompt(intent, retryHint string) string {
	var b strings.Builder
	if retryHint != "" {
		b.WriteString(retryHint)
		b.WriteString("\n\n")
	}
	b.WriteString("Intent: ")
	b.WriteString(intent)
	b.WriteString("\nTool:")
	return b.String()
}

type renderOpts struct {
	dropSynonyms bool
	truncDescTo  int // 0 = no truncation
}

func renderSystem(tools []ToolMeta, opts renderOpts) string {
	var b strings.Builder
	b.WriteString(routingInstructions)
	b.WriteString("\n\nAvailable tools:\n")
	for _, t := range tools {
		desc := t.Description
		if opts.truncDescTo > 0 && len(desc) > opts.truncDescTo {
			desc = strings.TrimSpace(desc[:opts.truncDescTo]) + "…"
		}
		if desc != "" {
			fmt.Fprintf(&b, "  %s: %s\n", t.Name, desc)
		} else {
			fmt.Fprintf(&b, "  %s\n", t.Name)
		}
		if !opts.dropSynonyms && len(t.Synonyms) > 0 {
			aliases := t.Synonyms
			if len(aliases) > 4 {
				aliases = aliases[:4]
			}
			fmt.Fprintf(&b, "    aliases: %s\n", strings.Join(aliases, ", "))
		}
	}
	return b.String()
}
