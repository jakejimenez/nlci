package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

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
	if err := writeFlagDrivenScaffold(path, "curl", result, map[string][]string{"json": {"request"}}); err != nil {
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
