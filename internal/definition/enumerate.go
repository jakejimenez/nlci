package definition

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// AvailableTool is the lightweight metadata used by the tool router and the
// `nlci config` listing. It is built by ListAvailableTools and intentionally
// omits the heavyweight bits of CLIDefinition (commands, capabilities, full
// system prompt) so a router prompt can include many tools cheaply.
type AvailableTool struct {
	Name        string
	Description string
	Synonyms    []string
	Source      string // "cwd" | "user:<path>" | "bundled"
}

// ListAvailableTools enumerates every tool with a curated definition reachable
// at runtime. Order matches Load's precedence: cwd, then each user path in the
// order given, then bundled. Duplicates by Name are deduped (first occurrence
// wins). The returned slice is sorted by Name for stable output.
//
// Files that fail to parse are skipped silently — a single broken user file
// must not break routing or `nlci config`.
func ListAvailableTools(userPaths []string) ([]AvailableTool, error) {
	tools := make([]AvailableTool, 0, 8)
	seen := make(map[string]bool)

	add := func(t AvailableTool) {
		if t.Name == "" || seen[t.Name] {
			return
		}
		seen[t.Name] = true
		tools = append(tools, t)
	}

	for _, t := range listFromDir(".", "cwd") {
		add(t)
	}
	for _, dir := range userPaths {
		for _, t := range listFromDir(dir, "user:"+dir) {
			add(t)
		}
	}
	for _, t := range listFromBundled() {
		add(t)
	}

	sort.Slice(tools, func(i, j int) bool { return tools[i].Name < tools[j].Name })
	return tools, nil
}

func listFromDir(dir, source string) []AvailableTool {
	matches, err := filepath.Glob(filepath.Join(dir, "*.nlci.yaml"))
	if err != nil {
		return nil
	}
	out := make([]AvailableTool, 0, len(matches))
	for _, path := range matches {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		def, err := parse(data)
		if err != nil {
			continue
		}
		out = append(out, toolFromDef(def, source))
	}
	return out
}

func listFromBundled() []AvailableTool {
	entries, err := fs.ReadDir(bundledFS, "bundled")
	if err != nil {
		return nil
	}
	out := make([]AvailableTool, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".nlci.yaml") {
			continue
		}
		data, err := bundledFS.ReadFile("bundled/" + e.Name())
		if err != nil {
			continue
		}
		def, err := parse(data)
		if err != nil {
			continue
		}
		out = append(out, toolFromDef(def, "bundled"))
	}
	return out
}

func toolFromDef(def *CLIDefinition, source string) AvailableTool {
	return AvailableTool{
		Name:        def.Name,
		Description: def.Description,
		Synonyms:    topSynonymKeys(def.Synonyms, 8),
		Source:      source,
	}
}

// topSynonymKeys returns up to max keys from the synonyms map, sorted with a
// preference for shorter keys (more likely to be intent words) and ties broken
// alphabetically. Keys with empty value lists are skipped.
func topSynonymKeys(syn map[string][]string, max int) []string {
	if len(syn) == 0 {
		return nil
	}
	keys := make([]string, 0, len(syn))
	for k, v := range syn {
		if len(v) == 0 {
			continue
		}
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if len(keys[i]) != len(keys[j]) {
			return len(keys[i]) < len(keys[j])
		}
		return keys[i] < keys[j]
	})
	if len(keys) > max {
		keys = keys[:max]
	}
	sort.Strings(keys)
	return keys
}
