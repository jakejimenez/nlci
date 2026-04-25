package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"regexp"
	"sort"
	"strings"
	"syscall"
	"time"

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
	initSeedPartRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)
)

func main() {
	// Signal-aware root context: Ctrl-C cancels inference and execution cleanly.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

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
			if len(args) < 1 {
				return cmd.Help()
			}
			toolName := args[0]
			// Join remaining args so unquoted multi-word intents work:
			//   nlci docker show me running containers
			// is equivalent to:
			//   nlci docker "show me running containers"
			if len(args) < 2 {
				return cmd.Help()
			}
			intent := strings.Join(args[1:], " ")
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

	if err := root.ExecuteContext(ctx); err != nil {
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
			return runInit(cmd.Context(), args[0])
		},
	}
}

func runInit(ctx context.Context, toolName string) error {
	fmt.Printf("Discovering %s --help...\n", toolName)

	filename := toolName + ".nlci.yaml"

	// Don't overwrite an existing definition
	if _, err := os.Stat(filename); err == nil {
		return fmt.Errorf("init: %s already exists — delete it first to re-scaffold", filename)
	}

	result, err := definition.DiscoverDetailed(toolName)
	if err != nil {
		return fmt.Errorf("init: could not discover %q: %w", toolName, err)
	}

	mode := "command_tree"
	quality := definition.AssessDiscoveryQuality(result)
	inferredCount := 0
	if quality.Weak {
		if flagResult, flagErr := definition.DiscoverFlagDriven(toolName); flagErr == nil && !flagResult.Weak {
			mode = "flag_driven"
			if err := writeFlagDrivenScaffold(filename, toolName, flagResult); err != nil {
				return err
			}
			fmt.Printf("Created %s with %d verified root flags across %d capabilities.\n", filename, len(flagResult.RootFlags), len(flagResult.Capabilities))
			fmt.Printf("  Discovery mode: %s\n", mode)
			fmt.Printf("  Discovery quality: %.1f/100\n", flagResult.QualityScore)
			fmt.Println()
			fmt.Println("Next steps:")
			fmt.Printf("  1. Add draft examples to capabilities in %s\n", filename)
			fmt.Printf("  2. Write a system_prompt describing the tool\n")
			fmt.Printf("  3. Run: nlci %s \"<your intent>\" --dry-run\n", toolName)
			return nil
		}

		cfg, cfgErr := config.Load()
		if cfgErr == nil {
			fmt.Println("Deterministic discovery is weak; trying inference-assisted probing...")
			inferCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
			if enriched, count, inferErr := enrichInitDiscovery(inferCtx, cfg, toolName, result); inferErr == nil {
				result = enriched
				inferredCount = count
				quality = definition.AssessDiscoveryQuality(result)
			} else {
				fmt.Printf("Inference-assisted probing skipped: %v\n", inferErr)
			}
			cancel()
		}
	}

	if err := writeScaffold(filename, toolName, result.Commands); err != nil {
		return err
	}

	fmt.Printf("Created %s with %d verified commands.\n", filename, len(result.Commands))
	if inferredCount > 0 {
		fmt.Printf("  Inference-assisted probing verified %d additional commands.\n", inferredCount)
	}
	fmt.Printf("  Discovery mode: %s\n", mode)
	fmt.Printf("  Discovery quality: %.1f/100\n", quality.Score)
	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Printf("  1. Add examples to each command in %s\n", filename)
	fmt.Printf("  2. Write a system_prompt describing the tool\n")
	fmt.Printf("  3. Run: nlci %s \"<your intent>\" --dry-run\n", toolName)
	return nil
}

