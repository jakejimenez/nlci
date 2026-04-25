package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/jakejimenez/nlci/internal/backend"
	"github.com/jakejimenez/nlci/internal/definition"
	"github.com/jakejimenez/nlci/internal/executor"
	"github.com/jakejimenez/nlci/internal/prompt"
	"github.com/jakejimenez/nlci/internal/router"
	"github.com/jakejimenez/nlci/internal/validator"
)

const maxRetries = 3

// Agent orchestrates the full pipeline: route → prompt → infer → validate → execute.
type Agent struct {
	def     *definition.CLIDefinition
	backend *backend.Router
	router  *router.Router
	opts    Options
}

// Options configures the agent's execution behavior.
type Options struct {
	DryRun  bool
	Explain bool
	Backend string // force a specific backend name (empty = auto-detect)
}

// Result is the outcome of a Run call.
type Result struct {
	Command     string
	Explanation string
	Backend     string
	Attempts    int
}

// New creates an Agent for the given definition and backend router.
func New(def *definition.CLIDefinition, br *backend.Router, opts Options) *Agent {
	// Wire the backend's GenerateRaw into the router for LOW-confidence routing inference
	r := router.New(func(ctx context.Context, system, p string) (string, error) {
		return br.GenerateRaw(ctx, system, p)
	})

	return &Agent{
		def:     def,
		backend: br,
		router:  r,
		opts:    opts,
	}
}

// Run executes the full pipeline for the given natural language intent.
// It infers a command, validates it, optionally confirms, and executes it.
func (a *Agent) Run(ctx context.Context, intent string) (*Result, error) {
	result, err := a.Generate(ctx, intent)
	if err != nil {
		return nil, err
	}

	// Validate
	v := validator.Validate(result.Command, a.def)
	if !v.Valid {
		return nil, fmt.Errorf("validation failed: %s", v.Error)
	}

	// Execute
	execOpts := executor.Options{
		DryRun:  a.opts.DryRun,
		Explain: a.opts.Explain,
	}
	if err := executor.Execute(result.Command, result.Explanation, v.RequiresConfirmation, execOpts); err != nil {
		return result, err
	}

	return result, nil
}

// Generate runs only the inference + validation part of the pipeline
// (no execution). Used by the SDK's Generate() method.
func (a *Agent) Generate(ctx context.Context, intent string) (*Result, error) {
	// Route to the best subcommand
	subcommand := a.router.Route(ctx, a.def, intent)

	return a.runLoop(ctx, intent, subcommand, "", 0)
}

// runLoop is the agentic retry loop.
func (a *Agent) runLoop(ctx context.Context, intent, subcommand, errorContext string, attempt int) (*Result, error) {
	if attempt >= maxRetries {
		return nil, fmt.Errorf("could not generate a valid command after %d attempts.\nTry rephrasing your intent", maxRetries)
	}

	// Build prompt
	system, userPrompt := router.BuildPrompt(a.def, intent, subcommand, errorContext)

	// Check token budget and trim if needed
	if !prompt.FitsInBudget(system, userPrompt) {
		system = prompt.TrimSchema(system, intent, prompt.SafeTokenCeiling)
	}

	// Run inference
	req := backend.Request{
		System: system,
		Intent: userPrompt,
	}
	resp, err := a.backend.Generate(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("inference failed: %w", err)
	}

	command := strings.TrimSpace(resp.Command)

	// Validate
	v := validator.Validate(command, a.def)
	if !v.Valid {
		// Build error context for next attempt
		nextError := buildErrorContext(command, v.Error, subcommand, a.def.Binary, attempt+1)
		return a.runLoop(ctx, intent, subcommand, nextError, attempt+1)
	}

	return &Result{
		Command:     command,
		Explanation: resp.Explanation,
		Backend:     a.backend.ActiveName(),
		Attempts:    attempt + 1,
	}, nil
}
