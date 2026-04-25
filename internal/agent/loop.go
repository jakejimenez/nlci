package agent

import (
	"fmt"

	"github.com/jakejimenez/nlci/internal/definition"
	"github.com/jakejimenez/nlci/internal/retrieval"
)

// buildErrorContext assembles the error context string injected into retry prompts
// after a validation failure during the inference loop.
func buildErrorContext(command, validationError string, candidates []retrieval.Candidate, binary string, attempt int) string {
	switch attempt {
	case 1:
		// First retry: inject live --help for the top candidate to expose exact flags.
		helpText := ""
		if len(candidates) > 0 {
			helpText = definition.SubcommandHelpText(binary, candidates[0].Command.Name)
		}
		if helpText != "" {
			return fmt.Sprintf(
				"The command %q is invalid: %s\n\nValid flags for %q:\n%s\n\nRegenerate using only valid flags.",
				command, validationError, candidates[0].Command.Name, helpText,
			)
		}
		return fmt.Sprintf(
			"The command %q is invalid: %s\n\nRegenerate with a corrected command.",
			command, validationError,
		)

	case 2:
		// Second retry: simplify the hint, ask for the most basic form.
		return fmt.Sprintf(
			"Two attempts have failed. Last command %q failed with: %s\n\nGenerate the simplest possible command for this intent. Use only the most basic flags.",
			command, validationError,
		)
	}

	return ""
}
