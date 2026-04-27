package definition

import (
	"regexp"
	"strings"
	"testing"
)

// fabricatedValuePatterns are values the old scaffold enrichment used to
// emit literally. With Fix A in place, no generated example should contain any
// of these — they are now bracketed placeholders instead.
var fabricatedValuePatterns = []*regexp.Regexp{
	regexp.MustCompile(`example\.com`),
	regexp.MustCompile(`localhost:\d`),
	regexp.MustCompile(`\b987654321\b`),
	regexp.MustCompile(`\b\d{4,}\b`),       // any long digit run
	regexp.MustCompile(`https?://[^<\s]+`), // any concrete URL
	regexp.MustCompile(`\bripgrep\b`),
	regexp.MustCompile(`\bnginx\b`),
	regexp.MustCompile(`\bpostgresql\b`),
	regexp.MustCompile(`\bowner/repo\b`),
	regexp.MustCompile(`\bindex\.html\b`),
}

func assertNoFabricatedValues(t *testing.T, label, text string) {
	t.Helper()
	for _, re := range fabricatedValuePatterns {
		if m := re.FindString(text); m != "" {
			t.Errorf("%s contains fabricated value %q: %q", label, m, text)
		}
	}
}

func TestEnrichCommandTreeScaffoldAddsExamplesAndSynonyms(t *testing.T) {
	commands := []Command{
		{Name: "install", Description: "Install a formula or cask"},
		{Name: "doctor", Description: "Check your system for potential problems"},
	}
	enriched, synonyms := EnrichCommandTreeScaffold("brew", commands)
	if len(enriched[0].Examples) == 0 {
		t.Fatalf("expected generated examples for install: %+v", enriched[0])
	}
	if len(enriched[1].Examples) == 0 {
		t.Fatalf("expected generated examples for doctor: %+v", enriched[1])
	}
	if len(synonyms["issues"]) == 0 {
		t.Fatalf("expected generic check synonym coverage, got %+v", synonyms)
	}
}

func TestEnrichFlagDrivenScaffoldAddsExamplesAndSynonyms(t *testing.T) {
	result := &FlagDiscoveryResult{
		RootFlags: []Flag{
			{Name: "output", Short: "o", Description: "Write to file instead of stdout"},
			{Name: "location", Short: "L", Description: "Follow redirects"},
			{Name: "head", Short: "I", Description: "Show document info only"},
		},
		Capabilities: []Capability{
			{Name: "output", Description: "Control where response bodies are written", Flags: []string{"output"}},
			{Name: "redirects", Description: "Follow or shape redirect behavior", Flags: []string{"location"}},
			{Name: "headers", Description: "Inspect or send HTTP headers", Flags: []string{"head"}},
		},
	}
	enriched, synonyms := EnrichFlagDrivenScaffold("curl", result)
	if len(enriched.Capabilities[0].Examples) == 0 {
		t.Fatalf("expected generated capability examples: %+v", enriched.Capabilities[0])
	}
	if len(synonyms["follow"]) == 0 {
		t.Fatalf("expected redirects synonym coverage, got %+v", synonyms)
	}
}

func TestCommandTreeExamples_NoFabricatedValues(t *testing.T) {
	// Cover every action family the enricher knows about. After Fix A no
	// example NL or cmd should contain a concrete URL, a long digit run, or
	// any of the historic placeholder strings (`ripgrep`, `nginx`, etc.).
	commands := []Command{
		{Name: "list", Description: "List installed packages"},
		{Name: "info", Description: "Show package information"},
		{Name: "search", Description: "Search the package index"},
		{Name: "install", Description: "Install a formula or cask"},
		{Name: "uninstall", Description: "Remove a package"},
		{Name: "update", Description: "Update Homebrew"},
		{Name: "upgrade", Description: "Upgrade installed formulae"},
		{Name: "cleanup", Description: "Remove old package versions"},
		{Name: "outdated", Description: "Show outdated packages"},
		{Name: "doctor", Description: "Check the system for problems"},
		{Name: "start", Description: "Start a service"},
		{Name: "stop", Description: "Stop a service"},
		{Name: "restart", Description: "Restart a service"},
		{Name: "browse", Description: "Open the repo in the browser"},
		{Name: "logs", Description: "Show service logs"},
		{Name: "exec", Description: "Run a command in a container"},
		{Name: "download", Description: "Download a URL"},
		{Name: "upload", Description: "Upload a file"},
		// PR family triggers the "check the status of CI checks on PR ..." example.
		{Name: "pr checks", Description: "Check the status of CI checks on a pull request"},
	}
	enriched, _ := EnrichCommandTreeScaffold("brew", commands)
	for _, c := range enriched {
		for _, ex := range c.Examples {
			assertNoFabricatedValues(t, c.Name+" NL", ex.NL)
			assertNoFabricatedValues(t, c.Name+" cmd", ex.Cmd)
		}
	}
}

func TestCommandTreeExamples_UsePlaceholders(t *testing.T) {
	// At least one example for value-requiring families should contain a
	// bracketed placeholder. We don't assert the exact placeholder name —
	// just that a `<...>` token is present somewhere in the value-requiring
	// outputs, proving the substitution path is wired.
	commands := []Command{
		{Name: "install", Description: "Install a formula or cask"},
		{Name: "search", Description: "Search packages"},
		{Name: "download", Description: "Download a file"},
	}
	enriched, _ := EnrichCommandTreeScaffold("brew", commands)
	for _, c := range enriched {
		found := false
		for _, ex := range c.Examples {
			if strings.Contains(ex.Cmd, "<") && strings.Contains(ex.Cmd, ">") {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("%s: expected at least one example with a <placeholder> token, got %+v", c.Name, c.Examples)
		}
	}
}

func TestCapabilityExamples_NoFabricatedValues(t *testing.T) {
	// All curl-shaped capabilities — request/headers/auth/output/redirects/
	// transfer/proxy/tls/debugging — formerly carried hardcoded URLs and
	// bodies. Verify every emitted example is now placeholder-only.
	flags := []Flag{
		{Name: "json"},
		{Name: "data"},
		{Name: "head"},
		{Name: "header"},
		{Name: "user"},
		{Name: "output"},
		{Name: "location"},
		{Name: "upload-file", Short: "T"},
		{Name: "proxy", Short: "x"},
		{Name: "insecure", Short: "k"},
		{Name: "verbose", Short: "v"},
	}
	capabilities := []Capability{
		{Name: "request"}, {Name: "headers"}, {Name: "auth"}, {Name: "output"},
		{Name: "redirects"}, {Name: "transfer"}, {Name: "proxy"}, {Name: "tls"},
		{Name: "debugging"},
	}
	result := &FlagDiscoveryResult{RootFlags: flags, Capabilities: capabilities}
	enriched, _ := EnrichFlagDrivenScaffold("curl", result)
	for _, cap := range enriched.Capabilities {
		for _, ex := range cap.Examples {
			assertNoFabricatedValues(t, cap.Name+" NL", ex.NL)
			assertNoFabricatedValues(t, cap.Name+" cmd", ex.Cmd)
		}
	}
}
