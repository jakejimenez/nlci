// Package nlci provides a natural language interface layer for any CLI tool.
// It uses on-device inference (Apple Intelligence, Ollama, llama.cpp, or LM Studio)
// to translate natural language intent into CLI commands.
//
// Basic usage:
//
//	a, err := nlci.New(nlci.Config{DefinitionPath: "docker.nlci.yaml"})
//	if err != nil {
//	    log.Fatal(err)
//	}
//	if err := a.Run(context.Background(), "show me running containers"); err != nil {
//	    log.Fatal(err)
//	}
package nlci

import (
	"context"
	"fmt"

	"github.com/jakejimenez/nlci/config"
	"github.com/jakejimenez/nlci/internal/agent"
	"github.com/jakejimenez/nlci/internal/backend"
	"github.com/jakejimenez/nlci/internal/definition"
)

// BackendHint controls which inference backend is used.
type BackendHint string

const (
	AutoDetect  BackendHint = ""
	ForceApple  BackendHint = "apple"
	ForceOllama BackendHint = "ollama"
	ForceLlamaCpp BackendHint = "llamacpp"
	ForceLMStudio BackendHint = "lmstudio"
)

// Config controls the behavior of a Client.
type Config struct {
	// DefinitionPath is an explicit path to a *.nlci.yaml file.
	// If empty, the tool name is derived from the definition or binary.
	DefinitionPath string

	// ToolName is used to find the definition when DefinitionPath is empty.
	ToolName string

	// Backend forces a specific inference backend. Default is AutoDetect.
	Backend BackendHint

	// DryRun prints the generated command without executing it.
	DryRun bool

	// Explain always prints the explanation alongside the command.
	Explain bool
}

// Result is the outcome of a Generate or Run call.
type Result struct {
	Command     string
	Explanation string
	Backend     string
	Attempts    int
}

// Client is the main nlci entry point.
type Client struct {
	def     *definition.CLIDefinition
	backend *backend.Router
	agent   *agent.Agent
}

// New creates a Client from the given Config.
func New(cfg Config) (*Client, error) {
	appConfig, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("nlci: load config: %w", err)
	}

	var def *definition.CLIDefinition
	if cfg.DefinitionPath != "" {
		def, err = definition.LoadFile(cfg.DefinitionPath)
	} else {
		toolName := cfg.ToolName
		if toolName == "" {
			return nil, fmt.Errorf("nlci: either DefinitionPath or ToolName must be set")
		}
		def, err = definition.Load(toolName, appConfig.Definitions.Paths)
	}
	if err != nil {
		return nil, fmt.Errorf("nlci: load definition: %w", err)
	}

	br := buildRouter(appConfig, cfg.Backend)

	agentOpts := agent.Options{
		DryRun:  cfg.DryRun,
		Explain: cfg.Explain,
	}
	a := agent.New(def, br, agentOpts)

	return &Client{
		def:     def,
		backend: br,
		agent:   a,
	}, nil
}

// Run translates the natural language intent into a command and executes it.
func (c *Client) Run(ctx context.Context, intent string) error {
	_, err := c.agent.Run(ctx, intent)
	return err
}

// Generate translates the natural language intent into a command without executing it.
func (c *Client) Generate(ctx context.Context, intent string) (*Result, error) {
	r, err := c.agent.Generate(ctx, intent)
	if err != nil {
		return nil, err
	}
	return &Result{
		Command:     r.Command,
		Explanation: r.Explanation,
		Backend:     r.Backend,
		Attempts:    r.Attempts,
	}, nil
}

// Definition returns the loaded CLIDefinition.
func (c *Client) Definition() *definition.CLIDefinition {
	return c.def
}

// BackendStatus returns a map of backend name → health error (nil = healthy).
func (c *Client) BackendStatus(ctx context.Context) map[string]error {
	return c.backend.Status(ctx)
}

func buildRouter(appConfig config.Config, hint BackendHint) *backend.Router {
	if hint != AutoDetect {
		// Force a specific backend
		switch hint {
		case ForceApple:
			return backend.NewRouterWithBackends([]backend.Backend{
				backend.NewApple(appConfig.Apple.Binary),
			})
		case ForceOllama:
			return backend.NewRouterWithBackends([]backend.Backend{
				backend.NewOllama(appConfig.Backend.Ollama.Host, appConfig.Backend.Ollama.Model),
			})
		case ForceLlamaCpp:
			return backend.NewRouterWithBackends([]backend.Backend{
				backend.NewLlamaCpp(appConfig.Backend.LlamaCpp.Host, appConfig.Backend.LlamaCpp.Model),
			})
		case ForceLMStudio:
			return backend.NewRouterWithBackends([]backend.Backend{
				backend.NewLMStudio(appConfig.Backend.LMStudio.Host, appConfig.Backend.LMStudio.Model),
			})
		}
	}
	return backend.NewRouter(appConfig)
}