func enrichInitDiscovery(ctx context.Context, cfg config.Config, toolName string, base *definition.DiscoveryResult) (*definition.DiscoveryResult, int, error) {
	if base == nil {
		base = &definition.DiscoveryResult{}
	}

	br := buildBackendRouter(cfg, flagBackend)
	seedPrompt := buildInitInferencePrompt(toolName, base)
	resp, err := br.Generate(ctx, backend.Request{
		System: initInferenceSystemPrompt(toolName),
		Intent: seedPrompt,
	})
	if err != nil {
		return base, 0, err
	}

	seedNames := parseInitSeedList(resp.Command)
	if len(seedNames) == 0 {
		return base, 0, fmt.Errorf("inference returned no candidate command paths")
	}

	seeds := make([]definition.Command, 0, len(seedNames))
	for _, name := range seedNames {
		seeds = append(seeds, definition.Command{Name: name})
	}

	enriched, err := definition.DiscoverFromSeeds(toolName, seeds, definition.DefaultDiscoveryOptions())
	if err != nil {
		return base, 0, err
	}

	before := len(base.Commands)
	merged := &definition.DiscoveryResult{
		Commands: definitionCommandsMerge(base.Commands, enriched.Commands),
		RootHelp: base.RootHelp,
		Metrics:  enriched.Metrics,
	}
	if merged.RootHelp == "" {
		merged.RootHelp = enriched.RootHelp
	}
	quality := definition.AssessDiscoveryQuality(merged)
	merged.Metrics.QualityScore = quality.Score
	return merged, len(merged.Commands) - before, nil
}

func buildInitInferencePrompt(toolName string, base *definition.DiscoveryResult) string {
	var b strings.Builder
	b.WriteString("Root help summary:\n")
	b.WriteString(base.RootHelp)
	b.WriteString("\n\nAlready verified commands:\n")
	if len(base.Commands) == 0 {
		b.WriteString("  (none)\n")
	} else {
		for _, c := range base.Commands {
			if c.Description != "" {
				b.WriteString(fmt.Sprintf("  %s: %s\n", c.Name, c.Description))
			} else {
				b.WriteString(fmt.Sprintf("  %s\n", c.Name))
			}
		}
	}
	b.WriteString("\nReturn the most likely real command paths for ")
	b.WriteString(toolName)
	b.WriteString(" that should be probed next. One path per line, lowercase, no explanations.")
	return b.String()
}

func initInferenceSystemPrompt(toolName string) string {
	return strings.TrimSpace(fmt.Sprintf(`You are helping discover the command surface of the %s CLI.
Return ONLY newline-separated command paths to probe next.
Rules:
- Output raw command paths only, one per line
- Do not include the binary name %q
- Prefer common real subcommands and nested paths
- Never invent placeholders, examples, or explanations
- Suggest at most 20 paths`, toolName, toolName))
}

func parseInitSeedList(raw string) []string {
	seen := make(map[string]bool)
	var result []string
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		line = strings.Trim(line, "`\"")
		line = strings.TrimPrefix(line, "-")
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Fields(strings.ToLower(line))
		if len(parts) == 0 {
			continue
		}
		ok := true
		for _, part := range parts {
			if !initSeedPartRe.MatchString(part) {
				ok = false
				break
			}
		}
		if !ok {
			continue
		}
		line = strings.Join(parts, " ")
		if !seen[line] {
			seen[line] = true
			result = append(result, line)
		}
	}
	sort.Strings(result)
	return result
}

func definitionCommandsMerge(base, additions []definition.Command) []definition.Command {
	result := append([]definition.Command(nil), base...)
	index := make(map[string]int, len(result))
	for i, cmd := range result {
		index[cmd.Name] = i
	}
	for _, cmd := range additions {
		if i, ok := index[cmd.Name]; ok {
			if result[i].Description == "" {
				result[i].Description = cmd.Description
			}
			if len(result[i].Flags) == 0 {
				result[i].Flags = append([]definition.Flag(nil), cmd.Flags...)
			}
			continue
		}
		index[cmd.Name] = len(result)
		result = append(result, cmd)
	}
	return result
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
	fmt.Fprintf(f, "mode: command_tree\n")
	fmt.Fprintf(f, "\nsystem_prompt: |\n")
	fmt.Fprintf(f, "  You are an expert %s user. Generate precise %s commands.\n", toolName, toolName)
	fmt.Fprintf(f, "  Output only the raw command — no markdown, no explanation.\n")
	fmt.Fprintf(f, "\ncommands:\n")

	for _, c := range commands {
		fmt.Fprintf(f, "  - name: %s\n", yamlQuote(c.Name))
		if c.Description != "" {
			fmt.Fprintf(f, "    description: %s\n", yamlQuote(c.Description))
		}
		fmt.Fprintf(f, "    examples:\n")
		fmt.Fprintf(f, "      # - nl: \"...\"\n")
		fmt.Fprintf(f, "      #   cmd: \"%s %s ...\"\n\n", toolName, c.Name)
	}

	fmt.Fprintf(f, "safety:\n")
	fmt.Fprintf(f, "  require_confirmation: []\n")
	fmt.Fprintf(f, "  forbidden: []\n")
	// init already writes a verified command inventory, so keep runtime loading
	// fast by default. Users can opt back into live discovery manually.
	fmt.Fprintf(f, "\nauto_discover: false\n")
	return nil
}

