package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
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
	"github.com/jakejimenez/nlci/internal/router"
)

var (
	flagDryRun  bool
	flagExplain bool
	flagYes     bool
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
			return dispatch(cmd, args)
		},
		// Silence usage on runtime errors — don't print full help on inference failure
		SilenceUsage: true,
	}

	root.PersistentFlags().BoolVar(&flagDryRun, "dry-run", false, "Print the generated command without executing it")
	root.PersistentFlags().BoolVar(&flagExplain, "explain", false, "Always print the command explanation")
	root.PersistentFlags().BoolVarP(&flagYes, "yes", "y", false, "Skip the y/N prompt and run the command immediately")
	root.PersistentFlags().StringVar(&flagBackend, "backend", "", "Force a specific backend (apple, ollama, llamacpp, lmstudio)")

	root.AddCommand(
		newInitCmd(),
		newAskCmd(),
		newConfigCmd(),
	)

	if err := root.ExecuteContext(ctx); err != nil {
		os.Exit(1)
	}
}

// dispatch decides which mode an invocation falls into:
//
//   - help: no args, or bare tool name with no intent
//   - routing: first arg is not a known tool/binary, or single arg has whitespace
//   - classic with auto-init: first arg is a known tool or installed binary
//
// The "is it a binary?" check (exec.LookPath) disambiguates unquoted multi-
// word intents (e.g., `nlci show me my containers` → routing) from classic
// dispatch (e.g., `nlci docker ps`) without forcing the user to quote.
func dispatch(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	if len(args) == 0 {
		return cmd.Help()
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	if len(args) == 1 {
		single := args[0]
		if strings.ContainsAny(single, " \t") {
			return runRouted(ctx, cfg, single)
		}
		if isKnownTool(single, cfg) {
			// Bare tool name with no intent — show help.
			return cmd.Help()
		}
		// Single-word natural-language intent (e.g., `nlci status`).
		return runRouted(ctx, cfg, single)
	}

	// len(args) >= 2
	first := args[0]
	if isKnownTool(first, cfg) {
		intent := strings.Join(args[1:], " ")
		return runToolWithAutoInit(ctx, cfg, first, intent)
	}
	// First arg is neither a curated tool nor a binary in PATH — treat the
	// whole thing as a natural-language intent (e.g., `nlci show me my pull
	// requests`).
	return runRouted(ctx, cfg, strings.Join(args, " "))
}

// looksLikeRoutingIntent reports whether dispatch would send args to the
// router. Pure helper, broken out for table-testing.
func looksLikeRoutingIntent(args []string, knownTool func(string) bool) bool {
	if len(args) == 0 {
		return false
	}
	if len(args) == 1 {
		if strings.ContainsAny(args[0], " \t") {
			return true
		}
		return !knownTool(args[0])
	}
	return !knownTool(args[0])
}

// isKnownTool returns true if name is a curated tool definition or a binary
// found on PATH. Both signal "user means this as a tool name."
func isKnownTool(name string, cfg config.Config) bool {
	if definition.IsCurated(name, cfg.Definitions.Paths) {
		return true
	}
	if _, err := exec.LookPath(name); err == nil {
		return true
	}
	return false
}

// runToolWithAutoInit ensures a curated definition exists for toolName,
// scaffolding one into the user config dir if not, then runs the normal
// translation pipeline.
func runToolWithAutoInit(ctx context.Context, cfg config.Config, toolName, intent string) error {
	if !definition.IsCurated(toolName, cfg.Definitions.Paths) {
		if err := autoInit(ctx, cfg, toolName); err != nil {
			return err
		}
	}
	return runTool(ctx, toolName, intent)
}

// runRouted asks the LLM to pick a tool for the given intent, then dispatches
// through the auto-init path so a routed-to-uncurated tool gets scaffolded
// transparently.
func runRouted(ctx context.Context, cfg config.Config, intent string) error {
	tools, err := definition.ListAvailableTools(cfg.Definitions.Paths)
	if err != nil {
		return err
	}
	if len(tools) == 0 {
		return fmt.Errorf("nlci: no tool definitions available — run `nlci init <tool>` to scaffold one, or invoke a tool directly: nlci <tool> \"<intent>\"")
	}

	br := buildBackendRouter(cfg, flagBackend)
	metas := router.MetasFromAvailable(tools)

	fmt.Fprintln(os.Stderr, "Routing intent to a tool…")
	toolName, err := router.Route(ctx, intent, metas, br)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "→ %s\n", toolName)

	return runToolWithAutoInit(ctx, cfg, toolName, intent)
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
		DryRun:      flagDryRun,
		Explain:     flagExplain,
		AutoConfirm: flagYes,
	}

	a := agent.New(def, br, agentOpts)
	_, err = a.Run(ctx, intent)
	return err
}

