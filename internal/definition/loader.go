package definition

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

//go:embed bundled/*.nlci.yaml
var bundledFS embed.FS

// Load finds and loads a CLIDefinition for the given tool name.
// Search order:
//  1. <tool>.nlci.yaml in the current directory
//  2. User paths from config (typically ~/.config/nlci/definitions/)
//  3. Bundled definitions embedded in the binary
//
// If auto_discover is true (or no YAML found), --help is also parsed and merged.
func Load(toolName string, userPaths []string) (*CLIDefinition, error) {
	filename := toolName + ".nlci.yaml"

	// 1. Current directory
	if def, err := loadFile(filename); err == nil {
		return enrich(def)
	}

	// 2. User-configured paths
	for _, dir := range userPaths {
		path := filepath.Join(dir, filename)
		if def, err := loadFile(path); err == nil {
			return enrich(def)
		}
	}

	// 3. Bundled definitions
	if def, err := loadBundled(filename); err == nil {
		return enrich(def)
	}

	// 4. Zero-config: no YAML found — build a minimal definition from --help
	def := &CLIDefinition{
		Name:         toolName,
		Binary:       toolName,
		AutoDiscover: true,
	}
	return enrich(def)
}

// LoadFile loads a CLIDefinition from an explicit file path.
func LoadFile(path string) (*CLIDefinition, error) {
	def, err := loadFile(path)
	if err != nil {
		return nil, err
	}
	return enrich(def)
}

func loadFile(path string) (*CLIDefinition, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return parse(data)
}

func loadBundled(filename string) (*CLIDefinition, error) {
	data, err := bundledFS.ReadFile("bundled/" + filename)
	if err != nil {
		return nil, err
	}
	return parse(data)
}

func parse(data []byte) (*CLIDefinition, error) {
	var def CLIDefinition
	if err := yaml.Unmarshal(data, &def); err != nil {
		return nil, fmt.Errorf("definition: parse error: %w", err)
	}
	if def.Binary == "" {
		def.Binary = def.Name
	}
	return &def, nil
}

// enrich runs auto-discovery if requested and merges results into the definition.
func enrich(def *CLIDefinition) (*CLIDefinition, error) {
	if !def.AutoDiscover {
		return def, nil
	}

	discovered, err := Discover(def.Binary)
	if err != nil {
		// Non-fatal: auto-discovery is best-effort
		return def, nil
	}

	// Merge: YAML-defined commands take priority; add any new discovered ones
	existing := make(map[string]bool)
	for _, c := range def.Commands {
		existing[c.Name] = true
	}
	for _, c := range discovered {
		if !existing[c.Name] {
			def.Commands = append(def.Commands, c)
		}
	}

	return def, nil
}
