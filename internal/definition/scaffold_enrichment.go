package definition

import (
	"strings"
	"unicode"
)

// EnrichCommandTreeScaffold adds deterministic active examples and conservative
// synonyms to init-generated command-tree definitions.
func EnrichCommandTreeScaffold(toolName string, commands []Command) ([]Command, map[string][]string) {
	enriched := make([]Command, len(commands))
	copy(enriched, commands)

	synonyms := make(map[string][]string)
	for i := range enriched {
		family := inferActionFamily(enriched[i].Name, enriched[i].Description)
		kind := inferEntityKind(enriched[i].Name, enriched[i].Description)
		if len(enriched[i].Examples) == 0 {
			enriched[i].Examples = generateCommandExamples(toolName, enriched[i], family, kind)
		}
		for _, word := range commandSynonyms(family, kind) {
			synonyms[word] = append(synonyms[word], enriched[i].Name)
		}
	}

	return enriched, normalizeSynonyms(synonyms)
}

// EnrichFlagDrivenScaffold adds deterministic active examples and generic
// synonyms to init-generated flag-driven definitions.
func EnrichFlagDrivenScaffold(toolName string, result *FlagDiscoveryResult) (*FlagDiscoveryResult, map[string][]string) {
	if result == nil {
		return &FlagDiscoveryResult{}, nil
	}

	rootFlags := make([]Flag, len(result.RootFlags))
	copy(rootFlags, result.RootFlags)
	flagByName := make(map[string]Flag, len(rootFlags))
	for _, f := range rootFlags {
		flagByName[f.Name] = f
	}

	capabilities := make([]Capability, len(result.Capabilities))
	copy(capabilities, result.Capabilities)
	synonyms := make(map[string][]string)
	for i := range capabilities {
		if len(capabilities[i].Examples) == 0 {
			capabilities[i].Examples = generateCapabilityExamples(toolName, capabilities[i], flagByName)
		}
		for _, word := range capabilitySynonyms(capabilities[i].Name) {
			synonyms[word] = append(synonyms[word], capabilities[i].Name)
		}
	}

	return &FlagDiscoveryResult{
		RootHelp:     result.RootHelp,
		RootFlags:    rootFlags,
		Capabilities: capabilities,
		QualityScore: result.QualityScore,
		Weak:         result.Weak,
	}, normalizeSynonyms(synonyms)
}

func inferActionFamily(name, desc string) string {
	leaf := strings.ToLower(pathLeaf(name))
	text := strings.ToLower(name + " " + desc)

	switch leaf {
	case "list", "ls", "ps", "status":
		return "list"
	case "view", "show", "info", "inspect", "describe", "cat", "desc":
		return "info"
	case "search", "find", "lookup":
		return "search"
	case "install", "add", "create", "pull", "clone", "fetch":
		return "install"
	case "remove", "delete", "rm", "uninstall", "untap", "unlink":
		return "remove"
	case "update", "refresh", "sync":
		return "update"
	case "upgrade":
		return "upgrade"
	case "cleanup", "clean", "prune", "autoremove":
		return "cleanup"
	case "outdated", "missing":
		return "outdated"
	case "doctor", "check", "verify", "audit", "test", "validate", "checks":
		return "check"
	case "start", "run", "launch":
		return "start"
	case "stop", "halt", "kill", "shutdown":
		return "stop"
	case "restart", "reload":
		return "restart"
	case "browse", "open", "home":
		return "open"
	case "logs", "log":
		return "logs"
	case "exec", "shell", "enter":
		return "exec"
	case "download", "get":
		return "download"
	case "upload", "push", "publish":
		return "upload"
	}

	switch {
	case strings.Contains(text, "pull request") || strings.Contains(text, "checks"):
		return "check"
	case strings.Contains(text, "outdated"):
		return "outdated"
	case strings.Contains(text, "list") || strings.Contains(text, "show all"):
		return "list"
	case strings.Contains(text, "install") || strings.Contains(text, "create"):
		return "install"
	case strings.Contains(text, "remove") || strings.Contains(text, "delete") || strings.Contains(text, "uninstall"):
		return "remove"
	case strings.Contains(text, "update"):
		return "update"
	case strings.Contains(text, "upgrade"):
		return "upgrade"
	case strings.Contains(text, "cleanup") || strings.Contains(text, "prune"):
		return "cleanup"
	case strings.Contains(text, "browser"):
		return "open"
	}

	return ""
}

