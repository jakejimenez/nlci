package definition

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	defaultDiscoveryDepth       = 2
	defaultDiscoveryConcurrency = 6
	defaultRootHelpTimeout      = 5 * time.Second
	defaultProbeTimeout         = 2 * time.Second
)

var (
	commandSectionRe = regexp.MustCompile(`(?i)^((available|additional|management|basic|other)\s+)?(commands?|subcommands?):?\s*$`)
	commandLineRe    = regexp.MustCompile(`^\s{1,8}([a-z0-9][a-z0-9_-]*)(?:,\s*-[A-Za-z])?(?:\s{2,}(.+))?$`)
	flagRe           = regexp.MustCompile(`^\s+(-([a-zA-Z]),\s+)?--([a-zA-Z][a-zA-Z0-9_-]*)(?:\s+\S+)?\s{2,}(.*)$`)
	commandPartRe    = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

	invalidHelpMarkers = []string{
		"unknown command",
		"unknown subcommand",
		"unknown shorthand flag",
		"unknown option",
		"unknown flag",
		"no such command",
		"unrecognized command",
		"invalid choice",
		"did you mean",
	}

	placeholderParts = map[string]bool{
		"arg": true, "args": true, "argument": true, "arguments": true,
		"cask": true, "command": true, "commands": true, "file": true,
		"files": true, "formula": true, "formulae": true, "name": true,
		"option": true, "options": true, "path": true, "regex": true,
		"repo": true, "subcommand": true, "subcommands": true,
		"text": true, "url": true, "value": true, "values": true,
	}

	skippedCommandNames = map[string]bool{
		"help":    true,
		"version": true,
	}
)

// DiscoveryOptions controls how aggressively deterministic discovery traverses a CLI.
type DiscoveryOptions struct {
	MaxDepth       int
	MaxConcurrency int
	RootTimeout    time.Duration
	ProbeTimeout   time.Duration
}

// DiscoveryMetrics summarizes deterministic discovery coverage.
type DiscoveryMetrics struct {
	VerifiedCommands         int
	CommandsWithDescriptions int
	NestedCommands           int
	ProbedCommands           int
	QualityScore             float64
}

// DiscoveryResult is the full output of deterministic CLI discovery.
type DiscoveryResult struct {
	Commands []Command
	RootHelp string
	Metrics  DiscoveryMetrics
}

// DiscoveryQuality is the discovery quality gate used by init.
type DiscoveryQuality struct {
	Score float64
	Weak  bool
}

type probeTarget struct {
	Command Command
	Depth   int
}

type probeResult struct {
	Verified bool
	Command  Command
	Children []Command
}

// DefaultDiscoveryOptions returns bounded traversal defaults suitable for init
// and runtime auto-discovery.
func DefaultDiscoveryOptions() DiscoveryOptions {
	return DiscoveryOptions{
		MaxDepth:       defaultDiscoveryDepth,
		MaxConcurrency: defaultDiscoveryConcurrency,
		RootTimeout:    defaultRootHelpTimeout,
		ProbeTimeout:   defaultProbeTimeout,
	}
}

// Discover runs deterministic CLI discovery and returns the final command set.
func Discover(binary string) ([]Command, error) {
	result, err := DiscoverDetailed(binary)
	if err != nil {
		return nil, err
	}
	return result.Commands, nil
}

// DiscoverDetailed returns the full deterministic discovery result.
func DiscoverDetailed(binary string) (*DiscoveryResult, error) {
	return DiscoverWithOptions(binary, DefaultDiscoveryOptions())
}