// newAskCmd is the explicit routing entry point. Useful for scripts that want
// to bypass the implicit-routing heuristic, or for single-word intents where
// the heuristic would treat the word as a tool name.
func newAskCmd() *cobra.Command {
	return &cobra.Command{
		Use:          "ask <intent...>",
		Short:        "Route a natural-language intent to a tool and run it",
		Args:         cobra.MinimumNArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			return runRouted(cmd.Context(), cfg, strings.Join(args, " "))
		},
	}
}

// autoInit scaffolds a definition into the user config dir for an uncurated
// tool. Silent except for one stderr line at start and one at completion.
// Errors out cleanly if the tool binary isn't on PATH.
func autoInit(ctx context.Context, cfg config.Config, toolName string) error {
	if _, err := exec.LookPath(toolName); err != nil {
		return fmt.Errorf("nlci: %q is not installed (not found in PATH).\n  Install it first, or run `nlci init %s` after installing.", toolName, toolName)
	}

	targetDir, err := autoInitTargetDir(cfg)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return fmt.Errorf("nlci: could not create %s: %w", targetDir, err)
	}

	fmt.Fprintf(os.Stderr, "First time using %s — generating definition…\n", toolName)

	br := buildBackendRouter(cfg, flagBackend)
	initCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	path, err := scaffoldDefinition(initCtx, toolName, scaffoldOptions{
		TargetDir:     targetDir,
		Quiet:         true,
		BackendRouter: br,
	})
	if err != nil {
		return fmt.Errorf("nlci: auto-init failed for %q: %w", toolName, err)
	}
	fmt.Fprintf(os.Stderr, "Saved definition to %s\n", path)
	return nil
}

