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
	"github.com/jakejimenez/nlci/internal/retrieval"
	"github.com/jakejimenez/nlci/internal/validator"
)

const (
	maxRetries        = 3 // max inference+validation retries per generation round
	maxExecRetries    = 1 // max rounds of "exec failed → regenerate" retries
	defaultCandidates = 3 // top-K candidates for initial retrieval
	widenedCandidates = 5 // top-K candidates on misrouting retry
)

// Agent orchestrates the full pipeline: retrieve → prompt → infer → validate → execute.
type Agent struct {
	def     *definition.CLIDefinition
	backend *backend.Router
	index   *retrieval.Index
	opts    Options
}

// Options configures the agent's execution behavior.
type Options struct {
	DryRun      bool
	Explain     bool
	AutoConfirm bool   // skip the y/N prompt before exec
	Backend     string // force a specific backend name (empty = auto-detect)
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
	return &Agent{
		def:     def,
		backend: br,
		index:   retrieval.Build(def),
		opts:    opts,
	}
}

// Run executes the full pipeline for the given natural language intent.
// It retrieves candidate commands, infers a command, validates it, optionally
// confirms, and executes it. On CLI usage errors it retries generation once
// with the error injected into the prompt. On misrouting errors it widens the
// candidate set before retrying.
func (a *Agent) Run(ctx context.Context, intent string) (*Result, error) {
	candidates := a.index.Retrieve(intent, defaultCandidates)
	return a.runExecLoop(ctx, intent, candidates, "", 0)
}

// runExecLoop generates a validated command then executes it.
// On a CLI usage error it injects stderr into the next generation prompt.
// On a misrouting error it also widens the retrieval candidate set.
func (a *Agent) runExecLoop(ctx context.Context, intent string, candidates []retrieval.Candidate, errCtx string, execAttempt int) (*Result, error) {
	result, _, err := a.runLoop(ctx, intent, candidates, errCtx, 0)
	if err != nil {
		return nil, err
	}

	execOpts := executor.Options{
		DryRun:      a.opts.DryRun,
		Explain:     a.opts.Explain,
		AutoConfirm: a.opts.AutoConfirm,
	}

	if err := executor.Execute(ctx, result.Command, result.Explanation, execOpts); err != nil {
		var execErr *executor.ExecError
		if execAttempt < maxExecRetries && errors.As(err, &execErr) && execErr.IsUsageError {
			if execErr.IsMisrouting {
				// Wrong subcommand — widen the candidate set and steer strongly.
				wider := a.index.Retrieve(intent, widenedCandidates)
				newCtx := fmt.Sprintf(
					"The command %q was not recognised (wrong subcommand):\n%s\n\nUse a different subcommand from the available commands list.",
					result.Command, strings.TrimSpace(execErr.Stderr),
				)
				return a.runExecLoop(ctx, intent, wider, newCtx, execAttempt+1)
			}
			// Bad flags / bad args — keep same candidates, inject the error.
			newCtx := fmt.Sprintf(
				"The command %q failed with a CLI usage error:\n%s\n\nRegenerate a corrected command.",
				result.Command, strings.TrimSpace(execErr.Stderr),
			)
			return a.runExecLoop(ctx, intent, candidates, newCtx, execAttempt+1)
		}
		return result, err
	}

	return result, nil
}

// Generate runs only the inference + validation part of the pipeline
// (no execution). Used by the SDK's Generate() method.
func (a *Agent) Generate(ctx context.Context, intent string) (*Result, error) {
	candidates := a.index.Retrieve(intent, defaultCandidates)
	result, _, err := a.runLoop(ctx, intent, candidates, "", 0)
	return result, err
}

// runLoop is the agentic retry loop for inference + validation.
// Returns the Result, the validator.Result, and any error.
func (a *Agent) runLoop(ctx context.Context, intent string, candidates []retrieval.Candidate, errorContext string, attempt int) (*Result, validator.Result, error) {
	if attempt >= maxRetries {
		return nil, validator.Result{}, fmt.Errorf("could not generate a valid command after %d attempts.\nTry rephrasing your intent", maxRetries)
	}

	// Build prompt from retrieved candidates.
	req := prompt.Request{
		Def:          a.def,
		Intent:       intent,
		Candidates:   candidates,
		ErrorContext: errorContext,
	}
	system, userPrompt := prompt.Build(req)

	// Trim schema if over token budget.
	if !prompt.FitsInBudget(system, userPrompt) {
		system = prompt.TrimSchema(system, intent, prompt.SafeTokenCeiling)
	}

	// Run inference.
	resp, err := a.backend.Generate(ctx, backend.Request{
		System: system,
		Intent: userPrompt,
	})
	if err != nil {
		return nil, validator.Result{}, fmt.Errorf("inference failed: %w", err)
	}

	command := strings.TrimSpace(resp.Command)

	// Validate.
	v := validator.Validate(command, a.def)
	if !v.Valid {
		nextError := buildErrorContext(command, v.Error, candidates, a.def.Binary, attempt+1)
		return a.runLoop(ctx, intent, candidates, nextError, attempt+1)
	}

	return &Result{
		Command:     command,
		Explanation: resp.Explanation,
		Backend:     a.backend.ActiveName(),
		Attempts:    attempt + 1,
	}, v, nil
}