// DiscoverWithOptions runs deterministic discovery with explicit traversal limits.
func DiscoverWithOptions(binary string, opts DiscoveryOptions) (*DiscoveryResult, error) {
	opts = normalizeDiscoveryOptions(opts)

	rootHelp, err := probeRootHelp(binary, opts.RootTimeout)
	if err != nil {
		return nil, err
	}

	result := &DiscoveryResult{RootHelp: compressHelpText(rootHelp)}

	initial := parseDiscoveryText(binary, "", rootHelp)
	inventory := discoverInventory(binary, opts.ProbeTimeout)
	initial = mergeCommands(initial, inventory)

	commands, metrics := traverseCommandTree(binary, initial, opts)
	result.Commands = commands
	result.Metrics = metrics
	quality := AssessDiscoveryQuality(result)
	result.Metrics.QualityScore = quality.Score

	return result, nil
}

// DiscoverFromSeeds verifies and expands a set of candidate command paths.
// It is used by init's inference-assisted fallback so the model can suggest
// likely paths while the CLI remains the source of truth.
func DiscoverFromSeeds(binary string, seeds []Command, opts DiscoveryOptions) (*DiscoveryResult, error) {
	opts = normalizeDiscoveryOptions(opts)
	commands, metrics := traverseCommandTree(binary, seeds, opts)
	result := &DiscoveryResult{Commands: commands, Metrics: metrics}
	quality := AssessDiscoveryQuality(result)
	result.Metrics.QualityScore = quality.Score
	return result, nil
}

// AssessDiscoveryQuality scores deterministic discovery so init can decide
// whether inference-assisted probing is worth attempting.
func AssessDiscoveryQuality(result *DiscoveryResult) DiscoveryQuality {
	if result == nil {
		return DiscoveryQuality{Score: 0, Weak: true}
	}
	total := len(result.Commands)
	if total == 0 {
		return DiscoveryQuality{Score: 0, Weak: true}
	}

	described := result.Metrics.CommandsWithDescriptions
	if described == 0 {
		for _, c := range result.Commands {
			if strings.TrimSpace(c.Description) != "" {
				described++
			}
		}
	}

	nested := result.Metrics.NestedCommands
	if nested == 0 {
		for _, c := range result.Commands {
			if strings.Contains(c.Name, " ") {
				nested++
			}
		}
	}

	descRatio := float64(described) / float64(total)
	nestedRatio := float64(nested) / float64(total)

	score := float64(total)*4 + descRatio*25 + nestedRatio*10
	if score > 100 {
		score = 100
	}

	weak := total == 0 || (total < 6 && descRatio < 0.6)
	return DiscoveryQuality{Score: score, Weak: weak}
}

func normalizeDiscoveryOptions(opts DiscoveryOptions) DiscoveryOptions {
	if opts.MaxDepth <= 0 {
		opts.MaxDepth = defaultDiscoveryDepth
	}
	if opts.MaxConcurrency <= 0 {
		opts.MaxConcurrency = defaultDiscoveryConcurrency
	}
	if opts.RootTimeout <= 0 {
		opts.RootTimeout = defaultRootHelpTimeout
	}
	if opts.ProbeTimeout <= 0 {
		opts.ProbeTimeout = defaultProbeTimeout
	}
	return opts
}

func probeRootHelp(binary string, timeout time.Duration) (string, error) {
	text, err := runFirstSuccessful(binary, timeout, [][]string{{"--help"}, {"help"}})
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(text) == "" {
		return "", fmt.Errorf("discover: no help output from %q", binary)
	}
	return text, nil
}

func discoverInventory(binary string, timeout time.Duration) []Command {
	var commands []Command
	for _, args := range []struct {
		argv    []string
		useList bool
	}{
		{argv: []string{"commands", "--quiet"}, useList: true},
		{argv: []string{"commands"}, useList: true},
		{argv: []string{"help", "commands"}},
	} {
		text, err := runCLI(binary, args.argv, timeout)
		if err != nil || strings.TrimSpace(text) == "" || isInvalidHelpOutput(text) {
			continue
		}
		if args.useList {
			commands = mergeCommands(commands, parseCommandList("", text))
		}
		commands = mergeCommands(commands, parseDiscoveryText(binary, "", text))
	}
	return commands
}

