package definition

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsCurated(t *testing.T) {
	// Bundled tool — should always resolve.
	if !IsCurated("docker", nil) {
		t.Fatalf("expected bundled docker to be curated")
	}
	if !IsCurated("gh", nil) {
		t.Fatalf("expected bundled gh to be curated")
	}

	// Unknown tool with no user paths and no cwd file — not curated.
	if IsCurated("definitely-not-a-real-tool", nil) {
		t.Fatalf("expected unknown tool to be uncurated")
	}

	// User-path hit.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "myfoo.nlci.yaml"), []byte("name: myfoo\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if !IsCurated("myfoo", []string{dir}) {
		t.Fatalf("expected user-path file to register as curated")
	}
	if IsCurated("missing", []string{dir}) {
		t.Fatalf("expected absent user-path file to not be curated")
	}

	// Cwd hit.
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmpCwd := t.TempDir()
	if err := os.Chdir(tmpCwd); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })

	if err := os.WriteFile("local-only.nlci.yaml", []byte("name: local-only\n"), 0o644); err != nil {
		t.Fatalf("write cwd: %v", err)
	}
	if !IsCurated("local-only", nil) {
		t.Fatalf("expected cwd file to register as curated")
	}
}
