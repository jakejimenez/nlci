package backend

import (
	"context"
	"errors"
)

// ErrBackendUnavailable is returned by Ping or Generate when a backend
// is not available. The router uses this to fall through to the next backend.
var ErrBackendUnavailable = errors.New("backend unavailable")

// Request is the input to an inference call.
type Request struct {
	System   string      `json:"system"`
	Schema   string      `json:"schema"`
	Examples [][2]string `json:"examples"`
	Intent   string      `json:"intent"`
}

// Response is the output from an inference call.
type Response struct {
	Command     string `json:"command"`
	Explanation string `json:"explanation"`
}

// Backend is the interface all inference backends must implement.
type Backend interface {
	// Name returns the backend identifier (e.g. "apple", "ollama").
	Name() string
	// Ping checks whether the backend is available and healthy.
	// Returns ErrBackendUnavailable if it cannot be used.
	Ping(ctx context.Context) error
	// Generate runs inference and returns the generated command.
	Generate(ctx context.Context, r Request) (Response, error)
	// GenerateRaw runs a simple system+prompt inference call, returning raw text.
	// Used by the routing inference step.
	GenerateRaw(ctx context.Context, system, prompt string) (string, error)
}
