package router

import (
	"context"
	"strings"
)

// InferenceRouter uses a backend inference call to route LOW-confidence intents
// to the best subcommand. It sends a tiny prompt (~70 tokens) and expects
// a one-word reply.
type InferenceRouter struct {
	// Generate is the inference function — matches the backend.Backend.Generate signature
	// but scoped here to avoid circular imports. The caller wires this up.
	Generate func(ctx context.Context, system, prompt string) (string, error)
}

// Route asks the model to pick the best subcommand for the given intent.
// Returns the matched subcommand name or an error.
func (r *InferenceRouter) Route(ctx context.Context, subcommands []string, intent string) (string, error) {
	system := "You are a CLI command router. Reply with one word only — the subcommand name."
	prompt := buildRoutingPrompt(subcommands, intent)

	raw, err := r.Generate(ctx, system, prompt)
	if err != nil {
		return "", err
	}

	response := strings.TrimSpace(strings.ToLower(raw))
	// Strip any punctuation or surrounding quotes the model might add
	response = strings.Trim(response, "\"'`.,!?")

	// Validate the response is actually one of our subcommands
	for _, sub := range subcommands {
		if strings.EqualFold(sub, response) || strings.HasPrefix(sub, response) {
			return sub, nil
		}
	}

	// If model hallucinated, pick the first subcommand as fallback
	if len(subcommands) > 0 {
		return subcommands[0], nil
	}

	return "", nil
}

func buildRoutingPrompt(subcommands []string, intent string) string {
	return "Subcommands: " + strings.Join(subcommands, ", ") +
		"\nIntent: " + intent +
		"\nWhich single subcommand best matches this intent? Reply with one word only."
}
