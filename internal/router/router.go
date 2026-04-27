// Package router picks a CLI tool to handle a natural-language intent when
// the user invokes nlci without naming one. It is a small wrapper over the
// existing inference backend: build a prompt listing available tools, ask the
// model to pick one, validate the answer is in the list.
package router

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jakejimenez/nlci/internal/backend"
	"github.com/jakejimenez/nlci/internal/definition"
)

// ToolMeta is the minimal metadata the router needs about a tool. Built from
// definition.AvailableTool via MetasFromAvailable.
type ToolMeta struct {
	Name        string
	Description string
	Synonyms    []string
}

// ErrNoTools is returned when the available-tool list is empty.
var ErrNoTools = errors.New("router: no tools available")

// ErrInvalidChoice is returned when the model's answer (after one retry) is
// not a name in the available list.
var ErrInvalidChoice = errors.New("router: model returned a tool not in the available list")

// MetasFromAvailable converts the enumeration result from definition into the
// router's lighter ToolMeta. Pure conversion — preserves order.
func MetasFromAvailable(in []definition.AvailableTool) []ToolMeta {
	out := make([]ToolMeta, 0, len(in))
	for _, t := range in {
		out = append(out, ToolMeta{
			Name:        t.Name,
			Description: t.Description,
			Synonyms:    t.Synonyms,
		})
	}
	return out
}

// Route asks the LLM to pick a tool from `tools` for the given intent. The
// returned name is guaranteed to appear in `tools`. On the first invalid
// answer the prompt is retried once with an explicit "choose from" hint;
// after that it returns ErrInvalidChoice.
func Route(ctx context.Context, intent string, tools []ToolMeta, br *backend.Router) (string, error) {
	if len(tools) == 0 {
		return "", ErrNoTools
	}

	names := toolNameSet(tools)

	system, user := buildRoutingPrompt(tools, intent, "")
	resp, err := br.Generate(ctx, backend.Request{System: system, Intent: user})
	if err != nil {
		return "", err
	}
	choice := normalizeChoice(resp.Command)
	if names[choice] {
		return choice, nil
	}

	hint := fmt.Sprintf(
		"Your previous answer %q was not in the list. Choose exactly one of: %s.",
		choice, strings.Join(toolNames(tools), ", "),
	)
	system, user = buildRoutingPrompt(tools, intent, hint)
	resp, err = br.Generate(ctx, backend.Request{System: system, Intent: user})
	if err != nil {
		return "", err
	}
	choice = normalizeChoice(resp.Command)
	if names[choice] {
		return choice, nil
	}
	return "", fmt.Errorf("%w (got %q; available: %s)", ErrInvalidChoice, choice, strings.Join(toolNames(tools), ", "))
}

// normalizeChoice extracts a clean tool name from raw model output.
// Strategy: take the first non-empty line, lowercase it, strip backticks,
// quotes, leading dashes/asterisks, and trailing punctuation/whitespace.
func normalizeChoice(raw string) string {
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		line = strings.Trim(line, "`\"'")
		line = strings.TrimPrefix(line, "-")
		line = strings.TrimPrefix(line, "*")
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Take only the first whitespace-delimited token in case the model
		// added a trailing description ("docker — for containers").
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		return strings.TrimRight(strings.ToLower(fields[0]), ".,;:")
	}
	return ""
}

func toolNames(tools []ToolMeta) []string {
	out := make([]string, 0, len(tools))
	for _, t := range tools {
		out = append(out, t.Name)
	}
	return out
}

func toolNameSet(tools []ToolMeta) map[string]bool {
	out := make(map[string]bool, len(tools))
	for _, t := range tools {
		out[t.Name] = true
	}
	return out
}