func traverseCommandTree(binary string, initial []Command, opts DiscoveryOptions) ([]Command, DiscoveryMetrics) {
	current := make([]probeTarget, 0, len(initial))
	queued := make(map[string]bool, len(initial))
	for _, c := range initial {
		if c.Name == "" || queued[c.Name] {
			continue
		}
		queued[c.Name] = true
		current = append(current, probeTarget{Command: c, Depth: 1})
	}

	verified := make(map[string]Command, len(initial))
	hasChildren := make(map[string]bool, len(initial))
	var order []string
	metrics := DiscoveryMetrics{}

	for len(current) > 0 {
		results := probeLevel(binary, current, opts)
		var next []probeTarget
		for _, r := range results {
			if !r.Verified {
				continue
			}
			metrics.ProbedCommands++

			existing, seen := verified[r.Command.Name]
			if seen {
				verified[r.Command.Name] = mergeCommand(existing, r.Command)
			} else {
				verified[r.Command.Name] = r.Command
				order = append(order, r.Command.Name)
			}

			if len(r.Children) > 0 {
				hasChildren[r.Command.Name] = true
			}

			for _, child := range r.Children {
				if queued[child.Name] || child.Name == r.Command.Name {
					continue
				}
				queued[child.Name] = true
				next = append(next, probeTarget{Command: child, Depth: depthOf(child.Name)})
			}
		}

		current = nil
		for _, candidate := range next {
			if candidate.Depth <= opts.MaxDepth {
				current = append(current, candidate)
			}
		}
	}

	commands := make([]Command, 0, len(order))
	for _, name := range order {
		if hasChildren[name] {
			continue
		}
		cmd := verified[name]
		commands = append(commands, cmd)
		if strings.TrimSpace(cmd.Description) != "" {
			metrics.CommandsWithDescriptions++
		}
		if strings.Contains(cmd.Name, " ") {
			metrics.NestedCommands++
		}
	}
	metrics.VerifiedCommands = len(commands)
	return commands, metrics
}

func probeLevel(binary string, current []probeTarget, opts DiscoveryOptions) []probeResult {
	results := make([]probeResult, len(current))
	sem := make(chan struct{}, opts.MaxConcurrency)
	var wg sync.WaitGroup

	for i, target := range current {
		wg.Add(1)
		go func(i int, target probeTarget) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			results[i] = probePath(binary, target.Command, opts.ProbeTimeout)
		}(i, target)
	}

	wg.Wait()
	return results
}

func probePath(binary string, target Command, timeout time.Duration) probeResult {
	helpText, err := probeCommandHelp(binary, target.Name, timeout)
	if err != nil || strings.TrimSpace(helpText) == "" || isInvalidHelpOutput(helpText) {
		return probeResult{}
	}

	verified := target
	if verified.Description == "" {
		verified.Description = extractHelpSummary(helpText)
	}
	verified.Flags = parseFlags(helpText)

	children := parseDiscoveryText(binary, target.Name, helpText)
	filteredChildren := make([]Command, 0, len(children))
	for _, child := range children {
		if child.Name != target.Name {
			filteredChildren = append(filteredChildren, child)
		}
	}

	return probeResult{
		Verified: true,
		Command:  verified,
		Children: filteredChildren,
	}
}

func probeCommandHelp(binary, path string, timeout time.Duration) (string, error) {
	pathArgs := strings.Fields(path)
	candidates := []string{strings.Join(append(pathArgs, "--help"), " ")}
	if len(pathArgs) > 0 {
		candidates = append(candidates, strings.Join(append([]string{"help"}, pathArgs...), " "))
	}

	text, err := runFirstSuccessful(binary, timeout, [][]string{append(pathArgs, "--help"), append([]string{"help"}, pathArgs...)})
	if err != nil {
		return "", fmt.Errorf("discover: probe %q: %w", path, err)
	}
	if strings.TrimSpace(text) == "" {
		return "", fmt.Errorf("discover: empty help for %q (tried %s)", path, strings.Join(candidates, ", "))
	}
	return text, nil
}

