# nlci

Natural language interface layer for any CLI tool. Drop a YAML file, get `docker "show me containers using more than 500MB"`. Powered by Apple Intelligence or llama.cpp — no cloud, no API keys.

```
nlci docker "show me containers using more than 500MB"

  > docker ps -s --filter status=running
    Lists running containers and their disk usage sizes

Run this command? [y/N]
```

## How it works

nlci sits between you and any CLI tool. You describe what you want in plain English; nlci translates it to the exact command, explains it, asks for confirmation on destructive operations, and runs it.

```
Your intent
    │
    ▼
Cascading router    keyword match → synonym dict → routing inference
    │
    ▼
Prompt builder      system prompt + schema + few-shot examples + intent
    │
    ▼
Inference backend   Apple Intelligence → Ollama → llama.cpp → LM Studio
    │
    ▼
Validator           schema conformance + safety rules
    │
    ▼
Executor            confirm (if needed) → run → stream output
    │
    ▼ on error
Agentic loop        inject --help + error context → retry (max 3×)
```

All inference runs on-device. No data leaves your machine.

## Requirements

**For Apple Intelligence backend:**
- macOS 26 (Tahoe) or later
- Apple Silicon (M1 or later)
- Apple Intelligence enabled in System Settings

**For Ollama / llama.cpp / LM Studio backend:**
- Any Mac (Intel or Apple Silicon)
- One of: [Ollama](https://ollama.com), [llama.cpp server](https://github.com/ggerganov/llama.cpp), or [LM Studio](https://lmstudio.ai)
- A 3B parameter model (e.g. `llama3.2:3b` via Ollama)

## Installation

```bash
git clone https://github.com/jakejimenez/nlci
cd nlci
make build
make install          # installs nlci to /usr/local/bin

# Apple Intelligence backend (macOS 26 + Apple Silicon required)
make build-apple
make install-apple    # installs nlci-apple to ~/.config/nlci/bin/
```

## Usage

```bash
# Use a bundled definition (docker, gh)
nlci docker "show me running containers"
nlci gh "list my open pull requests"

# Dry-run: see the command without executing it
nlci docker "clean up stopped containers" --dry-run

# Always show the explanation
nlci gh "approve PR 42" --explain

# Force a specific backend
nlci docker "pull the latest nginx" --backend ollama

# Check configuration and backend health
nlci config

# Scaffold a definition for any CLI tool
nlci init kubectl
```

## Bundled definitions

| Tool | Commands |
|------|----------|
| `docker` | ps, run, stop, rm, images, pull, build, exec, logs, inspect, stats, system prune, network, volume |
| `gh` | pr create/list/view/merge/checkout/checks/review/close/diff, issue create/list/view/close, repo clone/view/create/fork/list, run list/view/watch/rerun, release create/list/view, search, status, browse |

## Defining your own CLI

Run `nlci init <tool>` to scaffold a definition from the tool's `--help` output:

```bash
nlci init kubectl
# Creates kubectl.nlci.yaml in the current directory
# Edit it to add examples and a system_prompt
```

Or write one by hand:

```yaml
# mytool.nlci.yaml
name: mytool
description: My CLI tool
binary: mytool

system_prompt: |
  You are an expert mytool user. Generate precise mytool commands.
  Output only the raw command.

commands:
  - name: query
    description: Run a query
    examples:
      - nl: "show me all active records"
        cmd: "mytool query --status active"

safety:
  require_confirmation:
    - "mytool delete"
  forbidden:
    - "mytool delete --all --force"

auto_discover: true
```

nlci searches for definitions in this order:
1. `<tool>.nlci.yaml` in the current directory
2. `~/.config/nlci/definitions/`
3. Bundled definitions (`docker`, `gh`)
4. Zero-config: auto-discover from `<tool> --help`

## Configuration

`~/.config/nlci/config.yaml`:

```yaml
backend:
  priority: [apple, ollama, llamacpp, lmstudio]
  ollama:
    host: localhost:11434
    model: llama3.2:3b
  llamacpp:
    host: localhost:8080
    model: ""
  lmstudio:
    host: localhost:1234
    model: ""

apple:
  binary: ~/.config/nlci/bin/nlci-apple
```

nlci tries each backend in priority order and uses the first healthy one. Run `nlci config` to see current health status.

## SDK

```go
import "github.com/jakejimenez/nlci"

// Full pipeline: infer + confirm + execute
client, err := nlci.New(nlci.Config{
    ToolName: "docker",
    DryRun:   false,
})
if err := client.Run(ctx, "show me running containers"); err != nil {
    log.Fatal(err)
}

// Inference only — no execution
result, err := client.Generate(ctx, "clean up stopped containers")
fmt.Println(result.Command)      // docker system prune
fmt.Println(result.Explanation)  // Removes stopped containers, unused images...
fmt.Println(result.Backend)      // apple
fmt.Println(result.Attempts)     // 1
```

## Architecture

```
nlci/
├── apple/                   Swift Package — Apple Intelligence bridge
│   └── Sources/NLCIApple/   @Generable CommandResult, RunLoop, EOF guard
├── cmd/nlci/                Cobra CLI: run, init, config
├── internal/
│   ├── definition/          YAML loader + --help auto-discovery
│   ├── prompt/              Prompt builder + 3,500-token budget enforcer
│   ├── router/              Keyword → synonym → routing inference cascade
│   ├── backend/             Apple subprocess + OpenAI-compat (Ollama/llama.cpp/LM Studio)
│   ├── validator/           Schema conformance + safety rules
│   ├── executor/            Confirm prompt + subprocess execution
│   └── agent/               One-shot + agentic retry loop (max 3×)
├── config/                  ~/.config/nlci/config.yaml
├── definitions/             docker.nlci.yaml, gh.nlci.yaml
└── nlci.go                  Public SDK surface
```

The Swift binary (`nlci-apple`) is a subprocess invoked per-request. It reads a JSON payload from stdin, runs `LanguageModelSession.respond(to:generating:)` with `@Generable` structured output, and writes the result as JSON to stdout. The Go core handles everything else.

## Token budget

Apple Intelligence has a 4,096-token context window. A typical nlci request uses ~955 tokens, leaving ~3,141 tokens of headroom. For large CLI schemas, the prompt builder pre-filters commands by keyword relevance before sending.

## License

MIT
