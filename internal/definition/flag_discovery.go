package definition

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// FlagDiscoveryResult is the deterministic discovery output for flag-driven CLIs.
type FlagDiscoveryResult struct {
	RootHelp     string
	RootFlags    []Flag
	Capabilities []Capability
	QualityScore float64
	Weak         bool
}

// DiscoverFlagDriven inspects a flag-driven CLI by probing common full-help entrypoints.
// It is additive to command-tree discovery and should only be selected when
// command discovery is weak and the flag surface is strong.
func DiscoverFlagDriven(binary string) (*FlagDiscoveryResult, error) {
	helpVariants := [][]string{
		{"--help", "all"},
		{"help", "all"},
		{"-h", "all"},
		{"--help", "full"},
		{"--help"},
		{"help"},
	}

	var rootHelp string
	for _, args := range helpVariants {
		text, err := runCLI(binary, args, 5*time.Second)
		if err != nil || strings.TrimSpace(text) == "" {
			continue
		}
		flags := parseFlagsWithValueHints(text)
		if len(flags) == 0 {
			continue
		}
		rootHelp = text
		break
	}
	if strings.TrimSpace(rootHelp) == "" {
		return nil, fmt.Errorf("flag discovery: no flag-rich help output from %q", binary)
	}

	flags := parseFlagsWithValueHints(rootHelp)
	capabilities := synthesizeCapabilities(flags)
	quality := assessFlagDiscoveryQuality(flags, capabilities)

	return &FlagDiscoveryResult{
		RootHelp:     compressHelpText(rootHelp),
		RootFlags:    flags,
		Capabilities: capabilities,
		QualityScore: quality.Score,
		Weak:         quality.Weak,
	}, nil
}

type flagDiscoveryQuality struct {
	Score float64
	Weak  bool
}

func assessFlagDiscoveryQuality(flags []Flag, capabilities []Capability) flagDiscoveryQuality {
	if len(flags) == 0 {
		return flagDiscoveryQuality{Weak: true}
	}
	described := 0
	for _, f := range flags {
		if strings.TrimSpace(f.Description) != "" {
			described++
		}
	}
	descRatio := float64(described) / float64(len(flags))
	score := float64(len(flags))*1.25 + float64(len(capabilities))*8 + descRatio*20
	if score > 100 {
		score = 100
	}
	return flagDiscoveryQuality{
		Score: score,
		Weak:  len(flags) < 12 || len(capabilities) < 3,
	}
}

func parseFlagsWithValueHints(text string) []Flag {
	var flags []Flag
	seen := make(map[string]bool)

	for _, line := range strings.Split(text, "\n") {
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

		valueHint := ""
		if shortMatch := strings.Split(strings.TrimSpace(line), "  ")[0]; shortMatch != "" {
			valueHint = extractValueHint(shortMatch)
		}

		flags = append(flags, Flag{
			Name:        name,
			Short:       short,
			ValueHint:   valueHint,
			Description: desc,
		})
	}

	sort.SliceStable(flags, func(i, j int) bool {
		return flags[i].Name < flags[j].Name
	})
	return flags
}

func extractValueHint(flagSpec string) string {
	flagSpec = strings.TrimSpace(flagSpec)
	fields := strings.Fields(flagSpec)
	if len(fields) == 0 {
		return ""
	}
	last := fields[len(fields)-1]
	if strings.HasPrefix(last, "-") {
		return ""
	}
	if strings.HasPrefix(last, "<") || strings.HasPrefix(last, "[") {
		return strings.Trim(last, "<>[]")
	}
	if strings.ContainsAny(last, "<[]>") {
		return strings.Trim(last, "<>[]")
	}
	if strings.ToUpper(last) == last {
		return strings.Trim(last, "<>[]")
	}
	return ""
}

func synthesizeCapabilities(flags []Flag) []Capability {
	type bucket struct {
		name        string
		description string
		keywords    []string
	}
	buckets := []bucket{
		{name: "request", description: "Control HTTP method, body, and request payloads", keywords: []string{"data", "json", "form", "request", "get", "head", "url-query"}},
		{name: "headers", description: "Inspect or send HTTP headers", keywords: []string{"header", "include", "dump-header", "referer", "user-agent"}},
		{name: "auth", description: "Authenticate requests with credentials or tokens", keywords: []string{"user", "oauth", "basic", "digest", "bearer", "netrc", "cert", "key"}},
		{name: "output", description: "Control where response bodies are written", keywords: []string{"output", "remote-name", "remote-header-name", "output-dir", "write-out"}},
		{name: "redirects", description: "Follow or shape redirect behavior", keywords: []string{"location", "max-redirs", "post301", "post302", "post303"}},
		{name: "transfer", description: "Upload, download, resume, or limit transfers", keywords: []string{"upload", "remote-time", "continue-at", "range", "rate", "limit-rate", "parallel", "retry", "max-time", "connect-timeout"}},
		{name: "proxy", description: "Configure proxies and proxy authentication", keywords: []string{"proxy", "proxytunnel", "proxy-user", "socks"}},
		{name: "tls", description: "Adjust TLS, certificates, and verification", keywords: []string{"tls", "ssl", "cacert", "capath", "insecure", "pinnedpubkey", "curves", "ciphers"}},
		{name: "debugging", description: "Inspect verbose output, traces, and failures", keywords: []string{"verbose", "trace", "trace-ascii", "trace-time", "show-error", "silent", "fail", "stderr"}},
	}

	assigned := make(map[string][]string)
	for _, f := range flags {
		text := strings.ToLower(f.Name + " " + f.Description)
		for _, b := range buckets {
			for _, kw := range b.keywords {
				if strings.Contains(text, kw) {
					assigned[b.name] = append(assigned[b.name], f.Name)
					break
				}
			}
		}
	}

	var capabilities []Capability
	for _, b := range buckets {
		flags := dedupeStrings(assigned[b.name])
		if len(flags) == 0 {
			continue
		}
		capabilities = append(capabilities, Capability{
			Name:        b.name,
			Description: b.description,
			Flags:       flags,
		})
	}
	return capabilities
}

func dedupeStrings(in []string) []string {
	seen := make(map[string]bool, len(in))
	var out []string
	for _, s := range in {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}