func inferEntityKind(name, desc string) string {
	text := strings.ToLower(name + " " + desc)
	parts := strings.Fields(strings.ToLower(name))
	if len(parts) > 0 {
		switch parts[0] {
		case "pr":
			return "pr"
		case "issue":
			return "issue"
		case "repo":
			return "repo"
		case "service", "services":
			return "service"
		case "release":
			return "release"
		case "run":
			return "workflow"
		}
	}

	switch {
	case strings.Contains(text, "pull request"):
		return "pr"
	case strings.Contains(text, "issue"):
		return "issue"
	case strings.Contains(text, "repo") || strings.Contains(text, "repository"):
		return "repo"
	case strings.Contains(text, "service"):
		return "service"
	case strings.Contains(text, "image"):
		return "image"
	case strings.Contains(text, "container"):
		return "container"
	case strings.Contains(text, "formula") || strings.Contains(text, "cask") || strings.Contains(text, "package") || strings.Contains(text, "dependency"):
		return "package"
	case strings.Contains(text, "workflow") || strings.Contains(text, "run"):
		return "workflow"
	case strings.Contains(text, "branch"):
		return "branch"
	case strings.Contains(text, "release"):
		return "release"
	case strings.Contains(text, "file") || strings.Contains(text, "path"):
		return "file"
	case strings.Contains(text, "url"):
		return "url"
	}

	return "generic"
}

func generateCommandExamples(toolName string, cmd Command, family, kind string) []Example {
	base := joinParts(toolName, cmd.Name)
	value := sampleValueForFamilyAndKind(family, kind)
	plural := pluralLabelForKind(kind)
	toolPhrases := toolNamePhrases(toolName, cmd.Description)
	var examples []Example

	add := func(nl, command string) {
		if nl == "" || command == "" {
			return
		}
		for _, ex := range examples {
			if ex.NL == nl || ex.Cmd == command {
				return
			}
		}
		examples = append(examples, Example{NL: nl, Cmd: command})
	}

	switch family {
	case "list":
		switch kind {
		case "package":
			add("list all installed packages", base)
		case "pr":
			add("list open pull requests", base)
		case "issue":
			add("list open issues", base)
		case "service":
			add("list services", base)
		default:
			add("list "+plural, base)
		}
	case "info":
		if value != "" && kind != "generic" {
			add("show info about "+value, joinParts(toolName, cmd.Name, value))
		} else {
			add("show details", base)
		}
	case "search":
		add("search for <query>", joinParts(toolName, cmd.Name, "<query>"))
	case "install":
		add("install "+value, joinParts(toolName, cmd.Name, value))
	case "remove":
		if kind == "package" {
			add("uninstall "+value, joinParts(toolName, cmd.Name, value))
		}
		add("remove "+value, joinParts(toolName, cmd.Name, value))
	case "update":
		for _, phrase := range toolPhrases {
			add("update "+phrase, base)
		}
	case "upgrade":
		if kind == "package" {
			add("upgrade just "+value, joinParts(toolName, cmd.Name, value))
		}
		add("upgrade all installed "+plural, base)
	case "cleanup":
		if kind == "package" {
			add("clean up old package versions", base)
		} else {
			add("clean up unused data", base)
		}
	case "outdated":
		add("show what "+plural+" are outdated", base)
	case "check":
		switch kind {
		case "pr":
			add("check the status of CI checks on PR <id>", joinParts(toolName, cmd.Name, sampleValueForFamilyAndKind(family, kind)))
		default:
			add("check "+toolPhrases[0]+" for issues", base)
		}
	case "start":
		if kind == "service" {
			add("start the "+value+" service", joinParts(toolName, cmd.Name, value))
		} else {
			add("start "+value, joinParts(toolName, cmd.Name, value))
		}
	case "stop":
		if kind == "service" {
			add("stop the "+value+" service", joinParts(toolName, cmd.Name, value))
		} else {
			add("stop "+value, joinParts(toolName, cmd.Name, value))
		}
	case "restart":
		if kind == "service" {
			add("restart the "+value+" service", joinParts(toolName, cmd.Name, value))
		} else {
			add("restart "+value, joinParts(toolName, cmd.Name, value))
		}
	case "open":
		if kind == "repo" {
			add("open the repo in the browser", base)
		} else {
			add("open in the browser", base)
		}
	case "logs":
		add("show logs for "+value, joinParts(toolName, cmd.Name, value))
	case "exec":
		add("run a command in "+value, joinParts(toolName, cmd.Name, value))
	case "download":
		add("download <url>", joinParts(toolName, cmd.Name, sampleValueForFamilyAndKind(family, "url")))
	case "upload":
		add("upload <file>", joinParts(toolName, cmd.Name, sampleValueForFamilyAndKind(family, "file")))
	}

	if len(examples) > 2 {
		examples = examples[:2]
	}
	return examples
}

