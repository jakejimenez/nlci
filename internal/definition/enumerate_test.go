package definition

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

func TestListAvailableTools_BundledOnly(t *testing.T) {
	cwd, _ := os.Getwd()
	tmp := t.TempDir()
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })

	tools, err := ListAvailableTools(nil)
	if err != nil {
		t.Fatalf("ListAvailableTools: %v", err)
	}
	names := toolNames(tools)
	if !contains(names, "docker") || !contains(names, "gh") {
		t.Fatalf("expected bundled docker+gh, got %v", names)
	}
	for _, tool := range tools {
		if tool.Source != "bundled" {
			t.Fatalf("expected source=bundled, got %q for %s", tool.Source, tool.Name)
		}
	}
}

func TestListAvailableTools_CwdShadowsBundled(t *testing.T) {
	cwd, _ := os.Getwd()
	tmp := t.TempDir()
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })

	if err := os.WriteFile("docker.nlci.yaml", []byte("name: docker\ndescription: local override\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	tools, err := ListAvailableTools(nil)
	if err != nil {
		t.Fatalf("ListAvailableTools: %v", err)
	}
	for _, tool := range tools {
		if tool.Name == "docker" {
			if tool.Source != "cwd" {
				t.Fatalf("expected docker to be sourced from cwd, got %q", tool.Source)
			}
			if tool.Description != "local override" {
				t.Fatalf("expected cwd override description, got %q", tool.Description)
			}
			return
		}
	}
	t.Fatalf("docker not in result")
}

func TestListAvailableTools_BrokenYAMLSkipped(t *testing.T) {
	cwd, _ := os.Getwd()
	tmp := t.TempDir()
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })

	if err := os.WriteFile("broken.nlci.yaml", []byte(":::not yaml:::"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	tools, err := ListAvailableTools(nil)
	if err != nil {
		t.Fatalf("ListAvailableTools should not error on broken yaml: %v", err)
	}
	if contains(toolNames(tools), "broken") {
		t.Fatalf("broken yaml should have been skipped")
	}
}

func TestListAvailableTools_UserPath(t *testing.T) {
	cwd, _ := os.Getwd()
	tmp := t.TempDir()
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })

	userDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(userDir, "myfoo.nlci.yaml"), []byte("name: myfoo\ndescription: from user\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	tools, err := ListAvailableTools([]string{userDir})
	if err != nil {
		t.Fatalf("ListAvailableTools: %v", err)
	}
	for _, tool := range tools {
		if tool.Name == "myfoo" {
			if tool.Source != "user:"+userDir {
				t.Fatalf("expected user source, got %q", tool.Source)
			}
			return
		}
	}
	t.Fatalf("myfoo not in result")
}

func TestTopSynonymKeys(t *testing.T) {
	syn := map[string][]string{
		"show": {"ps"},
		"a":    {"x"},
		"empty": {},
		"longest-key": {"y"},
	}
	got := topSynonymKeys(syn, 8)
	sort.Strings(got)
	want := []string{"a", "longest-key", "show"}
	if len(got) != len(want) {
		t.Fatalf("want %v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("want %v, got %v", want, got)
		}
	}
}

func toolNames(tools []AvailableTool) []string {
	out := make([]string, 0, len(tools))
	for _, t := range tools {
		out = append(out, t.Name)
	}
	return out
}

func contains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}
