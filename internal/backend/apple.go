package backend

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// AppleBackend runs inference via the nlci-apple Swift subprocess.
// The Swift binary reads a JSON BridgeInput from stdin and writes a
// JSON BridgeOutput to stdout.
type AppleBackend struct {
	binaryPath   string
	capabilities []string // populated at Ping time; empty slice means "old binary, command-only"
}

// bridgeInput mirrors BridgeInput.swift.
type bridgeInput struct {
	Mode     string      `json:"mode,omitempty"`
	System   string      `json:"system"`
	Schema   string      `json:"schema"`
	Examples [][2]string `json:"examples"`
	Intent   string      `json:"intent"`
}

// bridgeOutput mirrors BridgeOutput.swift.
type bridgeOutput struct {
	Mode         string         `json:"mode,omitempty"`
	Command      string         `json:"command,omitempty"`
	Explanation  string         `json:"explanation,omitempty"`
	Metadata     *appleMetadata `json:"metadata,omitempty"`
	Capabilities []string       `json:"capabilities,omitempty"`
	Error        string         `json:"error,omitempty"`
}

// appleMetadata mirrors ToolMetadata.swift, including the array-of-pairs
// shape for synonyms (FoundationModels' @Generable doesn't support
// Dictionary directly).
type appleMetadata struct {
	Description  string              `json:"description"`
	SystemPrompt string              `json:"systemPrompt"`
	Safety       appleSafetyRules    `json:"safety"`
	Synonyms     []appleSynonymEntry `json:"synonyms"`
}

type appleSafetyRules struct {
	RequireConfirmation []string `json:"requireConfirmation"`
	Forbidden           []string `json:"forbidden"`
}

type appleSynonymEntry struct {
	Keyword string   `json:"keyword"`
	Targets []string `json:"targets"`
}

// NewApple creates an AppleBackend. binaryPath is the path to the nlci-apple binary.
func NewApple(binaryPath string) *AppleBackend {
	return &AppleBackend{binaryPath: binaryPath}
}

func (a *AppleBackend) Name() string { return "apple" }

// Ping checks that the nlci-apple binary exists and that Apple Intelligence
// is available on this device. The ping response also advertises which
// schemas the binary supports (e.g. "command", "metadata") so callers can
// gate optional capabilities without speculatively trying them.
func (a *AppleBackend) Ping(ctx context.Context) error {
	if _, err := os.Stat(a.binaryPath); err != nil {
		return fmt.Errorf("%w: nlci-apple binary not found at %s", ErrBackendUnavailable, a.binaryPath)
	}

	cmd := exec.CommandContext(ctx, a.binaryPath, "--ping")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%w: nlci-apple ping failed: %s", ErrBackendUnavailable, err)
	}

	// Parse capabilities; old binaries don't emit them, leaving the slice nil.
	var out bridgeOutput
	if err := json.Unmarshal(stdout.Bytes(), &out); err == nil {
		a.capabilities = out.Capabilities
	}

	return nil
}

// HasCapability reports whether the underlying Swift binary advertised the
// named mode at ping time. Returns false for binaries that predate the
// capability advertisement (i.e. only support the implicit "command" mode).
func (a *AppleBackend) HasCapability(name string) bool {
	for _, c := range a.capabilities {
		if c == name {
			return true
		}
	}
	return false
}

