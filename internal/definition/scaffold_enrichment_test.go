package definition

import "testing"

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
