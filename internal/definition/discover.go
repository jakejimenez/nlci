package definition

import (
	"bufio"
	"bytes"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"
	"context"
)

// Discover runs `<binary> --help` and parses the output into a list of Commands.
// It makes a best-effort attempt to extract subcommand names and descriptions.
func Discover(binary string) ([]Command, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binary, "--help")
	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errOut

	// Many CLIs write help to stderr and exit non-zero — that's fine
	_ = cmd.Run()

	helpText := out.String()
	if strings.TrimSpace(helpText) == "" {
		helpText = errOut.String()
	}

	if strings.TrimSpace(helpText) == "" {
		return nil, fmt.Errorf("discover: no help output from %q", binary)
	}

	return parseHelpText(helpText), nil
}

// DiscoverSubcommand runs `<binary> <subcommand> --help` and returns parsed flags.
func DiscoverSubcommand(binary, subcommand string) ([]Flag, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	args := append(strings.Fields(subcommand), "--help")
	cmd := exec.CommandContext(ctx, binary, args...)
	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errOut

	_ = cmd.Run()

	helpText := out.String()
	if strings.TrimSpace(helpText) == "" {
		helpText = errOut.String()
	}

	return parseFlags(helpText), nil
}

// SubcommandHelpText returns the raw --help output for a subcommand.
// Used by the agentic loop to inject into retry prompts.
func SubcommandHelpText(binary, subcommand string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	args := append(strings.Fields(subcommand), "--help")
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

	return compressHelpText(text)
}

// parseHelpText extracts subcommands from --help output.
// Handles multiple common CLI help formats (cobra, urfave/cli, custom).
func parseHelpText(text string) []Command {
	var commands []Command
	seen := make(map[string]bool)

	scanner := bufio.NewScanner(strings.NewReader(text))

	// State machine: look for a "Commands:" or "Available Commands:" section
	inCommandsSection := false
	commandSectionRe := regexp.MustCompile(`(?i)^(available\s+)?commands?:?\s*$`)
	// Match lines like:  "  subcommand   Description of subcommand"
	commandLineRe := regexp.MustCompile(`^\s{1,6}([a-z][a-z0-9_-]*)(?:\s{2,}(.+))?$`)

	for scanner.Scan() {
		line := scanner.Text()

		// Detect section headers
		trimmed := strings.TrimSpace(line)
		if commandSectionRe.MatchString(trimmed) {
			inCommandsSection = true
			continue
		}

		// A blank line or new section header ends the commands section
		if inCommandsSection {
			if trimmed == "" {
				continue
			}
			// New section (non-indented header)
			if !strings.HasPrefix(line, " ") && strings.HasSuffix(trimmed, ":") {
				inCommandsSection = false
				continue
			}
		}

		if !inCommandsSection {
			continue
		}

		matches := commandLineRe.FindStringSubmatch(line)
		if matches == nil {
			continue
		}

		name := matches[1]
		desc := ""
		if len(matches) > 2 {
			desc = strings.TrimSpace(matches[2])
		}

		// Skip common non-command words
		skip := map[string]bool{
			"help": true, "version": true, "completion": true,
			"help,": true, "options": true, "flags": true,
		}
		if skip[name] || seen[name] {
			continue
		}

		seen[name] = true
		commands = append(commands, Command{
			Name:        name,
			Description: desc,
		})
	}

	return commands
}

// parseFlags extracts flag information from subcommand --help output.
func parseFlags(text string) []Flag {
	var flags []Flag
	seen := make(map[string]bool)

	// Match patterns like:
	//   -f, --flag         Description
	//       --flag         Description
	//   -f string          Description
	flagRe := regexp.MustCompile(`^\s+(-([a-zA-Z]),\s+)?--([a-zA-Z][a-zA-Z0-9_-]*)(?:\s+\S+)?\s{2,}(.*)$`)

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
		// Keep flag lines and section headers; skip example/usage blocks
		if strings.HasPrefix(trimmed, "-") ||
			strings.HasPrefix(trimmed, "--") ||
			strings.HasSuffix(trimmed, ":") ||
			strings.Contains(line, "  ") {
			lines = append(lines, trimmed)
		}
	}

	result := strings.Join(lines, "\n")
	// Hard cap at ~800 chars to stay within token budget
	if len(result) > 800 {
		result = result[:800] + "\n[...truncated]"
	}
	return result
}
