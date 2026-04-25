package definition

import "testing"

func TestParseSectionCommands(t *testing.T) {
	help := `Available Commands:
  install     Install a package
  uninstall   Remove a package
  list        List installed packages

Flags:
  -h, --help  help for tool`

	commands := parseSectionCommands("", help)
	if len(commands) != 3 {
		t.Fatalf("expected 3 commands, got %d", len(commands))
	}
	if commands[0].Name != "install" || commands[0].Description != "Install a package" {
		t.Fatalf("unexpected first command: %+v", commands[0])
	}
}

func TestParseUsageCommands(t *testing.T) {
	help := `Example usage:
  brew search TEXT|/REGEX/
  brew info [FORMULA|CASK...]
  brew install FORMULA|CASK...
  brew update
  brew upgrade [FORMULA|CASK...]
  brew uninstall FORMULA|CASK...
  brew list [FORMULA|CASK...]`

	commands := parseUsageCommands("brew", "", help)
	assertCommandNames(t, commands, []string{"search", "info", "install", "update", "upgrade", "uninstall", "list"})
}

func TestParseUsageCommandsNestedPrefix(t *testing.T) {
	help := `Usage: brew services [subcommand]

[sudo] brew services info (formula|--all) [--json]:
[sudo] brew services run (formula|--all) [--file=]:
[sudo] brew services start formula
[sudo] brew services stop formula`

	commands := parseUsageCommands("brew", "services", help)
	assertCommandNames(t, commands, []string{"services info", "services run", "services start", "services stop"})
}

func TestParseUsageCommandsStopsBeforeAliases(t *testing.T) {
	help := `Usage:
  brew doctor, dr [options]
  brew update, up [options]
  brew list, ls [formula|cask ...]`

	commands := parseUsageCommands("brew", "", help)
	assertCommandNames(t, commands, []string{"doctor", "update", "list"})
}

func TestStripPrefixPathIgnoresSelfAliasLine(t *testing.T) {
	tail, ok := stripPrefixPath("doctor, dr [options]", "doctor")
	if !ok {
		t.Fatal("expected prefix path to match")
	}
	if tail != "" {
		t.Fatalf("expected empty tail for alias line, got %q", tail)
	}
}

func TestParseCommandListIgnoresPseudoEntries(t *testing.T) {
	text := "--cache\n--cellar\ninstall\nsearch\nservices\nupdate\n"
	commands := parseCommandList("", text)
	assertCommandNames(t, commands, []string{"install", "search", "services", "update"})
}

func TestNormalizePathPartRejectsPlaceholders(t *testing.T) {
	for _, token := range []string{"TEXT|/REGEX/", "FORMULA|CASK...", "--cache", "URL", "<name>", "[COMMAND]"} {
		if _, ok := normalizePathPart(token); ok {
			t.Fatalf("expected %q to be rejected", token)
		}
	}
	if got, ok := normalizePathPart("install"); !ok || got != "install" {
		t.Fatalf("expected install to survive, got %q ok=%v", got, ok)
	}
}

func TestAssessDiscoveryQuality(t *testing.T) {
	weak := &DiscoveryResult{}
	if q := AssessDiscoveryQuality(weak); !q.Weak || q.Score != 0 {
		t.Fatalf("expected empty result to be weak with 0 score, got %+v", q)
	}

	strong := &DiscoveryResult{Commands: []Command{
		{Name: "install", Description: "Install a package"},
		{Name: "search", Description: "Search available packages"},
		{Name: "services start", Description: "Start a service"},
		{Name: "services stop", Description: "Stop a service"},
		{Name: "list", Description: "List installed packages"},
		{Name: "update", Description: "Update Homebrew"},
	}}
	q := AssessDiscoveryQuality(strong)
	if q.Weak {
		t.Fatalf("expected populated result to be strong, got %+v", q)
	}
	if q.Score <= 0 {
		t.Fatalf("expected positive score, got %+v", q)
	}
}

func assertCommandNames(t *testing.T, commands []Command, want []string) {
	t.Helper()
	if len(commands) != len(want) {
		t.Fatalf("expected %d commands, got %d: %+v", len(want), len(commands), commands)
	}
	for i, name := range want {
		if commands[i].Name != name {
			t.Fatalf("command %d: want %q, got %q", i, name, commands[i].Name)
		}
	}
}