func generateCapabilityExamples(toolName string, cap Capability, flags map[string]Flag) []Example {
	var examples []Example
	add := func(nl, command string) {
		if nl == "" || command == "" {
			return
		}
		for _, ex := range examples {
			if ex.NL == nl || ex.Cmd == command {
				return
			}
		}
		examples = append(examples, Example{NL: nl, Cmd: command})
	}

	has := func(name string) bool {
		_, ok := flags[name]
		return ok
	}
	flagRef := func(name string) string {
		if f, ok := flags[name]; ok {
			if f.Short != "" && len(f.Name) > 6 {
				return "-" + f.Short
			}
			return "--" + f.Name
		}
		return ""
	}

	switch cap.Name {
	case "request":
		// GET first so the model has a flagless template to copy when the
		// user's intent doesn't ask for a body. Without this, the only
		// request-capability example is the JSON POST below, and the model
		// often emits --json even when the intent says "GET".
		add("send a GET request to <url>", joinParts(toolName, "<url>"))
		if has("json") {
			add("send a JSON POST to <url>", joinParts(toolName, flagRef("json"), "<json>", "<url>"))
		} else if has("data") {
			add("send form data to <url>", joinParts(toolName, flagRef("data"), "<data>", "<url>"))
		}
	case "headers":
		if has("head") {
			add("fetch only headers for <url>", joinParts(toolName, flagRef("head"), "<url>"))
		}
		if has("header") {
			add("send a custom header to <url>", joinParts(toolName, flagRef("header"), "<header>", "<url>"))
		}
	case "auth":
		if has("user") {
			add("use basic auth for <url>", joinParts(toolName, flagRef("user"), "<user:pass>", "<url>"))
		}
	case "output":
		if has("output") {
			add("save <url> to <file>", joinParts(toolName, flagRef("output"), "<file>", "<url>"))
		}
	case "redirects":
		if has("location") {
			add("follow redirects for <url>", joinParts(toolName, flagRef("location"), "<url>"))
		}
	case "transfer":
		add("download <url>", joinParts(toolName, "<url>"))
		if has("upload-file") {
			add("upload <file> to <url>", joinParts(toolName, flagRef("upload-file"), "<file>", "<url>"))
		}
	case "proxy":
		if has("proxy") {
			add("download <url> through a proxy", joinParts(toolName, flagRef("proxy"), "<host>", "<url>"))
		}
	case "tls":
		if has("insecure") {
			add("download <url> without verifying TLS", joinParts(toolName, flagRef("insecure"), "<url>"))
		}
	case "debugging":
		if has("verbose") {
			add("show verbose output for <url>", joinParts(toolName, flagRef("verbose"), "<url>"))
		}
	}

	if len(examples) > 2 {
		examples = examples[:2]
	}
	return examples
}

