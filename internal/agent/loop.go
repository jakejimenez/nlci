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
		// First retry: inject a structured flag list for the top candidate.
		// We use DiscoverSubcommand + FormatFlags instead of raw compressHelpText
		// to avoid the ambiguity that caused the model to generate bad flags
		// (e.g. "docker ps -f json" from seeing "-f" and "json" in the same
		// multi-line --format description).
		flagsText := ""
		if len(candidates) > 0 {
			flags, err := definition.DiscoverSubcommand(binary, candidates[0].Command.Name)
			if err == nil && len(flags) > 0 {
				flagsText = definition.FormatFlags(flags)
			}
		}
		if flagsText != "" {
			return fmt.Sprintf(
				"The command %q is invalid: %s\n\nExact flags for %q:\n%s\n\nRegenerate using only flags from the list above.",
				command, validationError, candidates[0].Command.Name, flagsText,
			)
		}
		return fmt.Sprintf(
			"The command %q is invalid: %s\n\nRegenerate with a corrected command.",
			command, validationError,
		)

	case 2:
		// Second retry: ask for the absolute minimum.
		return fmt.Sprintf(
			"Two attempts have failed. Last command %q failed with: %s\n\nGenerate the simplest possible command for this intent. Use only the most basic flags.",
			command, validationError,
		)
	}

	return ""
}
