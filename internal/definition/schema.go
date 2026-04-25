package definition

// CLIDefinition is the root structure loaded from a *.nlci.yaml file.
type CLIDefinition struct {
	Name         string            `yaml:"name"`
	Description  string            `yaml:"description"`
	Binary       string            `yaml:"binary"`
	Mode         string            `yaml:"mode"`
	SystemPrompt string            `yaml:"system_prompt"`
	Commands     []Command         `yaml:"commands"`
	RootFlags    []Flag            `yaml:"root_flags"`
	Capabilities []Capability      `yaml:"capabilities"`
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
	ValueHint   string `yaml:"value_hint,omitempty"`
	Description string `yaml:"description"`
}

// Capability is a semantic bucket used by flag-driven CLIs.
// It groups related root flags and examples without inventing fake subcommands.
type Capability struct {
	Name        string    `yaml:"name"`
	Description string    `yaml:"description"`
	Flags       []string  `yaml:"flags,omitempty"`
	Examples    []Example `yaml:"examples"`
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

// CapabilityNames returns a flat list of all capability names.
func (d *CLIDefinition) CapabilityNames() []string {
	names := make([]string, 0, len(d.Capabilities))
	for _, c := range d.Capabilities {
		names = append(names, c.Name)
	}
	return names
}

// FindCapability returns the capability matching the given name, or nil.
func (d *CLIDefinition) FindCapability(name string) *Capability {
	for i := range d.Capabilities {
		if d.Capabilities[i].Name == name {
			return &d.Capabilities[i]
		}
	}
	return nil
}
