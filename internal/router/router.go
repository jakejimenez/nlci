// Package router picks a CLI tool to handle a natural-language intent when
// the user invokes nlci without naming one. It is a small wrapper over the
// existing inference backend: build a prompt listing available tools, ask the
// model to pick one, validate the answer is in the list.
package router

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"

	"github.com/jakejimenez/nlci/internal/backend"
	"github.com/jakejimenez/nlci/internal/definition"
)

// otherBinaryRe restricts OTHER: <name> answers to plausible CLI binary names
// — same character class deterministic discovery uses for command parts.
var otherBinaryRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_.-]*$`)

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

// Route asks the LLM to pick a tool for the given intent. The returned name
// is either an entry from `tools` or — when the model answers `OTHER: <bin>`
// — a binary verified to exist on PATH so callers can chain into auto-init.
// The prompt is retried once when the answer is unparseable; after that it
// returns ErrInvalidChoice.
func Route(ctx context.Context, intent string, tools []ToolMeta, br *backend.Router) (string, error) {
	if len(tools) == 0 {
		return "", ErrNoTools
	}

	names := toolNameSet(tools)

	resolve := func(retryHint string) (string, error) {
		system, user := buildRoutingPrompt(tools, intent, retryHint)
		resp, err := br.Generate(ctx, backend.Request{System: system, Intent: user})
		if err != nil {
			return "", err
		}
		ch := parseChoice(resp.Command)
		// Explicit escape hatch: model used `OTHER: <binary>`.
		if ch.other {
			if !otherBinaryRe.MatchString(ch.name) {
				return "", fmt.Errorf("%w (got OTHER:%q which is not a valid binary name)", ErrInvalidChoice, ch.name)
			}
			if _, err := exec.LookPath(ch.name); err != nil {
				return "", fmt.Errorf("router: model suggested %q via OTHER but it is not installed on PATH", ch.name)
			}
			return ch.name, nil
		}
		// Listed tool — accept directly.
		if names[ch.name] {
			return ch.name, nil
		}
		// Implicit escape hatch: model returned a plain name not in the list.
		// If it parses as a real binary on PATH, treat it the same as OTHER.
		// Apple Intelligence frequently ignores the OTHER prefix instruction,
		// and forcing a retry rarely fixes that — be lenient when we can
		// safely verify the binary exists, otherwise fall through to retry.
		if otherBinaryRe.MatchString(ch.name) {
			if _, err := exec.LookPath(ch.name); err == nil {
				return ch.name, nil
			}
		}
		return "", invalidListChoice{got: ch.name}
	}

	if name, err := resolve(""); err == nil {
		return name, nil
	} else if _, ok := err.(invalidListChoice); !ok {
		return "", err
	}

	hint := fmt.Sprintf(
		"Your previous answer was not valid. Choose exactly one of: %s — or output OTHER: <binary> for a CLI not in the list.",
		strings.Join(toolNames(tools), ", "),
	)
	if name, err := resolve(hint); err == nil {
		return name, nil
	} else if invalid, ok := err.(invalidListChoice); ok {
		return "", fmt.Errorf("%w (got %q; available: %s)", ErrInvalidChoice, invalid.got, strings.Join(toolNames(tools), ", "))
	} else {
		return "", err
	}
}

// invalidListChoice is an internal sentinel signaling the model returned a
// plain name that is not in the available list (and is not an OTHER: answer).
// Used to drive a single retry; never escapes the package.
type invalidListChoice struct{ got string }

func (e invalidListChoice) Error() string { return "router: invalid list choice: " + e.got }

// choice is the parsed model answer. `other` true means the model invoked
// the escape hatch and `name` is the requested binary; otherwise `name` is
// expected to be one of the listed tools.
type choice struct {
	name  string
	other bool
}

// parseChoice extracts a clean answer from raw model output. Strategy: take
// the first non-empty line, lowercase it, strip backticks/quotes/leading
// markdown bullets and trailing punctuation. If the line begins with
// `other:`, return the rest as an escape-hatch binary name.
func parseChoice(raw string) choice {
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		line = strings.Trim(line, "`\"'")
		line = strings.TrimPrefix(line, "-")
		line = strings.TrimPrefix(line, "*")
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		lower := strings.ToLower(line)
		if rest, ok := strings.CutPrefix(lower, "other:"); ok {
			rest = strings.TrimSpace(rest)
			rest = strings.Trim(rest, "`\"'")
			fields := strings.Fields(rest)
			if len(fields) == 0 {
				return choice{}
			}
			return choice{name: strings.TrimRight(fields[0], ".,;:"), other: true}
		}
		fields := strings.Fields(lower)
		if len(fields) == 0 {
			continue
		}
		return choice{name: strings.TrimRight(fields[0], ".,;:")}
	}
	return choice{}
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