func runFirstSuccessful(binary string, timeout time.Duration, argSets [][]string) (string, error) {
	var lastErr error
	for _, args := range argSets {
		if len(args) == 0 {
			continue
		}
		text, err := runCLI(binary, args, timeout)
		if err == nil && strings.TrimSpace(text) != "" && !isInvalidHelpOutput(text) {
			return text, nil
		}
		if err != nil {
			lastErr = err
		}
	}
	if lastErr != nil {
		return "", lastErr
	}
	return "", fmt.Errorf("discover: no output from %q", binary)
}

func runCLI(binary string, args []string, timeout time.Duration) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, binary, args...)
	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errOut
	_ = cmd.Run()

	text := out.String()
	if strings.TrimSpace(text) == "" {
		text = errOut.String()
	}
	if strings.TrimSpace(text) == "" {
		return "", fmt.Errorf("discover: no output from %q %s", binary, strings.Join(args, " "))
	}
	return text, nil
}

func parseDiscoveryText(binary, prefix, text string) []Command {
	binary = filepath.Base(binary)
	commands := parseSectionCommands(prefix, text)
	commands = mergeCommands(commands, parseUsageCommands(binary, prefix, text))
	commands = mergeCommands(commands, parseCommandList(prefix, text))
	return commands
}

func parseSectionCommands(prefix, text string) []Command {
	var commands []Command
	seen := make(map[string]bool)
	scanner := bufio.NewScanner(strings.NewReader(text))
	inSection := false

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		if commandSectionRe.MatchString(trimmed) {
			inSection = true
			continue
		}
		if !inSection {
			continue
		}
		if trimmed == "" {
			continue
		}
		if !strings.HasPrefix(line, " ") && strings.HasSuffix(trimmed, ":") {
			inSection = false
			continue
		}

		matches := commandLineRe.FindStringSubmatch(line)
		if matches == nil {
			continue
		}

		name, ok := joinCommandPath(prefix, matches[1])
		if !ok || skippedCommandNames[name] || seen[name] {
			continue
		}
		seen[name] = true

		desc := ""
		if len(matches) > 2 {
			desc = strings.TrimSpace(matches[2])
		}
		commands = append(commands, Command{Name: name, Description: desc})
	}

	return commands
}

func parseUsageCommands(binary, prefix, text string) []Command {
	var commands []Command
	seen := make(map[string]bool)

	scanner := bufio.NewScanner(strings.NewReader(text))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		line = trimUsagePrefix(line)
		line = trimStructuredPrefixes(line)
		tail, ok := stripBinaryPrefix(line, binary)
		if !ok {
			continue
		}
		if prefix != "" {
			tail, ok = stripPrefixPath(tail, prefix)
			if !ok {
				continue
			}
		}

		name, ok := parseCommandPathFromTail(prefix, tail)
		if !ok || seen[name] || skippedCommandNames[name] {
			continue
		}
		seen[name] = true
		commands = append(commands, Command{Name: name})
	}

	return commands
}

func parseCommandList(prefix, text string) []Command {
	var lines []string
	scanner := bufio.NewScanner(strings.NewReader(text))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			lines = append(lines, line)
		}
	}
	if len(lines) < 3 {
		return nil
	}

	commandish := 0
	for _, line := range lines {
		if strings.Contains(line, " ") || strings.Contains(line, "\t") {
			continue
		}
		if _, ok := normalizePathPart(line); ok || strings.HasPrefix(line, "--") {
			commandish++
		}
	}
	if commandish*3 < len(lines)*2 {
		return nil
	}

	var commands []Command
	seen := make(map[string]bool)
	for _, line := range lines {
		if strings.Contains(line, " ") || strings.Contains(line, "\t") {
			continue
		}
		name, ok := joinCommandPath(prefix, line)
		if !ok || seen[name] || skippedCommandNames[name] {
			continue
		}
		seen[name] = true
		commands = append(commands, Command{Name: name})
	}
	return commands
}

