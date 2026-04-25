package main

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/jakejimenez/nlci/config"
	"github.com/jakejimenez/nlci/internal/agent"
	"github.com/jakejimenez/nlci/internal/backend"
	"github.com/jakejimenez/nlci/internal/definition"
)

var (
	flagDryRun  bool
	flagExplain bool
	flagBackend string
)

func main() {
	root := &cobra.Command{
		Use:   "nlci",
		Short: "Natural language interface for any CLI tool",
		Long: `nlci wraps any CLI tool with natural language understanding.
Uses on-device inference via Apple Intelligence, Ollama, llama.cpp, or LM Studio.

Examples:
  nlci docker "show me running containers"
  nlci kubectl "restart the auth deployment in production"
  nlci docker "clean up stopped containers" --dry-run`,
	}

	root.PersistentFlags().BoolVar(&flagDryRun, "dry-run", false, "Print the generated command without executing it")
	root.PersistentFlags().BoolVar(&flagExplain, "explain", false, "Always print the command explanation")
	root.PersistentFlags().StringVar(&flagBackend, "backend", "", "Force a specific backend (apple, ollama, llamacpp, lmstudio)")

	root.AddCommand(
		newRunCmd(),
		newInitCmd(),
		newConfigCmd(),
	)

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

// newRunCmd returns the implicit "run" command — invoked as:
// nlci <tool> "<intent>"
func newRunCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "<tool> <intent>",
		Short: "Translate natural language intent to a CLI command and run it",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			toolName := args[0]
			intent := args[1]
			return runTool(cmd.Context(), toolName, intent)
		},
		// Disable the default help flag so it doesn't conflict with passing --help to tools
		DisableFlagParsing: false,
	}
	return cmd
}

func runTool(ctx context.Context, toolName, intent string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	def, err := definition.Load(toolName, cfg.Definitions.Paths)
	if err != nil {
		return fmt.Errorf("could not load definition for %q: %w", toolName, err)
	}

	br := buildBackendRouter(cfg, flagBackend)

	agentOpts := agent.Options{
		DryRun:  flagDryRun,
		Explain: flagExplain,
	}

	a := agent.New(def, br, agentOpts)
	_, err = a.Run(ctx, intent)
	return err
}

// newInitCmd scaffolds a *.nlci.yaml from a tool's --help output.
func newInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init <tool>",
		Short: "Scaffold a *.nlci.yaml definition from a tool's --help output",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			toolName := args[0]
			return runInit(toolName)
		},
	}
}

func runInit(toolName string) error {
	fmt.Printf("Discovering %s --help...\n", toolName)

	commands, err := definition.Discover(toolName)
	if err != nil {
		return fmt.Errorf("init: could not discover %q: %w", toolName, err)
	}

	def := &definition.CLIDefinition{
		Name:         toolName,
		Description:  toolName + " CLI",
		Binary:       toolName,
		AutoDiscover: true,
		Commands:     commands,
	}

	// Write scaffold YAML
	filename := toolName + ".nlci.yaml"
	if err := writeScaffold(filename, def); err != nil {
		return err
	}

	fmt.Printf("Created %s with %d discovered commands.\n", filename, len(commands))
	fmt.Printf("Next: add examples and a system_prompt to %s\n", filename)
	return nil
}

func writeScaffold(filename string, def *definition.CLIDefinition) error {
	f, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("init: create file: %w", err)
	}
	defer f.Close()

	fmt.Fprintf(f, "name: %s\n", def.Name)
	fmt.Fprintf(f, "description: %s\n", def.Description)
	fmt.Fprintf(f, "binary: %s\n", def.Binary)
	fmt.Fprintf(f, "\nsystem_prompt: |\n  You are an expert %s user. Generate precise %s commands.\n  Output only the raw command.\n", def.Name, def.Binary)
	fmt.Fprintf(f, "\ncommands:\n")

	for _, c := range def.Commands {
		fmt.Fprintf(f, "  - name: %s\n", c.Name)
		if c.Description != "" {
			fmt.Fprintf(f, "    description: %s\n", c.Description)
		}
		fmt.Fprintf(f, "    examples:\n")
		fmt.Fprintf(f, "      # - nl: \"...\"\n")
		fmt.Fprintf(f, "      #   cmd: \"%s %s ...\"\n\n", def.Binary, c.Name)
	}

	fmt.Fprintf(f, "safety:\n")
	fmt.Fprintf(f, "  require_confirmation: []\n")
	fmt.Fprintf(f, "  forbidden: []\n")
	fmt.Fprintf(f, "\nauto_discover: true\n")

	return nil
}

// newConfigCmd shows current configuration and backend status.
func newConfigCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "config",
		Short: "Show current configuration and backend health",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConfig(cmd.Context())
		},
	}
}

func runConfig(ctx context.Context) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	fmt.Println("Backend priority:", cfg.Backend.Priority)
	fmt.Printf("Ollama:    %s (model: %s)\n", cfg.Backend.Ollama.Host, cfg.Backend.Ollama.Model)
	fmt.Printf("llama.cpp: %s\n", cfg.Backend.LlamaCpp.Host)
	fmt.Printf("LM Studio: %s\n", cfg.Backend.LMStudio.Host)
	fmt.Printf("nlci-apple binary: %s\n\n", cfg.Apple.Binary)

	fmt.Println("Backend health:")
	br := buildBackendRouter(cfg, "")
	for name, err := range br.Status(ctx) {
		if err == nil {
			fmt.Printf("  %-12s healthy\n", name)
		} else {
			fmt.Printf("  %-12s unavailable (%s)\n", name, err)
		}
	}

	return nil
}

func buildBackendRouter(cfg config.Config, forceName string) *backend.Router {
	if forceName != "" {
		switch forceName {
		case "apple":
			return backend.NewRouterWithBackends([]backend.Backend{
				backend.NewApple(cfg.Apple.Binary),
			})
		case "ollama":
			return backend.NewRouterWithBackends([]backend.Backend{
				backend.NewOllama(cfg.Backend.Ollama.Host, cfg.Backend.Ollama.Model),
			})
		case "llamacpp":
			return backend.NewRouterWithBackends([]backend.Backend{
				backend.NewLlamaCpp(cfg.Backend.LlamaCpp.Host, cfg.Backend.LlamaCpp.Model),
			})
		case "lmstudio":
			return backend.NewRouterWithBackends([]backend.Backend{
				backend.NewLMStudio(cfg.Backend.LMStudio.Host, cfg.Backend.LMStudio.Model),
			})
		}
	}
	return backend.NewRouter(cfg)
}
