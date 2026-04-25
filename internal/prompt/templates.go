package prompt

import "fmt"

// DefaultSystemPrompt returns the default system prompt for a CLI tool
// when no system_prompt is provided in the YAML definition.
func DefaultSystemPrompt(name, binary string) string {
	return fmt.Sprintf(
		`You are an expert %s administrator and CLI user.
Your job is to translate natural language intent into exact %s commands.

Rules:
- Output ONLY the raw command — no markdown, no backticks, no explanation
- The command MUST start with "%s"
- Use the most minimal flags that satisfy the intent
- If the intent is ambiguous, choose the safest interpretation`,
		name, binary, binary,
	)
}