// autoInitTargetDir picks the destination for a scaffolded definition: the
// first user-configured definitions path, or ~/.config/nlci/definitions/.
func autoInitTargetDir(cfg config.Config) (string, error) {
	for _, p := range cfg.Definitions.Paths {
		if p != "" {
			return p, nil
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("nlci: could not resolve home directory: %w", err)
	}
	return filepath.Join(home, ".config", "nlci", "definitions"), nil
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

// scaffoldOptions controls scaffold generation. TargetDir == "" writes to cwd
// (matching the original `nlci init` behavior); a non-empty value writes there
// (used by auto-init to persist into the user config dir). Quiet suppresses
// stdout chatter — callers like auto-init emit a single stderr line instead.
// BackendRouter is reused when supplied so auto-init doesn't re-resolve the
// backend a second time.
type scaffoldOptions struct {
	TargetDir     string
	Quiet         bool
	BackendRouter *backend.Router
}

func runInit(ctx context.Context, toolName string) error {
	_, err := scaffoldDefinition(ctx, toolName, scaffoldOptions{TargetDir: "", Quiet: false})
	return err
}

// scaffoldDefinition is the shared core behind both `nlci init` and auto-init.
// It runs deterministic discovery, falls back to flag-driven mode when weak,
// optionally runs inference-assisted probing, and writes the resulting YAML
// scaffold. Returns the path of the file written.
func scaffoldDefinition(ctx context.Context, toolName string, opts scaffoldOptions) (string, error) {
	if !opts.Quiet {
		fmt.Printf("Discovering %s --help...\n", toolName)
	}

	filename := filepath.Join(opts.TargetDir, toolName+".nlci.yaml")

	// Don't overwrite an existing definition.
	if _, err := os.Stat(filename); err == nil {
		return "", fmt.Errorf("init: %s already exists — delete it first to re-scaffold", filename)
	}

	result, err := definition.DiscoverDetailed(toolName)
	if err != nil {
		return "", fmt.Errorf("init: could not discover %q: %w", toolName, err)
	}

	mode := "command_tree"
	quality := definition.AssessDiscoveryQuality(result)
	inferredCount := 0
	if quality.Weak {
		if flagResult, flagErr := definition.DiscoverFlagDriven(toolName); flagErr == nil && !flagResult.Weak {
			enrichedFlagResult, synonyms := definition.EnrichFlagDrivenScaffold(toolName, flagResult)
			mode = "flag_driven"
			if err := writeFlagDrivenScaffold(filename, toolName, enrichedFlagResult, synonyms); err != nil {
				return "", err
			}
			if !opts.Quiet {
				fmt.Printf("Created %s with %d verified root flags across %d capabilities.\n", filename, len(enrichedFlagResult.RootFlags), len(enrichedFlagResult.Capabilities))
				fmt.Printf("  Discovery mode: %s\n", mode)
				fmt.Printf("  Discovery quality: %.1f/100\n", enrichedFlagResult.QualityScore)
				fmt.Println()
				fmt.Println("Next steps:")
				fmt.Printf("  1. Review the generated examples and synonyms in %s\n", filename)
				fmt.Printf("  2. Write a system_prompt describing the tool\n")
				fmt.Printf("  3. Run: nlci %s \"<your intent>\" --dry-run\n", toolName)
			}
			return filename, nil
		}

		br := opts.BackendRouter
		if br == nil {
			if cfg, cfgErr := config.Load(); cfgErr == nil {
				br = buildBackendRouter(cfg, flagBackend)
			}
		}
		if br != nil {
			if !opts.Quiet {
				fmt.Println("Deterministic discovery is weak; trying inference-assisted probing...")
			}
			inferCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
			if enriched, count, inferErr := enrichInitDiscovery(inferCtx, br, toolName, result); inferErr == nil {
				result = enriched
				inferredCount = count
				quality = definition.AssessDiscoveryQuality(result)
			} else if !opts.Quiet {
				fmt.Printf("Inference-assisted probing skipped: %v\n", inferErr)
			}
			cancel()
		}
	}

	enrichedCommands, generatedSynonyms := definition.EnrichCommandTreeScaffold(toolName, result.Commands)
	result.Commands = enrichedCommands

	if err := writeScaffold(filename, toolName, result.Commands, generatedSynonyms); err != nil {
		return "", err
	}

	if !opts.Quiet {
		fmt.Printf("Created %s with %d verified commands.\n", filename, len(result.Commands))
		if inferredCount > 0 {
			fmt.Printf("  Inference-assisted probing verified %d additional commands.\n", inferredCount)
		}
		fmt.Printf("  Discovery mode: %s\n", mode)
		fmt.Printf("  Discovery quality: %.1f/100\n", quality.Score)
		fmt.Println()
		fmt.Println("Next steps:")
		fmt.Printf("  1. Review the generated examples and synonyms in %s\n", filename)
		fmt.Printf("  2. Write a system_prompt describing the tool\n")
		fmt.Printf("  3. Run: nlci %s \"<your intent>\" --dry-run\n", toolName)
	} else if quality.Weak {
		fmt.Fprintf(os.Stderr, "note: definition for %s is sparse — edit %s to improve it.\n", toolName, filename)
	}
	return filename, nil
}

func enrichInitDiscovery(ctx context.Context, br *backend.Router, toolName string, base *definition.DiscoveryResult) (*definition.DiscoveryResult, int, error) {
	if base == nil {
		base = &definition.DiscoveryResult{}
	}

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

func writeScaffold(filename, toolName string, commands []definition.Command, synonyms map[string][]string) error {
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
		writeExamples(f, "      ", c.Examples, fmt.Sprintf("%s %s ...", toolName, c.Name))
		fmt.Fprintln(f)
	}

	fmt.Fprintf(f, "safety:\n")
	fmt.Fprintf(f, "  require_confirmation: []\n")
	fmt.Fprintf(f, "  forbidden: []\n")
	if len(synonyms) > 0 {
		writeSynonyms(f, synonyms)
	}
	// init already writes a verified command inventory, so keep runtime loading
	// fast by default. Users can opt back into live discovery manually.
	fmt.Fprintf(f, "\nauto_discover: false\n")
	return nil
}

func writeFlagDrivenScaffold(filename, toolName string, result *definition.FlagDiscoveryResult, synonyms map[string][]string) error {
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
		writeExamples(f, "      ", cap.Examples, fmt.Sprintf("%s ...", toolName))
	}

	fmt.Fprintf(f, "\nsafety:\n")
	fmt.Fprintf(f, "  require_confirmation: []\n")
	fmt.Fprintf(f, "  forbidden: []\n")
	if len(synonyms) > 0 {
		writeSynonyms(f, synonyms)
	}
	fmt.Fprintf(f, "\nauto_discover: false\n")
	return nil
}

func writeSynonyms(f *os.File, synonyms map[string][]string) {
	keys := make([]string, 0, len(synonyms))
	for key := range synonyms {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	fmt.Fprintf(f, "\nsynonyms:\n")
	for _, key := range keys {
		fmt.Fprintf(f, "  %s:\n", yamlQuote(key))
		for _, target := range synonyms[key] {
			fmt.Fprintf(f, "    - %s\n", yamlQuote(target))
		}
	}
}

func writeExamples(f *os.File, indent string, examples []definition.Example, fallbackCmd string) {
	if len(examples) == 0 {
		fmt.Fprintf(f, "%s# - nl: \"...\"\n", indent)
		fmt.Fprintf(f, "%s#   cmd: %s\n", indent, yamlQuote(fallbackCmd))
		return
	}
	for _, ex := range examples {
		fmt.Fprintf(f, "%s- nl: %s\n", indent, yamlQuote(ex.NL))
		fmt.Fprintf(f, "%s  cmd: %s\n", indent, yamlQuote(ex.Cmd))
	}
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

	fmt.Println("Available Tools")
	fmt.Println("─────────────────────────────────────")
	tools, err := definition.ListAvailableTools(cfg.Definitions.Paths)
	if err != nil {
		return err
	}
	if len(tools) == 0 {
		fmt.Println("  (none — run `nlci init <tool>` to scaffold one)")
	} else {
		for _, t := range tools {
			source := t.Source
			if t.Description != "" {
				fmt.Printf("  %-12s  %s  [%s]\n", t.Name, t.Description, source)
			} else {
				fmt.Printf("  %-12s  [%s]\n", t.Name, source)
			}
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
