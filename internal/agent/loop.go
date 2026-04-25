package agent

import (
	"fmt"

	"github.com/jakejimenez/nlci/internal/definition"
)

// buildErrorContext assembles the error context string injected into retry prompts.
func buildErrorContext(command, validationError, subcommand, binary string, attempt int) string {
	var context string

	switch attempt {
	case 1:
		// First retry: inject --help for the specific subcommand
		helpText := ""
		if subcommand != "" {
			helpText = definition.SubcommandHelpText(binary, subcommand)
		}

		if helpText != "" {
			context = fmt.Sprintf(
				"The command %q is invalid: %s\n\nValid flags for %q:\n%s\n\nRegenerate using only valid flags.",
				command, validationError, subcommand, helpText,
			)
		} else {
			context = fmt.Sprintf(
				"The command %q is invalid: %s\n\nRegenerate with a corrected command.",
				command, validationError,
			)
		}

	case 2:
		// Second retry: simplify hint
		context = fmt.Sprintf(
			"Two attempts have failed. Last command %q failed with: %s\n\nGenerate the simplest possible command for this intent. Use only the most basic flags.",
			command, validationError,
		)
	}

	return context
}
