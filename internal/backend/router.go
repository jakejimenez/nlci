package backend

import (
	"context"
	"errors"
	"fmt"

	"github.com/jakejimenez/nlci/config"
)

// Router selects the first healthy backend from a priority list.
type Router struct {
	backends []Backend
	active   Backend // cached after first successful Ping
}

// NewRouter builds a Router from the application config.
func NewRouter(cfg config.Config) *Router {
	// Build the ordered backend list from config priority
	backendMap := map[string]Backend{
		"apple":    NewApple(cfg.Apple.Binary),
		"ollama":   NewOllama(cfg.Backend.Ollama.Host, cfg.Backend.Ollama.Model),
		"llamacpp": NewLlamaCpp(cfg.Backend.LlamaCpp.Host, cfg.Backend.LlamaCpp.Model),
		"lmstudio": NewLMStudio(cfg.Backend.LMStudio.Host, cfg.Backend.LMStudio.Model),
	}

	var ordered []Backend
	for _, name := range cfg.Backend.Priority {
		if b, ok := backendMap[name]; ok {
			ordered = append(ordered, b)
		}
	}

	// Fallback: if priority is empty, use default order
	if len(ordered) == 0 {
		ordered = []Backend{
			backendMap["apple"],
			backendMap["ollama"],
			backendMap["llamacpp"],
			backendMap["lmstudio"],
		}
	}

	return &Router{backends: ordered}
}

// NewRouterWithBackends creates a Router from an explicit backend list.
// Useful for tests and the SDK.
func NewRouterWithBackends(backends []Backend) *Router {
	return &Router{backends: backends}
}

// Resolve finds and caches the first healthy backend.
func (r *Router) Resolve(ctx context.Context) (Backend, error) {
	if r.active != nil {
		return r.active, nil
	}

	var lastErr error
	for _, b := range r.backends {
		if err := b.Ping(ctx); err != nil {
			if errors.Is(err, ErrBackendUnavailable) {
				lastErr = err
				continue
			}
			lastErr = err
			continue
		}
		r.active = b
		return b, nil
	}

	return nil, fmt.Errorf("no available backend: %w", lastErr)
}

// Generate resolves the active backend and runs inference.
func (r *Router) Generate(ctx context.Context, req Request) (Response, error) {
	b, err := r.Resolve(ctx)
	if err != nil {
		return Response{}, err
	}
	return b.Generate(ctx, req)
}

// GenerateRaw resolves the active backend and runs a raw inference call.
func (r *Router) GenerateRaw(ctx context.Context, system, prompt string) (string, error) {
	b, err := r.Resolve(ctx)
	if err != nil {
		return "", err
	}
	return b.GenerateRaw(ctx, system, prompt)
}

// ActiveName returns the name of the currently active backend, or empty string.
func (r *Router) ActiveName() string {
	if r.active != nil {
		return r.active.Name()
	}
	return ""
}

// Status returns a map of backend name → error (nil = healthy).
func (r *Router) Status(ctx context.Context) map[string]error {
	result := make(map[string]error, len(r.backends))
	for _, b := range r.backends {
		result[b.Name()] = b.Ping(ctx)
	}
	return result
}
