package validator

import (
	"fmt"
	"strings"

	"github.com/jakejimenez/nlci/internal/definition"
)

// Result is the output of a validation check.
type Result struct {
	Valid               bool
	RequiresConfirmation bool
	Error               string
}

// Validate checks a generated command against the CLI definition rules.
func Validate(command string, def *definition.CLIDefinition) Result {
	command = strings.TrimSpace(command)

	if command == "" {
		return Result{Error: "model returned an empty command"}
	}

	// 1. Command must start with the registered binary name
	if !strings.HasPrefix(command, def.Binary) {
		return Result{
			Error: fmt.Sprintf("generated command %q does not start with %q", command, def.Binary),
		}
	}

	// 2. Check against forbidden patterns
	for _, forbidden := range def.Safety.Forbidden {
		if strings.Contains(command, forbidden) {
			return Result{
				Error: fmt.Sprintf("command contains forbidden pattern %q", forbidden),
			}
		}
	}

	// 3. Check whether this command requires confirmation before execution
	requiresConfirm := false
	for _, pattern := range def.Safety.RequireConfirmation {
		if strings.Contains(command, pattern) {
			requiresConfirm = true
			break
		}
	}

	return Result{
		Valid:               true,
		RequiresConfirmation: requiresConfirm,
	}
}