// Generate runs the Swift subprocess, sends the request via stdin,
// and reads the response from stdout.
func (a *AppleBackend) Generate(ctx context.Context, r Request) (Response, error) {
	// bridgeInput keeps schema/examples fields for wire compatibility with
	// existing nlci-apple binaries; they are intentionally empty — the system
	// prompt already embeds the schema, and examples are part of the user prompt.
	input := bridgeInput{
		System:   r.System,
		Schema:   "",
		Examples: make([][2]string, 0),
		Intent:   r.Intent,
	}

	inputJSON, err := json.Marshal(input)
	if err != nil {
		return Response{}, fmt.Errorf("apple: marshal input: %w", err)
	}

	cmd := exec.CommandContext(ctx, a.binaryPath)
	cmd.Stdin = bytes.NewReader(inputJSON)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		// Check if stderr contains useful diagnostic info
		stderrStr := strings.TrimSpace(stderr.String())
		if stderrStr != "" {
			return Response{}, fmt.Errorf("apple: subprocess failed: %w\n%s", err, stderrStr)
		}
		return Response{}, fmt.Errorf("apple: subprocess failed: %w", err)
	}

	var out bridgeOutput
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		return Response{}, fmt.Errorf("apple: invalid output: %w (raw: %s)", err, stdout.String())
	}

	if out.Error != "" {
		if strings.HasPrefix(out.Error, "unavailable:") {
			return Response{}, fmt.Errorf("%w: %s", ErrBackendUnavailable, out.Error)
		}
		return Response{}, fmt.Errorf("apple: %s", out.Error)
	}

	return Response{
		Command:     out.Command,
		Explanation: out.Explanation,
	}, nil
}

// GenerateMetadata implements MetadataGenerator. It invokes the Swift bridge
// in metadata mode, which constrains the model output to a Generable
// ToolMetadata schema instead of the default CommandResult.
//
// Returns an error advising a rebuild when the binary doesn't advertise the
// "metadata" capability — old binaries silently produce a CommandResult and
// the metadata field would be absent.
func (a *AppleBackend) GenerateMetadata(ctx context.Context, r MetadataRequest) (*MetadataResult, error) {
	if !a.HasCapability("metadata") {
		return nil, fmt.Errorf("apple: nlci-apple binary does not advertise metadata mode — run `make build-apple && make install-apple` to upgrade")
	}

	input := bridgeInput{
		Mode:     "metadata",
		System:   r.System,
		Examples: make([][2]string, 0),
		Intent:   r.Prompt,
	}

	inputJSON, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("apple: marshal metadata input: %w", err)
	}

	cmd := exec.CommandContext(ctx, a.binaryPath)
	cmd.Stdin = bytes.NewReader(inputJSON)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		stderrStr := strings.TrimSpace(stderr.String())
		if stderrStr != "" {
			return nil, fmt.Errorf("apple: metadata subprocess failed: %w\n%s", err, stderrStr)
		}
		return nil, fmt.Errorf("apple: metadata subprocess failed: %w", err)
	}

	var out bridgeOutput
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		return nil, fmt.Errorf("apple: invalid metadata output: %w (raw: %s)", err, stdout.String())
	}

	if out.Error != "" {
		if strings.HasPrefix(out.Error, "unavailable:") {
			return nil, fmt.Errorf("%w: %s", ErrBackendUnavailable, out.Error)
		}
		return nil, fmt.Errorf("apple: %s", out.Error)
	}

	if out.Metadata == nil {
		return nil, fmt.Errorf("apple: metadata field absent in response (raw: %s)", stdout.String())
	}

	syn := make(map[string][]string, len(out.Metadata.Synonyms))
	for _, e := range out.Metadata.Synonyms {
		if e.Keyword == "" || len(e.Targets) == 0 {
			continue
		}
		// Append + dedupe in case the model emits the same keyword twice.
		seen := make(map[string]bool, len(syn[e.Keyword]))
		for _, t := range syn[e.Keyword] {
			seen[t] = true
		}
		for _, t := range e.Targets {
			if !seen[t] {
				syn[e.Keyword] = append(syn[e.Keyword], t)
				seen[t] = true
			}
		}
	}

	return &MetadataResult{
		Description:  out.Metadata.Description,
		SystemPrompt: out.Metadata.SystemPrompt,
		Safety: Safety{
			RequireConfirmation: out.Metadata.Safety.RequireConfirmation,
			Forbidden:           out.Metadata.Safety.Forbidden,
		},
		Synonyms: syn,
	}, nil
}