func mergeCommands(base []Command, additions ...[]Command) []Command {
	result := append([]Command(nil), base...)
	index := make(map[string]int, len(result))
	for i, cmd := range result {
		index[cmd.Name] = i
	}

	for _, set := range additions {
		for _, cmd := range set {
			if cmd.Name == "" {
				continue
			}
			if i, ok := index[cmd.Name]; ok {
				result[i] = mergeCommand(result[i], cmd)
				continue
			}
			index[cmd.Name] = len(result)
			result = append(result, cmd)
		}
	}

	return result
}

func mergeCommand(a, b Command) Command {
	merged := a
	if merged.Name == "" {
		merged.Name = b.Name
	}
	if merged.Description == "" {
		merged.Description = b.Description
	}
	if len(merged.Flags) == 0 {
		merged.Flags = append([]Flag(nil), b.Flags...)
	}
	return merged
}

func joinCommandPath(prefix, raw string) (string, bool) {
	part, ok := normalizePathPart(raw)
	if !ok {
		return "", false
	}
	if prefix == "" {
		return part, true
	}
	return prefix + " " + part, true
}

func stripBinaryPrefix(line, binary string) (string, bool) {
	if line == binary {
		return "", true
	}
	if strings.HasPrefix(line, binary+" ") {
		return strings.TrimSpace(strings.TrimPrefix(line, binary)), true
	}
	return "", false
}

func stripPrefixPath(tail, prefix string) (string, bool) {
	prefixParts := strings.Fields(prefix)
	tailParts := strings.Fields(tail)
	if len(tailParts) < len(prefixParts) {
		return "", false
	}
	matchedAliasToken := false
	for i, want := range prefixParts {
		raw := tailParts[i]
		part, ok := normalizePathPart(tailParts[i])
		if !ok || part != want {
			return "", false
		}
		if i == len(prefixParts)-1 && strings.HasSuffix(raw, ",") {
			matchedAliasToken = true
		}
	}
	if matchedAliasToken {
		return "", true
	}
	return strings.Join(tailParts[len(prefixParts):], " "), true
}

func parseCommandPathFromTail(prefix, tail string) (string, bool) {
	fields := strings.Fields(tail)
	var parts []string
	for _, field := range fields {
		part, ok := normalizePathPart(field)
		if !ok {
			if len(parts) == 0 {
				return "", false
			}
			break
		}
		parts = append(parts, part)
		// Usage lines like "brew doctor, dr [options]" expose aliases inline.
		// Keep the canonical path segment and stop before alias tokens.
		if strings.HasSuffix(field, ",") {
			break
		}
	}
	if len(parts) == 0 {
		return "", false
	}
	name := strings.Join(parts, " ")
	if prefix != "" {
		name = prefix + " " + name
	}
	if skippedCommandNames[name] {
		return "", false
	}
	return name, true
}

func normalizePathPart(token string) (string, bool) {
	token = strings.TrimSpace(token)
	token = strings.Trim(token, `"'`)
	token = strings.TrimRight(token, ":,")
	if token == "" {
		return "", false
	}

	if strings.HasPrefix(token, "[") && strings.HasSuffix(token, "]") && !strings.Contains(token, " ") {
		inner := strings.ToLower(strings.Trim(token, "[]"))
		if inner == "" || placeholderParts[inner] {
			return "", false
		}
		token = inner
	}

	if strings.HasPrefix(token, "-") || strings.ContainsAny(token, `/|<>(){}=`) {
		return "", false
	}
	if placeholderParts[strings.ToLower(token)] {
		return "", false
	}
	if !commandPartRe.MatchString(token) {
		return "", false
	}
	return token, true
}

