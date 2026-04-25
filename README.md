# nlci

Natural language interface layer for any CLI tool. Drop a YAML file, get `nlci docker "clean up stopped containers"`. Powered by Apple Intelligence or llama.cpp — no cloud, no API keys.

```
nlci docker "clean up stopped containers"

  > docker rm $(docker ps -aq -f status=exited)
    Removes all stopped containers by their IDs

Run this command? [y/N]
```

## Requirements

**Apple Intelligence backend** (primary, on-device, fastest)
- macOS 26 (Tahoe) or later
- Apple Silicon (M1 or later)
- Apple Intelligence enabled in System Settings → Apple Intelligence
- Xcode installed (the macOS 26 SDK is bundled — no separate download)

**Ollama / llama.cpp / LM Studio** (any Mac, Intel or Apple Silicon)
- One of: [Ollama](https://ollama.com), [llama.cpp server](https://github.com/ggerganov/llama.cpp), [LM Studio](https://lmstudio.ai)
- A model loaded (e.g. `ollama pull llama3.2:3b`)

The install script auto-detects your setup and configures accordingly.

## Installation

```bash
curl -fsSL https://raw.githubusercontent.com/jakejimenez/nlci/main/install.sh | sh
```

Installs `nlci` to `~/.local/bin`. On macOS 26 + Apple Silicon, also builds and installs the Apple Intelligence bridge automatically. No `sudo` required.

If `~/.local/bin` is not in your PATH, the script will tell you what to add to your shell profile.

<details>
<summary>Install from source manually</summary>

```bash
git clone https://github.com/jakejimenez/nlci
cd nlci
make build
make install           # installs nlci to ~/.local/bin

# Apple Intelligence bridge (macOS 26 + Apple Silicon)
make build-apple
make install-apple     # installs nlci-apple to ~/.config/nlci/bin/
```

</details>

## How it works

nlci sits between you and any CLI tool. You describe what you want in plain English; nlci translates it to the exact command, explains it, asks for confirmation on destructive operations, and runs it.

```
Your intent
    │
    ▼
Retrieval index     lexical top-K match against command names, descriptions, examples
    │
    ▼
Prompt builder      system prompt + candidate schema + few-shot examples + live --help + intent
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
Agentic loop        inject error context → retry (max 3×); widen candidates on misrouting
```

All inference runs on-device. No data leaves your machine.

## Usage

```bash
# Use a bundled definition (docker, gh)
nlci docker "show me running containers"
nlci gh "list my open pull requests"

# Quotes are optional for multi-word intents
nlci docker show me running containers
nlci gh list my open pull requests

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

# true = also discover commands from `mytool --help` at runtime
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
client, err := nlci.New(nlci.Config{ToolName: "docker"})
if err != nil {
    log.Fatal(err)
}
if err := client.Run(ctx, "show me running containers"); err != nil {
    log.Fatal(err)
}

// Inference only — no execution
result, err := client.Generate(ctx, "clean up stopped containers")
if err != nil {
    log.Fatal(err)
}
fmt.Println(result.Command)      // docker rm $(docker ps -aq -f status=exited)
fmt.Println(result.Explanation)  // Removes all stopped containers
fmt.Println(result.Backend)      // apple
fmt.Println(result.Attempts)     // 1
```

## Architecture

```
nlci/
├── apple/                   Swift Package — Apple Intelligence bridge
│   └── Sources/NLCIApple/   @Generable CommandResult, App.swift entry point
├── cmd/nlci/                Cobra CLI: run, init, config
├── internal/
│   ├── definition/          YAML loader + --help auto-discovery (one level deep)
│   ├── retrieval/           Lexical top-K retrieval index (no model call)
│   ├── prompt/              Prompt builder + 3,500-token budget enforcer
│   ├── backend/             Apple subprocess + OpenAI-compat (Ollama/llama.cpp/LM Studio)
│   ├── validator/           Schema conformance + safety rules
│   ├── executor/            Confirm prompt + subprocess execution
│   └── agent/               Retrieve → infer → validate → execute loop (max 3×)
├── config/                  ~/.config/nlci/config.yaml
├── definitions/             docker.nlci.yaml, gh.nlci.yaml
└── nlci.go                  Public SDK surface
```

The Swift binary (`nlci-apple`) is a subprocess invoked per-request. It reads a JSON payload from stdin, runs `LanguageModelSession.respond(to:generating:)` with `@Generable` structured output, and writes the result as JSON to stdout. The Go core handles everything else.

## Token budget

Apple Intelligence has a 4,096-token context window. nlci keeps within a 3,500-token safe ceiling. The retrieval step sends only the top-K matched commands to the prompt builder (not the full schema), which keeps typical requests well under budget even for large CLI definitions.

## Verification

```bash
# 1. Build the Go binary
make build

# 2. Check backend health
./bin/nlci config

# 3. Dry-run with no backend — should fail with a clear "no inference backend" message
./bin/nlci docker "show me running containers" --dry-run

# 4. Apple Intelligence (macOS 26 + Apple Silicon)
make build-apple && make install-apple
./bin/nlci docker "show me running containers" --dry-run
./bin/nlci docker "remove all stopped containers" --dry-run  # triggers confirmation prompt

# 5. Ollama (any Mac)
ollama pull llama3.2:3b && ollama serve &
./bin/nlci docker "show me running containers" --dry-run
```

## License

MIT
