package executor

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Options controls execution behavior.
type Options struct {
	DryRun  bool
	Explain bool // always print explanation even if not requiring confirmation
}

// Execute runs the given command string as a subprocess.
// It displays the command and explanation, optionally confirms, then runs.
func Execute(command, explanation string, requiresConfirmation bool, opts Options) error {
	// Always display the command
	Display(command, explanation)

	// Dry-run: print and exit without executing
	if opts.DryRun {
		fmt.Println("\n  [dry-run: command not executed]")
		return nil
	}

	// Confirmation required: ask the user
	if requiresConfirmation {
		if !Confirm(command, "") {
			fmt.Println("  Cancelled.")
			return nil
		}
	} else {
		fmt.Println() // spacing before output
	}

	return run(command)
}

// run executes the command string as a shell subprocess,
// streaming stdout and stderr directly to the terminal.
func run(command string) error {
	parts := splitCommand(command)
	if len(parts) == 0 {
		return fmt.Errorf("executor: empty command")
	}

	cmd := exec.Command(parts[0], parts[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("executor: command failed: %w", err)
	}

	return nil
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
