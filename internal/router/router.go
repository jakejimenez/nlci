package router

import (
	"context"

	"github.com/jakejimenez/nlci/internal/definition"
	"github.com/jakejimenez/nlci/internal/prompt"
)

// Router routes a user's natural language intent to the best matching
// subcommand in a CLIDefinition using the cascading strategy:
//   1. Keyword match (HIGH confidence) — exact subcommand name in intent
//   2. Synonym dictionary (MEDIUM confidence) — known word mappings
//   3. Routing inference call (LOW confidence) — tiny model call
type Router struct {
	inference *InferenceRouter
}

// New creates a Router. The generate function is optional — if nil, the router
// will only use keyword matching (no inference fallback for LOW confidence).
func New(generate func(ctx context.Context, system, prompt string) (string, error)) *Router {
	r := &Router{}
	if generate != nil {
		r.inference = &InferenceRouter{Generate: generate}
	}
	return r
}

// Route returns the best matching subcommand name for the given intent.
// If no subcommands are defined, it returns an empty string (zero-config mode).
func (r *Router) Route(ctx context.Context, def *definition.CLIDefinition, intent string) string {
	subcommands := def.SubcommandNames()
	if len(subcommands) == 0 {
		return ""
	}

	match := KeywordMatch(intent, subcommands, def.Synonyms)

	switch match.Confidence {
	case ConfidenceHigh, ConfidenceMedium:
		return match.Subcommand

	case ConfidenceLow:
		if r.inference == nil {
			// No inference backend wired — return empty, let main inference handle it
			return ""
		}
		routed, err := r.inference.Route(ctx, subcommands, intent)
		if err != nil {
			return ""
		}
		return routed
	}

	return ""
}

// BuildPrompt assembles the full inference prompt for a given intent and subcommand.
// If subcommand is non-empty, it fetches targeted --help for that subcommand.
func BuildPrompt(def *definition.CLIDefinition, intent, subcommand, errorContext string) (system, userPrompt string) {
	var helpText string
	if subcommand != "" {
		helpText = definition.SubcommandHelpText(def.Binary, subcommand)
	}

	req := prompt.Request{
		Def:            def,
		Intent:         intent,
		SubcommandHelp: helpText,
		ErrorContext:   errorContext,
	}

	return prompt.Build(req)
}
