package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jakejimenez/nlci/internal/backend"
	"github.com/jakejimenez/nlci/internal/definition"
)

func TestParseInitSeedList(t *testing.T) {
	raw := "install\nservices start\n- list\n`search`\nINVALID/PATH\nInstall\n"
	got := parseInitSeedList(raw)
	want := []string{"install", "list", "search", "services start"}
	if len(got) != len(want) {
		t.Fatalf("expected %d seeds, got %d: %#v", len(want), len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("seed %d: want %q, got %q", i, want[i], got[i])
		}
	}
}

func TestWriteFlagDrivenScaffold(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "curl.nlci.yaml")
	result := &definition.FlagDiscoveryResult{
		RootFlags: []definition.Flag{{Name: "json", Description: "HTTP POST JSON"}},
		Capabilities: []definition.Capability{{Name: "request", Description: "Control request body", Flags: []string{"json"}}},
	}
	if err := writeFlagDrivenScaffold(path, "curl", result, map[string][]string{"json": {"request"}}, nil); err != nil {
		t.Fatalf("writeFlagDrivenScaffold: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read scaffold: %v", err)
	}
	text := string(data)
	for _, want := range []string{"mode: flag_driven", "root_flags:", "capabilities:", "synonyms:", "auto_discover: false"} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected scaffold to contain %q\n%s", want, text)
		}
	}
}

func TestParseEnrichmentYAML_WellFormed(t *testing.T) {
	raw := `<evidence>
- "rm: Remove containers" → destructive
</evidence>
<yaml>
description: "Manage Docker containers"
system_prompt: |
  You are a docker expert.
  Output only the raw command.
safety:
  require_confirmation:
    - "docker rm"
    - "docker rmi"
  forbidden: []
synonyms:
  list: ["ps"]
  delete: ["rm", "rmi"]
</yaml>`
	meta, err := parseEnrichmentYAML(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if meta.Description != "Manage Docker containers" {
		t.Fatalf("description: got %q", meta.Description)
	}
	if !strings.Contains(meta.SystemPrompt, "docker expert") {
		t.Fatalf("system_prompt: got %q", meta.SystemPrompt)
	}
	if len(meta.Safety.RequireConfirmation) != 2 || meta.Safety.RequireConfirmation[0] != "docker rm" {
		t.Fatalf("safety: got %+v", meta.Safety)
	}
	if len(meta.Synonyms["list"]) != 1 || meta.Synonyms["list"][0] != "ps" {
		t.Fatalf("synonyms: got %+v", meta.Synonyms)
	}
}

func TestParseEnrichmentYAML_NoBlock(t *testing.T) {
	_, err := parseEnrichmentYAML("the model forgot the yaml tags")
	if err == nil {
		t.Fatalf("expected error for missing yaml block")
	}
}

func TestParseEnrichmentYAML_EmptyBlock(t *testing.T) {
	_, err := parseEnrichmentYAML("<yaml>\n   \n</yaml>")
	if err == nil {
		t.Fatalf("expected error for empty yaml block")
	}
}

func TestParseEnrichmentYAML_RejectsPlaceholderInDescription(t *testing.T) {
	raw := `<yaml>
description: "<your-tool> CLI"
system_prompt: |
  Generic.
safety:
  require_confirmation: []
  forbidden: []
synonyms: {}
</yaml>`
	_, err := parseEnrichmentYAML(raw)
	if err == nil {
		t.Fatalf("expected rejection of placeholder description")
	}
}

func TestParseEnrichmentYAML_JunkBeforeBlock(t *testing.T) {
	raw := `Sure, here is the metadata you asked for:

<yaml>
description: "A tool"
system_prompt: "x"
safety:
  require_confirmation: []
  forbidden: []
synonyms: {}
</yaml>

That's all!`
	meta, err := parseEnrichmentYAML(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if meta.Description != "A tool" {
		t.Fatalf("description: got %q", meta.Description)
	}
}

func TestMergeSynonyms_Union(t *testing.T) {
	heur := map[string][]string{"list": {"ps"}, "issues": {"doctor"}}
	llm := map[string][]string{"list": {"ls"}, "delete": {"rm"}}
	merged := mergeSynonyms(heur, llm)
	if len(merged["list"]) != 2 {
		t.Fatalf("list should union to ps+ls, got %v", merged["list"])
	}
	if len(merged["issues"]) != 1 {
		t.Fatalf("heuristic-only key dropped: %v", merged)
	}
	if len(merged["delete"]) != 1 || merged["delete"][0] != "rm" {
		t.Fatalf("llm-only key not added: %v", merged["delete"])
	}
}

func TestMergeSynonyms_NilLLM(t *testing.T) {
	heur := map[string][]string{"list": {"ps"}}
	merged := mergeSynonyms(heur, nil)
	if len(merged) != 1 || merged["list"][0] != "ps" {
		t.Fatalf("nil llm should pass heuristic through unchanged: %+v", merged)
	}
}

func TestConvertNativeMetadata_HappyPath(t *testing.T) {
	r := &backend.MetadataResult{
		Description:  "Manage Docker things",
		SystemPrompt: "You are a docker expert.",
		Safety: backend.Safety{
			RequireConfirmation: []string{"docker rm"},
			Forbidden:           []string{},
		},
		Synonyms: map[string][]string{"list": {"ps"}},
	}
	got := convertNativeMetadata(r)
	if got == nil {
		t.Fatalf("expected conversion, got nil")
	}
	if got.Description != "Manage Docker things" {
		t.Fatalf("description: got %q", got.Description)
	}
	if len(got.Safety.RequireConfirmation) != 1 || got.Safety.RequireConfirmation[0] != "docker rm" {
		t.Fatalf("safety: got %+v", got.Safety)
	}
	if got.Synonyms["list"][0] != "ps" {
		t.Fatalf("synonyms: got %+v", got.Synonyms)
	}
}

func TestConvertNativeMetadata_RejectsPlaceholderInDescription(t *testing.T) {
	r := &backend.MetadataResult{
		Description:  "<your-tool> CLI",
		SystemPrompt: "Generic.",
	}
	if got := convertNativeMetadata(r); got != nil {
		t.Fatalf("expected nil for placeholder in description, got %+v", got)
	}
}

func TestConvertNativeMetadata_NilInput(t *testing.T) {
	if got := convertNativeMetadata(nil); got != nil {
		t.Fatalf("expected nil for nil input, got %+v", got)
	}
}
