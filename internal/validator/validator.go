package validator

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/jakejimenez/nlci/internal/definition"
)

// placeholderREs is a list of compiled regular expressions that identify
// template/placeholder tokens the model should never emit.  Each pattern is
// checked against the full generated command; a match causes validation failure
// and triggers a retry with an anti-hallucination reminder.
var placeholderREs = []*regexp.Regexp{
	// Angle-bracket tokens: <name>, <your-value>, <OWNER>
	regexp.MustCompile(`<[^>]+>`),
	// "your-*" tokens (e.g. your-repo, your-branch, your-image-name)
	regexp.MustCompile(`\byour-\S+`),
	// ALL-CAPS/ALL-CAPS owner/repo style (e.g. OWNER/REPO, USER/REPO)
	regexp.MustCompile(`\b[A-Z]{2,}/[A-Z]{2,}\b`),
	// ALL-CAPS-HYPHEN tokens (e.g. PR-BRANCH, MY-REPO, YOUR-TAG)
	// Must be ≥2 caps segments separated by hyphens, all uppercase.
	// Excludes single ALL-CAPS words that could be env vars or legitimate names.
	regexp.MustCompile(`\b[A-Z]{2,}(-[A-Z]{2,})+\b`),
	// Square-bracket tokens: [name], [value]
	regexp.MustCompile(`\[[^\]]+\]`),
}

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

	// 2. Reject placeholder tokens — the model should never emit template values
	//    such as <name>, your-repo-url, OWNER/REPO, or [value].
	if token := findPlaceholder(command); token != "" {
		return Result{
			Error: fmt.Sprintf("command contains a placeholder token %q — use a real value instead", token),
		}
	}

	// 3. Check against forbidden patterns, anchored to word boundaries.
	for _, forbidden := range def.Safety.Forbidden {
		if containsAtBoundary(command, forbidden) {
			return Result{
				Error: fmt.Sprintf("command contains forbidden pattern %q", forbidden),
			}
		}
	}

	// 4. For flag-driven CLIs, reject obvious unknown flags against the discovered
	//    root flag surface. Positionals and values remain free-form.
	if def.Mode == "flag_driven" {
		if badFlag := findUnknownFlag(command, def.RootFlags); badFlag != "" {
			return Result{Error: fmt.Sprintf("command contains unknown flag %q", badFlag)}
		}
	}

	// 5. Check whether this command requires confirmation before execution.
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

// findPlaceholder returns the first placeholder token found in command,
// or an empty string if none are present.
func findPlaceholder(command string) string {
	for _, re := range placeholderREs {
		if m := re.FindString(command); m != "" {
			return m
		}
	}
	return ""
}

func findUnknownFlag(command string, flags []definition.Flag) string {
	if len(flags) == 0 {
		return ""
	}
	knownLong := make(map[string]struct{}, len(flags))
	knownShort := make(map[string]struct{}, len(flags))
	shortNeedsValue := make(map[string]bool, len(flags))
	for _, f := range flags {
		knownLong["--"+f.Name] = struct{}{}
		if f.Short != "" {
			key := "-" + f.Short
			knownShort[key] = struct{}{}
			shortNeedsValue[key] = f.ValueHint != ""
		}
	}
	for _, field := range strings.Fields(command) {
		if !strings.HasPrefix(field, "-") || field == "--" {
			continue
		}
		base := field
		if i := strings.Index(base, "="); i >= 0 {
			base = base[:i]
		}
		if strings.HasPrefix(base, "--") {
			if _, ok := knownLong[base]; !ok {
				return base
			}
			continue
		}
		if _, ok := knownShort[base]; ok {
			continue
		}
		if !isValidShortFlagCluster(base, knownShort, shortNeedsValue) {
			return base
		}
	}
	return ""
}

func isValidShortFlagCluster(field string, knownShort map[string]struct{}, shortNeedsValue map[string]bool) bool {
	if len(field) < 3 || !strings.HasPrefix(field, "-") || strings.HasPrefix(field, "--") {
		return false
	}
	for i := 1; i < len(field); i++ {
		key := "-" + string(field[i])
		if _, ok := knownShort[key]; !ok {
			return false
		}
		if shortNeedsValue[key] {
			// Allow attached values like -oout.txt or -XPOST.
			return true
		}
	}
	return true
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