func commandSynonyms(family, kind string) []string {
	var words []string
	switch family {
	case "search":
		words = append(words, "find")
	case "cleanup":
		words = append(words, "cleanup", "clean", "prune")
	case "check":
		words = append(words, "verify", "issues", "problems")
	case "open":
		words = append(words, "browse")
	case "logs":
		words = append(words, "stream", "follow")
	case "exec":
		words = append(words, "shell", "enter")
	case "download":
		words = append(words, "fetch")
	}

	switch kind {
	case "pr":
		words = append(words, "pr")
	case "repo":
		words = append(words, "repo", "repository")
	case "service":
		words = append(words, "service")
	case "issue":
		words = append(words, "issue")
	}

	return dedupeStrings(words)
}

func capabilitySynonyms(name string) []string {
	switch name {
	case "request":
		return []string{"json", "post", "body", "request"}
	case "headers":
		return []string{"header", "headers", "head", "include"}
	case "auth":
		return []string{"auth", "login", "user", "password", "token"}
	case "output":
		return []string{"save", "write", "output", "file"}
	case "redirects":
		return []string{"redirect", "redirects", "follow"}
	case "transfer":
		return []string{"download", "upload", "fetch", "transfer"}
	case "proxy":
		return []string{"proxy"}
	case "tls":
		return []string{"tls", "ssl", "certificate", "insecure"}
	case "debugging":
		return []string{"debug", "verbose", "trace"}
	default:
		return nil
	}
}

func normalizeSynonyms(in map[string][]string) map[string][]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string][]string, len(in))
	for word, targets := range in {
		word = strings.TrimSpace(strings.ToLower(word))
		if word == "" {
			continue
		}
		targets = dedupeStrings(targets)
		if len(targets) > 0 {
			out[word] = targets
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func toolNamePhrases(toolName, desc string) []string {
	phrases := []string{strings.ToLower(toolName)}
	if proper := extractProperNoun(desc); proper != "" && proper != phrases[0] {
		phrases = append([]string{proper}, phrases...)
	}
	return dedupeStrings(phrases)
}

func extractProperNoun(desc string) string {
	stop := map[string]bool{
		"Show": true, "Display": true, "Check": true, "Create": true, "Open": true,
		"Run": true, "Manage": true, "Return": true, "Control": true, "Generate": true,
		"Build": true, "Fetch": true, "List": true,
	}
	for _, field := range strings.Fields(desc) {
		clean := strings.Trim(field, ".,:;()[]{}\"'")
		if clean == "" || stop[clean] {
			continue
		}
		for _, r := range clean {
			if unicode.IsUpper(r) {
				return strings.ToLower(clean)
			}
			break
		}
	}
	return ""
}

// sampleValueForFamilyAndKind returns a bracketed placeholder for the given
// kind. Placeholders signal "user value goes here" to the model and are
// already rejected by the validator's `placeholderREs` if copied verbatim,
// which forces the agentic retry loop to substitute the user's actual value.
func sampleValueForFamilyAndKind(family, kind string) string {
	switch kind {
	case "package":
		return "<package>"
	case "service":
		return "<service>"
	case "image":
		return "<image>"
	case "container":
		return "<container>"
	case "repo":
		return "<owner/repo>"
	case "pr", "issue":
		return "<id>"
	case "workflow":
		return "<id>"
	case "branch":
		return "<branch>"
	case "release":
		return "<version>"
	case "file":
		return "<file>"
	case "url":
		return "<url>"
	default:
		if family == "download" {
			return "<url>"
		}
		if family == "upload" {
			return "<file>"
		}
		return "<value>"
	}
}

func pluralLabelForKind(kind string) string {
	switch kind {
	case "package":
		return "packages"
	case "service":
		return "services"
	case "container":
		return "containers"
	case "image":
		return "images"
	case "repo":
		return "repositories"
	case "pr":
		return "pull requests"
	case "issue":
		return "issues"
	case "workflow":
		return "workflow runs"
	case "release":
		return "releases"
	default:
		return "items"
	}
}

func pathLeaf(name string) string {
	parts := strings.Fields(name)
	if len(parts) == 0 {
		return ""
	}
	return parts[len(parts)-1]
}

func joinParts(parts ...string) string {
	filtered := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			filtered = append(filtered, p)
		}
	}
	return strings.Join(filtered, " ")
}
