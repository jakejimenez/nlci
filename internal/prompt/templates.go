package prompt

import "fmt"

// DefaultSystemPrompt returns the default system prompt for a CLI tool
// when no system_prompt is provided in the YAML definition.
func DefaultSystemPrompt(name, binary string) string {
	return fmt.Sprintf(
		`You are an expert %s CLI user.
Your job is to translate natural language intent into exact %s commands.

Rules:
- Output ONLY the raw command — no markdown, no backticks, no explanation
- The command MUST start with "%s"
- Use the simplest flags that satisfy the intent — prefer no flags when the plain command suffices
- If the intent is ambiguous, choose the safest interpretation
- Copy identifiers (names, numbers, paths) exactly as stated — never invent placeholders
- Do not add --format, --json, or destructive flags unless explicitly requested`,
		name, binary, binary,
	)
}

// extraConstraints returns a compact set of anti-hallucination rules appended
// to every system prompt (including YAML-provided ones) so all definitions benefit.
func extraConstraints() string {
	return `- Copy any identifiers the user mentions (container names, PR numbers, repo names, branches, file paths) exactly as stated
- Never invent placeholder values such as "your-repo-url", "OWNER/REPO", "<name>", or any template tokens
- Do not add --format, --json, --output, or output-formatting flags unless the user explicitly requested a specific format
- Do not add destructive or optional flags (--volumes, --all, --force, --prune) unless the user explicitly requested them
- When an example command closely matches the intent, use that command — do not add unrequested flags`
}
