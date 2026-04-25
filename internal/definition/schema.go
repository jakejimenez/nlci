package definition

// CLIDefinition is the root structure loaded from a *.nlci.yaml file.
type CLIDefinition struct {
	Name         string            `yaml:"name"`
	Description  string            `yaml:"description"`
	Binary       string            `yaml:"binary"`
	SystemPrompt string            `yaml:"system_prompt"`
	Commands     []Command         `yaml:"commands"`
	Safety       Safety            `yaml:"safety"`
	Synonyms     map[string][]string `yaml:"synonyms"`
	AutoDiscover bool              `yaml:"auto_discover"`
}

// Command represents a single CLI subcommand exposed to the natural language layer.
type Command struct {
	Name        string    `yaml:"name"`
	Description string    `yaml:"description"`
	Examples    []Example `yaml:"examples"`
	Flags       []Flag    `yaml:"flags"` // populated by auto-discovery
}

// Example is a natural language → command pair used as a few-shot example.
type Example struct {
	NL  string `yaml:"nl"`
	Cmd string `yaml:"cmd"`
}

// Flag represents a CLI flag discovered from --help output.
type Flag struct {
	Name        string `yaml:"name"`
	Short       string `yaml:"short"`
	Description string `yaml:"description"`
}

// Safety contains rules for command confirmation and forbidden patterns.
type Safety struct {
	RequireConfirmation []string `yaml:"require_confirmation"`
	Forbidden           []string `yaml:"forbidden"`
}

// SubcommandNames returns a flat list of all command names in the definition.
func (d *CLIDefinition) SubcommandNames() []string {
	names := make([]string, 0, len(d.Commands))
	for _, c := range d.Commands {
		names = append(names, c.Name)
	}
	return names
}

// FindCommand returns the command matching the given name, or nil.
func (d *CLIDefinition) FindCommand(name string) *Command {
	for i := range d.Commands {
		if d.Commands[i].Name == name {
			return &d.Commands[i]
		}
	}
	return nil
}
