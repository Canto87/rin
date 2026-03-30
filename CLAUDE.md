# project-rin-oss

RIN infrastructure project. Memory, session pipeline, execution environment.

## Structure

```
src/rin_memory/     # MCP server (fastmcp) — long-term memory store/search
  server.py         # MCP tool definitions (memory_*, routing_*)
  store.py          # SQLite + LanceDB storage (legacy Python module)
  embedding.py      # Ollama embeddings
  ingest.py         # Markdown parsing/ingestion
src/rin_memory_go/  # Go MCP server (primary) — PostgreSQL + pgvector + AGE
  main.go           # MCP server entry, subcommands (recall, insert-log, reindex)
  store.go          # PostgreSQL + pgvector storage
  embed.go          # Ollama embeddings
  config.go         # Config loader (~/.rin/memory-config.json)
  schema.sql        # Database schema (auto-applied on first connect)
  cmd_recall.go     # Session start memory recall
  tools_memory.go   # memory_* MCP tools
  tools_routing.go  # routing_* MCP tools
src/rin_proxy/      # Go — Anthropic API ↔ Gemini API conversion proxy
  main.go           # HTTP server, model-based routing
  request.go        # Anthropic → Gemini request conversion
  response.go       # Gemini → Anthropic response conversion
  streaming.go      # Gemini SSE → Anthropic SSE lifecycle synthesis
  types.go          # API types + ToolMap
  config.go         # Config loader (~/.rin/proxy-config.json)
  passthrough.go    # Anthropic API passthrough
.claude/
  agents/           # Agent definitions (code-edit, code-review)
  skills/           # Skill definitions (auto-impl, gc, plan-feature, qa-gate, etc.)
  commands/         # Commands (commit, pr, code-review)
scripts/
  rin               # RIN launcher (claude --append-system-prompt)
  sync-harness.sh   # Deploy harness files to other projects
  rin-team          # Team mode: Claude leader + provider teammates (tmux env isolation)
  rin-cc            # Team mode exit (tmux env removal)
  session-harvest.py   # Session log collection (launchd 10min)
  session-review.sh    # Session transcript summarization + knowledge extraction (launchd 1h)
  memory-dream.sh      # Memory consolidation (launchd 24h)
  rin-memory-recall.py # Load recent memory at session start
context/
  rin-context.md    # RIN principles and decision boundaries (injected as system prompt)
launchd/            # macOS launchd plist
```

## Tech Stack

- Python 3.11+, hatchling
- fastmcp 2.0+ (MCP server)
- PostgreSQL 17+ (metadata + full-text) + pgvector (vector search) + AGE (knowledge graph)
- Ollama (local embeddings)
- Go 1.26+ (rin-memory-go, rin-proxy)
- Docker (PostgreSQL hosting)

## Commands

```bash
make install         # Full install: venv + MCP + model + Docker PG + Go + launchd + PATH
make rin             # Launch RIN
make test            # Full pipeline test in Docker (8 steps)
make install-db      # Start PostgreSQL via Docker (PG17 + pgvector + AGE)
make memory-go       # Build Go memory server
make proxy           # Build Go proxy
make harvest         # Run session collection manually
make review          # Run session review manually
make dream           # Run memory consolidation manually
make team            # Team mode: Claude lead + provider teammates
make cc              # Exit team mode
make sync-harness TARGET=<path>  # Deploy harness to another project
```

## Testing

```bash
.venv/bin/pytest
```

Linter: `ruff check src/`, Formatter: `ruff format src/`

## Harness (Agents / Skills / Commands)

RIN's harness infrastructure. project-rin is the canonical source, deployed to other projects via `sync-harness.sh`.

### Agents (`.claude/agents/`)
| File | Role |
|------|------|
| `code-edit.md` | Single-task code modification (scope tier based) |
| `code-review.md` | Weighted code review |
| `validate.md` | Dual-mode validation (artifact consistency + AC implementation) |

### Skills (`.claude/skills/`)
| Skill | Role | Config |
|-------|------|--------|
| `auto-impl` | Phase orchestrator | `config.yaml` |
| `auto-research` | Autonomous experiment loop (karpathy/autoresearch pattern) | `config.yaml` |
| `plan-feature` | Interactive design document generation | `config.yaml` |
| `qa-gate` | Review + validation parallel gate | — |
| `troubleshoot` | 5-stage diagnostic pipeline | — |
| `smart-commit` | Auto-grouped commits | `config.yaml` |
| `gc` | Entropy auto-detection/fix + memory cleanup | `config.yaml` |
| `create-pr` | Auto PR creation | `config.project.yaml` |

### Commands (`.claude/commands/`)
| Command | Delegates to |
|---------|-------------|
| `/commit` | smart-commit skill |
| `/pr` | create-pr skill |
| `/code-review` | code-review agent |

### Sync
```bash
make sync-harness TARGET=~/workspace/other-project  # Deploy harness
./scripts/sync-harness.sh --dry-run ~/workspace/other-project  # Dry run
```

Rule: Only `skill.md` is synced. `config.yaml`/`config.project.yaml` are kept per-project.

## Notes

- **Schema changes require migration**: Changing PostgreSQL schema or pgvector dimensions will break compatibility with existing data. Write migration scripts first.
- **Data path**: Runtime data is stored in PostgreSQL `rin_memory` database. `memory/sessions/` in the project is a temporary staging area before collection.
- **rin-context.md defines agent behavior**: Principles and decision boundaries. Injected as system prompt every session.
- **rin-proxy is a Go binary**: Build with `go build` in `src/rin_proxy/`. Stock models (opus/sonnet/haiku) pass through to Anthropic; only custom aliases (gemini-pro/gemini-flash) convert to Gemini.
