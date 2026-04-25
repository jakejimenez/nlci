package executor

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

// Options controls execution behavior.
type Options struct {
	DryRun  bool
	Explain bool // always print explanation even if not requiring confirmation
}

// ExecError carries details about a failed command execution.
// IsUsageError is true when the failure looks like a CLI flag/syntax error
// (e.g. "unknown flag", "unknown command") rather than a runtime failure.
type ExecError struct {
	Cmd         string
	Err         error
	Stderr      string
	IsUsageError bool
}

func (e *ExecError) Error() string {
	if e.Stderr != "" {
		return fmt.Sprintf("executor: command failed: %v\n%s", e.Err, strings.TrimSpace(e.Stderr))
	}
	return fmt.Sprintf("executor: command failed: %v", e.Err)
}

func (e *ExecError) Unwrap() error { return e.Err }

// usageErrorPatterns are substrings matched case-insensitively against stderr
// to detect CLI flag/syntax errors that are worth retrying with a new generation.
var usageErrorPatterns = []string{
	"unknown flag",
	"unknown command",
	"unknown shorthand flag",
	"flag provided but not defined",
	"invalid argument",
	"invalid option",
	"error: unknown",
	"no such command",
	"bad flag syntax",
	"usage:",
}

func isUsageError(stderr string) bool {
	lower := strings.ToLower(stderr)
	for _, p := range usageErrorPatterns {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}

// Execute runs the given command string as a subprocess.
// It displays the command and explanation, optionally confirms, then runs.
func Execute(ctx context.Context, command, explanation string, requiresConfirmation bool, opts Options) error {
	// Always display the command
	Display(command, explanation)

	// Dry-run: print and exit without executing
	if opts.DryRun {
		fmt.Println("\n  [dry-run: command not executed]")
		return nil
	}

	// Confirmation required: ask the user
	if requiresConfirmation {
		if !Confirm() {
			fmt.Println("  Cancelled.")
			return nil
		}
	} else {
		fmt.Println() // spacing before output
	}

	return run(ctx, command)
}

// run executes the command string, using sh -lc when shell metacharacters are
// detected, or direct exec otherwise. Stderr is streamed to the terminal and
// also captured so callers can inspect it for usage errors.
func run(ctx context.Context, command string) error {
	var cmd *exec.Cmd
	if hasShellMetacharacters(command) {
		cmd = exec.CommandContext(ctx, "sh", "-lc", command)
	} else {
		parts := splitCommand(command)
		if len(parts) == 0 {
			return fmt.Errorf("executor: empty command")
		}
		cmd = exec.CommandContext(ctx, parts[0], parts[1:]...)
	}

	var stderrBuf bytes.Buffer
	cmd.Stdout = os.Stdout
	cmd.Stderr = io.MultiWriter(os.Stderr, &stderrBuf)
	cmd.Stdin = os.Stdin

	if err := cmd.Run(); err != nil {
		stderrStr := stderrBuf.String()
		return &ExecError{
			Cmd:          command,
			Err:          err,
			Stderr:       stderrStr,
			IsUsageError: isUsageError(stderrStr),
		}
	}

	return nil
}

// hasShellMetacharacters reports whether the command contains shell syntax
// that requires execution via sh -lc rather than direct exec.
func hasShellMetacharacters(command string) bool {
	for _, meta := range []string{"$(", "`", "|", ">", "<", "&&", "||", ";"} {
		if strings.Contains(command, meta) {
			return true
		}
	}
	return false
}

// splitCommand splits a command string into binary + args,
// respecting quoted strings.
func splitCommand(command string) []string {
	var parts []string
	var current strings.Builder
	inQuote := false
	quoteChar := rune(0)

	for _, r := range command {
		switch {
		case !inQuote && (r == '"' || r == '\''):
			inQuote = true
			quoteChar = r
		case inQuote && r == quoteChar:
			inQuote = false
			quoteChar = 0
		case !inQuote && r == ' ':
			if current.Len() > 0 {
				parts = append(parts, current.String())
				current.Reset()
			}
		default:
			current.WriteRune(r)
		}
	}

	if current.Len() > 0 {
		parts = append(parts, current.String())
	}

	return parts
}