func trimUsagePrefix(line string) string {
	lower := strings.ToLower(line)
	if strings.HasPrefix(lower, "usage:") {
		return strings.TrimSpace(line[len("usage:"):])
	}
	return line
}

func trimStructuredPrefixes(line string) string {
	for strings.HasPrefix(line, "[") {
		end := strings.Index(line, "]")
		if end <= 0 {
			break
		}
		line = strings.TrimSpace(line[end+1:])
	}
	return line
}

func depthOf(path string) int {
	if strings.TrimSpace(path) == "" {
		return 0
	}
	return len(strings.Fields(path))
}

func extractHelpSummary(text string) string {
	scanner := bufio.NewScanner(strings.NewReader(text))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		lower := strings.ToLower(line)
		if strings.HasPrefix(lower, "usage:") || strings.HasSuffix(line, ":") {
			continue
		}
		return line
	}
	return ""
}

func isInvalidHelpOutput(text string) bool {
	lower := strings.ToLower(text)
	for _, marker := range invalidHelpMarkers {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

// DiscoverSubcommand runs `<binary> <subcommand> --help` and returns parsed flags.
func DiscoverSubcommand(binary, subcommand string) ([]Flag, error) {
	helpText, err := probeCommandHelp(binary, subcommand, defaultRootHelpTimeout)
	if err != nil {
		return nil, err
	}
	return parseFlags(helpText), nil
}

// SubcommandHelpText returns the raw --help output for a subcommand.
// Used by the agentic loop to inject into retry prompts.
func SubcommandHelpText(binary, subcommand string) string {
	helpText, err := probeCommandHelp(binary, subcommand, defaultRootHelpTimeout)
	if err != nil {
		return ""
	}
	return compressHelpText(helpText)
}

// parseFlags extracts flag information from subcommand --help output.
func parseFlags(text string) []Flag {
	var flags []Flag
	seen := make(map[string]bool)

	scanner := bufio.NewScanner(strings.NewReader(text))
	for scanner.Scan() {
		line := scanner.Text()
		matches := flagRe.FindStringSubmatch(line)
		if matches == nil {
			continue
		}

		short := matches[2]
		name := matches[3]
		desc := strings.TrimSpace(matches[4])
		if seen[name] {
			continue
		}
		seen[name] = true

		flags = append(flags, Flag{
			Name:        name,
			Short:       short,
			Description: desc,
		})
	}

	return flags
}

// FormatFlags renders a []Flag slice as a compact, one-per-line prompt string.
// E.g.: "  --follow (-f): Follow log output"
// This is used in retry prompts to give the model an authoritative flag list
// without the ambiguity of raw --help output (continuation lines, examples, etc.).
func FormatFlags(flags []Flag) string {
	if len(flags) == 0 {
		return ""
	}
	var lines []string
	for _, f := range flags {
		var line string
		if f.Short != "" {
			line = fmt.Sprintf("  --%s (-%s): %s", f.Name, f.Short, f.Description)
		} else {
			line = fmt.Sprintf("  --%s: %s", f.Name, f.Description)
		}
		lines = append(lines, line)
	}
	result := strings.Join(lines, "\n")
	if len(result) > 600 {
		result = result[:600] + "\n  [...]"
	}
	return result
}

// compressHelpText trims a --help output to a compact, token-efficient form.
// Keeps flag names and one-line descriptions, strips usage examples and padding.
func compressHelpText(text string) string {
	var lines []string
	scanner := bufio.NewScanner(strings.NewReader(text))
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "-") ||
			strings.HasPrefix(trimmed, "--") ||
			strings.HasSuffix(trimmed, ":") ||
			strings.Contains(line, "  ") {
			lines = append(lines, trimmed)
		}
	}

	result := strings.Join(lines, "\n")
	if len(result) > 800 {
		result = result[:800] + "\n[...truncated]"
	}
	return result
}
