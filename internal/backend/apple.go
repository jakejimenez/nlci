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
	binaryPath string
}

// bridgeInput mirrors BridgeInput.swift.
type bridgeInput struct {
	System   string      `json:"system"`
	Schema   string      `json:"schema"`
	Examples [][2]string `json:"examples"`
	Intent   string      `json:"intent"`
}

// bridgeOutput mirrors BridgeOutput.swift.
type bridgeOutput struct {
	Command     string `json:"command"`
	Explanation string `json:"explanation"`
	Error       string `json:"error"`
}

// NewApple creates an AppleBackend. binaryPath is the path to the nlci-apple binary.
func NewApple(binaryPath string) *AppleBackend {
	return &AppleBackend{binaryPath: binaryPath}
}

func (a *AppleBackend) Name() string { return "apple" }

// Ping checks that the nlci-apple binary exists and that Apple Intelligence
// is available on this device.
func (a *AppleBackend) Ping(ctx context.Context) error {
	if _, err := os.Stat(a.binaryPath); err != nil {
		return fmt.Errorf("%w: nlci-apple binary not found at %s", ErrBackendUnavailable, a.binaryPath)
	}

	// Run with --ping flag to check Apple Intelligence availability
	cmd := exec.CommandContext(ctx, a.binaryPath, "--ping")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%w: nlci-apple ping failed: %s", ErrBackendUnavailable, err)
	}

	return nil
}

// Generate runs the Swift subprocess, sends the request via stdin,
// and reads the response from stdout.
func (a *AppleBackend) Generate(ctx context.Context, r Request) (Response, error) {
	// Ensure Examples is never marshalled as JSON null — Swift's non-optional
	// [[String]] decoder will throw on null even with decodeIfPresent as a safeguard.
	examples := r.Examples
	if examples == nil {
		examples = make([][2]string, 0)
	}
	input := bridgeInput{
		System:   r.System,
		Schema:   r.Schema,
		Examples: examples,
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

// GenerateRaw runs a simple single-turn prompt through the Swift binary.
// Used for routing inference calls.
func (a *AppleBackend) GenerateRaw(ctx context.Context, system, prompt string) (string, error) {
	r := Request{
		System: system,
		Intent: prompt,
	}
	resp, err := a.Generate(ctx, r)
	if err != nil {
		return "", err
	}
	return resp.Command, nil
}
