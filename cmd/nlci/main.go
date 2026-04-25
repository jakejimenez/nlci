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
		Use:   "nlci <tool> \"<intent>\"",
		Short: "Natural language interface for any CLI tool",
		Long: `nlci wraps any CLI tool with natural language understanding.
Uses on-device inference via Apple Intelligence, Ollama, llama.cpp, or LM Studio.

Examples:
  nlci docker "show me running containers"
  nlci gh "list my open pull requests"
  nlci docker "clean up stopped containers" --dry-run
  nlci gh "create a draft PR" --explain`,
		// ArbitraryArgs lets unknown subcommand names (tool names) fall through
		// to this RunE instead of erroring.
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) < 2 {
				return cmd.Help()
			}
			toolName := args[0]
			intent := args[1]
			return runTool(cmd.Context(), toolName, intent)
		},
		// Silence usage on runtime errors — don't print full help on inference failure
		SilenceUsage: true,
	}

	root.PersistentFlags().BoolVar(&flagDryRun, "dry-run", false, "Print the generated command without executing it")
	root.PersistentFlags().BoolVar(&flagExplain, "explain", false, "Always print the command explanation")
	root.PersistentFlags().StringVar(&flagBackend, "backend", "", "Force a specific backend (apple, ollama, llamacpp, lmstudio)")

	root.AddCommand(
		newInitCmd(),
		newConfigCmd(),
	)

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

func runTool(ctx context.Context, toolName, intent string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	def, err := definition.Load(toolName, cfg.Definitions.Paths)
	if err != nil {
		return fmt.Errorf("could not load definition for %q: %w\n\nRun 'nlci init %s' to scaffold a definition", toolName, err, toolName)
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
		Use:          "init <tool>",
		Short:        "Scaffold a *.nlci.yaml definition from a tool's --help output",
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runInit(args[0])
		},
	}
}

func runInit(toolName string) error {
	fmt.Printf("Discovering %s --help...\n", toolName)

	commands, err := definition.Discover(toolName)
	if err != nil {
		return fmt.Errorf("init: could not discover %q: %w", toolName, err)
	}

	filename := toolName + ".nlci.yaml"

	// Don't overwrite an existing definition
	if _, err := os.Stat(filename); err == nil {
		return fmt.Errorf("init: %s already exists — delete it first to re-scaffold", filename)
	}

	if err := writeScaffold(filename, toolName, commands); err != nil {
		return err
	}

	fmt.Printf("Created %s with %d discovered commands.\n", filename, len(commands))
	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Printf("  1. Add examples to each command in %s\n", filename)
	fmt.Printf("  2. Write a system_prompt describing the tool\n")
	fmt.Printf("  3. Run: nlci %s \"<your intent>\" --dry-run\n", toolName)
	return nil
}

func writeScaffold(filename, toolName string, commands []definition.Command) error {
	f, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("init: create %s: %w", filename, err)
	}
	defer f.Close()

	fmt.Fprintf(f, "name: %s\n", toolName)
	fmt.Fprintf(f, "description: %s CLI\n", toolName)
	fmt.Fprintf(f, "binary: %s\n", toolName)
	fmt.Fprintf(f, "\nsystem_prompt: |\n")
	fmt.Fprintf(f, "  You are an expert %s user. Generate precise %s commands.\n", toolName, toolName)
	fmt.Fprintf(f, "  Output only the raw command — no markdown, no explanation.\n")
	fmt.Fprintf(f, "\ncommands:\n")

	for _, c := range commands {
		fmt.Fprintf(f, "  - name: %s\n", c.Name)
		if c.Description != "" {
			fmt.Fprintf(f, "    description: %s\n", c.Description)
		}
		fmt.Fprintf(f, "    examples:\n")
		fmt.Fprintf(f, "      # - nl: \"...\"\n")
		fmt.Fprintf(f, "      #   cmd: \"%s %s ...\"\n\n", toolName, c.Name)
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
		Use:          "config",
		Short:        "Show current configuration and backend health",
		SilenceUsage: true,
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

	fmt.Println("Configuration")
	fmt.Println("─────────────────────────────────────")
	fmt.Printf("  Backend priority:  %v\n", cfg.Backend.Priority)
	fmt.Printf("  Ollama:            %s  (model: %s)\n", cfg.Backend.Ollama.Host, cfg.Backend.Ollama.Model)
	fmt.Printf("  llama.cpp:         %s\n", cfg.Backend.LlamaCpp.Host)
	fmt.Printf("  LM Studio:         %s\n", cfg.Backend.LMStudio.Host)
	fmt.Printf("  nlci-apple:        %s\n", cfg.Apple.Binary)
	fmt.Println()

	fmt.Println("Backend Health")
	fmt.Println("─────────────────────────────────────")
	br := buildBackendRouter(cfg, "")
	status := br.Status(ctx)
	for _, name := range cfg.Backend.Priority {
		err := status[name]
		if err == nil {
			fmt.Printf("  %-12s  healthy\n", name)
		} else {
			fmt.Printf("  %-12s  unavailable\n", name)
		}
	}
	fmt.Println()

	fmt.Println("Bundled Definitions")
	fmt.Println("─────────────────────────────────────")
	fmt.Println("  docker   gh")
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
