package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jakejimenez/nlci/internal/backend"
	"github.com/jakejimenez/nlci/internal/definition"
	"github.com/jakejimenez/nlci/internal/executor"
	"github.com/jakejimenez/nlci/internal/prompt"
	"github.com/jakejimenez/nlci/internal/router"
	"github.com/jakejimenez/nlci/internal/validator"
)

const (
	maxRetries     = 3 // max inference+validation retries per generation round
	maxExecRetries = 1 // max rounds of "exec failed → regenerate" retries
)

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
// On CLI usage errors (unknown flag, unknown command, etc.) it retries
// generation once with the error output injected into the prompt.
func (a *Agent) Run(ctx context.Context, intent string) (*Result, error) {
	subcommand := a.router.Route(ctx, a.def, intent)
	return a.runExecLoop(ctx, intent, subcommand, "", 0)
}

// runExecLoop generates a validated command then executes it.
// On a CLI usage error it injects the stderr into the next generation prompt
// and retries up to maxExecRetries times.
func (a *Agent) runExecLoop(ctx context.Context, intent, subcommand, errCtx string, execAttempt int) (*Result, error) {
	result, v, err := a.runLoop(ctx, intent, subcommand, errCtx, 0)
	if err != nil {
		return nil, err
	}

	execOpts := executor.Options{
		DryRun:  a.opts.DryRun,
		Explain: a.opts.Explain,
	}

	if err := executor.Execute(ctx, result.Command, result.Explanation, v.RequiresConfirmation, execOpts); err != nil {
		var execErr *executor.ExecError
		if execAttempt < maxExecRetries && errors.As(err, &execErr) && execErr.IsUsageError {
			newCtx := fmt.Sprintf(
				"The command %q failed with a CLI usage error:\n%s\n\nRegenerate a corrected command.",
				result.Command, strings.TrimSpace(execErr.Stderr),
			)
			return a.runExecLoop(ctx, intent, subcommand, newCtx, execAttempt+1)
		}
		return result, err
	}

	return result, nil
}

// Generate runs only the inference + validation part of the pipeline
// (no execution). Used by the SDK's Generate() method.
func (a *Agent) Generate(ctx context.Context, intent string) (*Result, error) {
	subcommand := a.router.Route(ctx, a.def, intent)
	result, _, err := a.runLoop(ctx, intent, subcommand, "", 0)
	return result, err
}

// runLoop is the agentic retry loop for inference + validation.
// Returns the Result, the validator.Result (for RequiresConfirmation), and any error.
func (a *Agent) runLoop(ctx context.Context, intent, subcommand, errorContext string, attempt int) (*Result, validator.Result, error) {
	if attempt >= maxRetries {
		return nil, validator.Result{}, fmt.Errorf("could not generate a valid command after %d attempts.\nTry rephrasing your intent", maxRetries)
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
		return nil, validator.Result{}, fmt.Errorf("inference failed: %w", err)
	}

	command := strings.TrimSpace(resp.Command)

	// Validate
	v := validator.Validate(command, a.def)
	if !v.Valid {
		nextError := buildErrorContext(command, v.Error, subcommand, a.def.Binary, attempt+1)
		return a.runLoop(ctx, intent, subcommand, nextError, attempt+1)
	}

	return &Result{
		Command:     command,
		Explanation: resp.Explanation,
		Backend:     a.backend.ActiveName(),
		Attempts:    attempt + 1,
	}, v, nil
}
