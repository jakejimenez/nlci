package validator

import (
	"fmt"
	"strings"

	"github.com/jakejimenez/nlci/internal/definition"
)

// Result is the output of a validation check.
type Result struct {
	Valid                bool
	RequiresConfirmation bool
	Error                string
}

// Validate checks a generated command against the CLI definition rules.
func Validate(command string, def *definition.CLIDefinition) Result {
	command = strings.TrimSpace(command)

	if command == "" {
		return Result{Error: "model returned an empty command"}
	}

	// 1. Command must start with exactly the registered binary name, followed
	//    by a space or end-of-string. HasPrefix alone would pass "dockerd ..."
	//    for a "docker" definition.
	if command != def.Binary && !strings.HasPrefix(command, def.Binary+" ") {
		return Result{
			Error: fmt.Sprintf("generated command %q does not start with %q", command, def.Binary),
		}
	}

	// 2. Check against forbidden patterns, anchored to word boundaries.
	for _, forbidden := range def.Safety.Forbidden {
		if containsAtBoundary(command, forbidden) {
			return Result{
				Error: fmt.Sprintf("command contains forbidden pattern %q", forbidden),
			}
		}
	}

	// 3. Check whether this command requires confirmation before execution.
	requiresConfirm := false
	for _, pattern := range def.Safety.RequireConfirmation {
		if containsAtBoundary(command, pattern) {
			requiresConfirm = true
			break
		}
	}

	return Result{
		Valid:                true,
		RequiresConfirmation: requiresConfirm,
	}
}

// containsAtBoundary reports whether command contains pattern such that the
// character immediately after the pattern (if any) is a space or end-of-string.
// This prevents "docker rm" matching "docker rmi" or "docker rmdir".
func containsAtBoundary(command, pattern string) bool {
	idx := strings.Index(command, pattern)
	if idx < 0 {
		return false
	}
	after := idx + len(pattern)
	// The pattern must end at the end of the command or be followed by a space.
	return after == len(command) || command[after] == ' '
}