func writeFlagDrivenScaffold(filename, toolName string, result *definition.FlagDiscoveryResult) error {
	f, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("init: create %s: %w", filename, err)
	}
	defer f.Close()

	fmt.Fprintf(f, "name: %s\n", toolName)
	fmt.Fprintf(f, "description: %s CLI\n", toolName)
	fmt.Fprintf(f, "binary: %s\n", toolName)
	fmt.Fprintf(f, "mode: flag_driven\n")
	fmt.Fprintf(f, "\nsystem_prompt: |\n")
	fmt.Fprintf(f, "  You are an expert %s user. Generate precise %s commands.\n", toolName, toolName)
	fmt.Fprintf(f, "  This tool is flag-driven: build commands from flags and positional arguments, not subcommands.\n")
	fmt.Fprintf(f, "  Output only the raw command — no markdown, no explanation.\n")

	fmt.Fprintf(f, "\nroot_flags:\n")
	for _, flag := range result.RootFlags {
		fmt.Fprintf(f, "  - name: %s\n", yamlQuote(flag.Name))
		if flag.Short != "" {
			fmt.Fprintf(f, "    short: %s\n", yamlQuote(flag.Short))
		}
		if flag.ValueHint != "" {
			fmt.Fprintf(f, "    value_hint: %s\n", yamlQuote(flag.ValueHint))
		}
		if flag.Description != "" {
			fmt.Fprintf(f, "    description: %s\n", yamlQuote(flag.Description))
		}
	}

	fmt.Fprintf(f, "\ncapabilities:\n")
	for _, cap := range result.Capabilities {
		fmt.Fprintf(f, "  - name: %s\n", yamlQuote(cap.Name))
		if cap.Description != "" {
			fmt.Fprintf(f, "    description: %s\n", yamlQuote(cap.Description))
		}
		if len(cap.Flags) > 0 {
			fmt.Fprintf(f, "    flags:\n")
			for _, flagName := range cap.Flags {
				fmt.Fprintf(f, "      - %s\n", yamlQuote(flagName))
			}
		}
		fmt.Fprintf(f, "    examples:\n")
		fmt.Fprintf(f, "      # - nl: \"...\"\n")
		fmt.Fprintf(f, "      #   cmd: \"%s ...\"\n", toolName)
	}

	fmt.Fprintf(f, "\nsafety:\n")
	fmt.Fprintf(f, "  require_confirmation: []\n")
	fmt.Fprintf(f, "  forbidden: []\n")
	fmt.Fprintf(f, "\nauto_discover: false\n")
	return nil
}

// yamlQuote wraps s in double quotes if it contains YAML special characters.
// Command names and descriptions from --help output can contain colons,
// brackets, or other characters that would break bare YAML scalars.
func yamlQuote(s string) string {
	needsQuote := false
	for _, special := range []string{":", "#", "&", "*", "!", "|", ">", "{", "}", "[", "]", ","} {
		if strings.Contains(s, special) {
			needsQuote = true
			break
		}
	}
	if needsQuote || s == "" {
		return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
	}
	return s
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
