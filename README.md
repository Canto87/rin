<p align="center">
  <br>
  <code>██████╗ ██╗███╗   ██╗</code><br>
  <code>██╔══██╗██║████╗  ██║</code><br>
  <code>██████╔╝██║██╔██╗ ██║</code><br>
  <code>██╔══██╗██║██║╚██╗██║</code><br>
  <code>██║  ██║██║██║ ╚████║</code><br>
  <code>╚═╝  ╚═╝╚═╝╚═╝  ╚═══╝</code><br>
  <br>
  <strong>凛 — Clear and resolute</strong><br>
  <sub>Autonomous development agent with persistent memory and multi-model routing</sub>
</p>

<p align="center">
  <a href="#quick-start">Quick Start</a> &middot;
  <a href="#how-it-works">How It Works</a> &middot;
  <a href="#architecture">Architecture</a> &middot;
  <a href="#team-mode">Team Mode</a> &middot;
  <a href="#commands">Commands</a> &middot;
  <a href="README.ko.md">한국어</a> &middot;
  <a href="README.ja.md">日本語</a>
</p>

---

RIN is an autonomous development agent built on [Claude Code](https://github.com/anthropics/claude-code). It wraps the CLI with **persistent long-term memory** (PostgreSQL + pgvector + AGE graph), **automatic session harvesting**, **multi-model routing** (Gemini, GLM), and a **self-defined identity** — turning a stateless LLM into a continuous collaborator that remembers past decisions, learns from mistakes, and maintains context across sessions.

## Quick Start

### 1. Install Prerequisites

| Tool | macOS | Linux |
|------|-------|-------|
| Python 3.11+ | `brew install python@3.12` | `apt install python3 python3-venv` |
| Go 1.26+ | `brew install go` | [go.dev/dl](https://go.dev/dl/) |
| Docker | [Docker Desktop](https://www.docker.com/products/docker-desktop/) | `apt install docker.io docker-compose-plugin` |
| Ollama | `brew install ollama` | [ollama.com](https://ollama.com/) |
| Claude Code | `npm i -g @anthropic-ai/claude-code` | same |

### 2. Install RIN

```bash
git clone https://github.com/Canto87/rin.git
cd project-rin-oss
make install
```

This runs the following in order:
1. `make check` — verify all prerequisites are installed
2. `make setup` — create Python venv (for session scripts)
3. `make install-db` — start PostgreSQL via Docker (PG17 + pgvector + AGE)
4. `make memory-go` — build Go memory server
5. `make pull-model` — start Ollama + pull embedding model (~670MB)
6. `make sync-mcp` — register MCP server in `~/.claude.json`
7. `make install-statusline` — install Claude Code statusline (usage + memory count)
8. `make install-harness-global` — deploy agents/skills/commands to `~/.claude/` (available in all projects)
9. `make install-cron` — register session harvest/review/dream (macOS launchd, skipped on Linux)
10. `make shell-setup` — add `rin` to PATH (auto-detects zsh/bash/fish)

### 3. Launch

```bash
source ~/.zshrc   # or restart your shell
rin
```

## How It Works

```
  rin                              # start
   ├─ session-picker.py            # choose: new / resume / load context
   ├─ rin-memory-recall            # inject recent memory into system prompt
   ├─ rin-context.md               # identity, principles, decision boundaries
   └─ claude                       # Claude Code with system prompt
        │
        ├─ rin-memory-go (MCP)     # semantic search, store decisions, graph relations
        │   ├─ PostgreSQL          #   structured metadata + full-text search
        │   ├─ pgvector            #   vector embeddings (Ollama, 1024-dim)
        │   └─ AGE                 #   knowledge graph (relation traversal)
        │
    [session ends]
        │
        ├─ session-harvest         # JSONL → markdown (launchd, 10min)
        └─ session-review          # RIN summarizes → memory_store (launchd, 1h)
```

**Session lifecycle:**

1. **Start** — Picker shows recent sessions. Resume one or start fresh with loaded context.
2. **Work** — RIN reads/writes memory via MCP tools. Decisions and patterns accumulate.
3. **End** — Session JSONL is automatically harvested into structured notes.
4. **Review** — A background RIN instance summarizes notes and extracts knowledge.
5. **Next session** — Recalled memory includes past decisions, active tasks, and patterns.

## Architecture

```
src/
  rin_memory_go/         # MCP server (Go, PostgreSQL + pgvector + AGE)
    main.go              #   entrypoint + MCP tool registration
    store.go             #   PostgreSQL connection + storage
    search.go            #   semantic + full-text hybrid search
    graph.go             #   AGE graph operations
    embed.go             #   Ollama embedding
    tools_memory.go      #   memory_* tools (store, search, lookup, update, relate, ingest)
    tools_routing.go     #   routing_* tools (suggest, log, stats)
    cmd_recall.go        #   recall subcommand (system prompt injection)
  rin_proxy/             # API proxy (Go, multi-model routing)
    main.go              #   HTTP server (:3456)
    openai.go            #   OpenAI-compatible API → provider translation
    passthrough.go       #   Anthropic models pass through directly
    streaming.go         #   SSE streaming support
scripts/
  rin                    #   entrypoint (banner + picker + claude)
  rin-team               #   team mode (multi-provider tmux)
  rin-cc                 #   exit team mode
  session-picker.py      #   interactive session selector
  session-harvest.py     #   JSONL → markdown (launchd)
  session-review.sh      #   RIN-powered summarization (launchd)
  rin-memory-recall.py   #   memory → system prompt injection
  sync-mcp.py            #   MCP config → ~/.claude.json
  migrate-to-pg.py       #   SQLite → PostgreSQL migration
context/
  rin-context.md         #   identity, principles, decision boundaries
launchd/                 #   macOS agent plists (templated)
config/
  mcp-servers.json       #   MCP server definitions
```

### Data

| Path | Purpose |
|------|---------|
| PostgreSQL `rin_memory` | Documents, vectors, relation graph |
| pgvector HNSW index | 1024-dim semantic search |
| AGE `rin_memory` graph | Knowledge relation traversal (supersedes, related, implements, contradicts) |
| `memory/sessions/` | Harvested session notes (pre-ingestion) |

### Memory Kinds

| Kind | Description |
|------|-------------|
| `session_journal` | Session title + summary |
| `arch_decision` | Architectural decisions with rationale |
| `domain_knowledge` | External service quirks, troubleshooting |
| `team_pattern` | Collaboration patterns, workflow rules |
| `active_task` | Unfinished work carried across sessions |
| `error_pattern` | Recurring error patterns and solutions |
| `preference` | User preferences (workflow, tools, style) |
| `routing_log` | Model routing performance data |

## Team Mode

`rin-team` combines Opus (leader) with other provider models (teammates) for multi-agent teams.

```bash
rin-team gemini          # teammates: Gemini
rin-team glm             # teammates: GLM
rin-team all             # opus→Gemini Pro, sonnet→GLM-5, haiku→Gemini Flash
```

```
  rin-team gemini
   │
   ├─ rin-proxy (:3456)             # API gateway
   │
   ├─ Leader (claude-opus-4-6)     # → proxy → Anthropic (passthrough)
   │   └─ Design, review, orchestration
   │
   ├─ Teammate (sonnet alias)      # → proxy → Gemini
   │   └─ Implementation, research, testing
   │
   └─ Teammate (haiku alias)       # → proxy → Gemini Flash
       └─ Quick tasks, exploration
```

**Prerequisite:** Register rin-proxy via `make install-proxy`.

## Development Workflow

### Daily Usage

```bash
rin                          # launch RIN — picker shows recent sessions
rin --resume <session-id>    # resume a specific session
```

RIN remembers across sessions. Decisions, error patterns, and preferences are stored in memory and automatically recalled on next launch.

### Built-in Skills & Commands

```bash
/commit          # auto-grouped commits with meaningful messages
/pr              # create PR with summary and test plan
/code-review     # weighted code review of current changes
```

These are defined in `.claude/commands/` and delegate to skills in `.claude/skills/`.

### Deploying to Other Projects

RIN's harness (agents, skills, commands) can be deployed per-project or globally:

```bash
# Per-project — copies to target/.claude/
make sync-harness TARGET=~/workspace/other-project

# Global — copies to ~/.claude/, available in all projects
make sync-harness TARGET=global
```

This copies `skill.md` files. Per-project `config.yaml` files are not overwritten. Global deploy is recommended if you use RIN's harness across multiple projects.

### Customization

- **`context/rin-context.md`** — Behavioral principles and decision boundaries. Edit to change how RIN works.
- **`context/rin-context-local.md`** — Environment-specific overrides (gitignored). Create this file to add local rules without modifying the shared context. Content is appended to the system prompt after `rin-context.md`.
- **`.claude/skills/*/config.yaml`** — Per-skill configuration (thresholds, modes).
- **`~/.rin/memory-config.json`** — Database DSN, Ollama URL overrides.

Example `rin-context-local.md`:
```markdown
## Local Overrides
- Always respond in Japanese.
- Use Serena MCP for code navigation when available.
- Default commit messages in English.
```

## Commands

### Core

```
make install            Full install (venv + Docker PG + Go build + MCP + model + launchd + PATH)
make rin                Launch RIN
make test               Run full pipeline test in Docker (build + unit tests + MCP server)
make test-install       Run install pipeline test in Docker (sync-mcp, statusline, harness, shell-setup)
```

### Individual Steps

`make install` runs all of these, but they can also be run independently:

```
make check              Check prerequisites (Python, Go, Docker, Ollama)
make setup              Create Python venv (for session scripts)
make install-db         Start PostgreSQL via Docker (PG17 + pgvector + AGE)
make memory-go          Build Go memory server
make proxy              Build Go proxy
make install-cron       Register session harvest/review/dream launchd agents
make sync-mcp           Sync MCP config to ~/.claude.json
make shell-setup        Add RIN scripts to PATH
```

### Operations

```
make harvest            Run session harvest manually
make review             Run session review manually
make dream              Run memory consolidation manually
make team               Team mode: Claude lead + provider teammates (gemini|glm|all)
make cc              Exit team mode
make sync-harness       Deploy harness to another project (TARGET=<path>)
make help               Show all targets
```

### Optional

```bash
# rin-proxy (team mode prerequisite)
GEMINI_API_KEY=<key> GLM_API_KEY=<key> make install-proxy

# Ollama always-on (otherwise starts on-demand)
make install-ollama
```

### Cleanup

```bash
make uninstall-db       Stop and remove PostgreSQL container + data
make uninstall-cron     Remove launchd agents
make uninstall-proxy    Remove rin-proxy launchd agent
```

## License

MIT
