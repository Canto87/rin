.PHONY: setup install check rin harvest review dream sync-mcp pull-model \
	install-db uninstall-db install-cron uninstall-cron \
	install-ollama uninstall-ollama \
	proxy install-proxy uninstall-proxy cc help \
	shell-setup memory-go sync-harness test test-install install-statusline

LAUNCH_AGENTS_DIR := $(HOME)/Library/LaunchAgents
RIN_HOME := $(shell pwd)
OLLAMA_MODEL := mxbai-embed-large

# Read values from ~/.secrets (existing env vars take precedence)
_secret = $(shell grep -E '^export $(1)=' $(HOME)/.secrets 2>/dev/null | cut -d= -f2-)
GEMINI_API_KEY ?= $(call _secret,GEMINI_API_KEY)
GLM_API_KEY    ?= $(call _secret,GLM_API_KEY)

# Plist template substitution (shared base)
PLIST_SED = sed 's|__RIN_HOME__|$(RIN_HOME)|g; s|__HOME__|$(HOME)|g'

# ── Check ───────────────────────────────────────────────

check:  ## Check prerequisites (Python 3.11+, Go 1.26+, Docker, Ollama, Claude CLI)
	@printf "Checking prerequisites...\n"
	@command -v python3 >/dev/null 2>&1 || { printf "  ✗ python3 not found\n"; exit 1; }
	@python3 -c "import sys; v=sys.version_info; assert v >= (3,11), f'need 3.11+, got {v.major}.{v.minor}'" 2>&1 || exit 1
	@printf "  ✓ python3 %s\n" "$$(python3 -c 'import sys; print(f"{sys.version_info.major}.{sys.version_info.minor}.{sys.version_info.micro}")')"
	@command -v go >/dev/null 2>&1 || { printf "  ✗ go not found\n"; exit 1; }
	@printf "  ✓ go %s\n" "$$(go version | awk '{print $$3}' | sed 's/go//')"
	@command -v docker >/dev/null 2>&1 || { printf "  ✗ docker not found\n"; exit 1; }
	@docker info >/dev/null 2>&1 || { printf "  ✗ docker daemon not running\n"; exit 1; }
	@printf "  ✓ docker\n"
	@if command -v ollama >/dev/null 2>&1; then \
		printf "  ✓ ollama\n"; \
	else \
		printf "  ✗ ollama not found (brew install ollama)\n"; exit 1; \
	fi
	@if command -v claude >/dev/null 2>&1; then \
		printf "  ✓ claude\n"; \
	else \
		printf "  ⚠ claude CLI not found (npm i -g @anthropic-ai/claude-code)\n"; \
	fi

# ── Setup ────────────────────────────────────────────────

setup: check  ## Create Python venv (for session scripts)
	python3 -m venv .venv

serve-ollama:  ## Start Ollama server (background)
	@command -v ollama >/dev/null 2>&1 || { echo "ollama not found"; exit 1; }
	@if curl -sf http://localhost:11434/api/tags >/dev/null 2>&1; then \
		echo "Ollama already running"; \
	else \
		nohup ollama serve > /dev/null 2>&1 & \
		echo "Ollama server started (pid $$!)"; \
		sleep 2; \
	fi

pull-model: serve-ollama  ## Pull Ollama embedding model
	@ollama pull $(OLLAMA_MODEL)

sync-mcp:  ## Sync MCP servers to ~/.claude.json
	@RIN_HOME="$(RIN_HOME)" python3 scripts/sync-mcp.py

shell-setup:  ## Add RIN scripts to PATH in shell rc file
	@RC_FILE=""; \
	case "$$(basename $$SHELL)" in \
		zsh)  RC_FILE="$(HOME)/.zshrc" ;; \
		bash) RC_FILE="$(HOME)/.bashrc" ;; \
		fish) RC_FILE="$(HOME)/.config/fish/config.fish" ;; \
		*)    RC_FILE="$(HOME)/.profile" ;; \
	esac; \
	if grep -q '$(RIN_HOME)/scripts' "$$RC_FILE" 2>/dev/null; then \
		echo "PATH already configured in $$RC_FILE"; \
	else \
		if [ "$$(basename $$SHELL)" = "fish" ]; then \
			mkdir -p "$$(dirname $$RC_FILE)"; \
			echo '' >> "$$RC_FILE"; \
			echo '# RIN' >> "$$RC_FILE"; \
			echo 'set -gx PATH $(RIN_HOME)/scripts $$PATH' >> "$$RC_FILE"; \
		else \
			echo '' >> "$$RC_FILE"; \
			echo '# RIN' >> "$$RC_FILE"; \
			echo 'export PATH="$(RIN_HOME)/scripts:$$PATH"' >> "$$RC_FILE"; \
		fi; \
		echo "Added to $$RC_FILE — restart your shell or: source $$RC_FILE"; \
	fi

install-statusline:  ## Install Claude Code statusline (usage + memory count)
	@cp scripts/statusline.sh $(HOME)/.claude/statusline-command.sh
	@chmod +x $(HOME)/.claude/statusline-command.sh
	@python3 -c "\
	import json, os; \
	p = os.path.expanduser('~/.claude/settings.json'); \
	d = json.load(open(p)) if os.path.exists(p) else {}; \
	d['statusLine'] = {'type': 'command', 'command': os.path.expanduser('~/.claude/statusline-command.sh')}; \
	json.dump(d, open(p, 'w'), indent=2)" 2>/dev/null
	@echo "Installed: statusline (usage + memory count)"

install-harness-global:  ## Deploy harness (agents/skills/commands) to ~/.claude/
	@./scripts/sync-harness.sh --global

install: setup install-db memory-go pull-model sync-mcp install-statusline install-harness-global install-cron shell-setup  ## Full install
	@mkdir -p $(HOME)/.rin
	@echo ""
	@echo "══════════════════════════════════════════════"
	@echo "  RIN install complete. Run: rin"
	@echo "══════════════════════════════════════════════"

# ── Run ──────────────────────────────────────────────────

rin:  ## Launch RIN
	@./scripts/rin

# ── Session Pipeline ─────────────────────────────────────

harvest:  ## Run session harvest manually
	@.venv/bin/python scripts/session-harvest.py

review:  ## Run session review manually
	@bash scripts/session-review.sh

dream:  ## Run memory dream (consolidation) manually
	@bash scripts/memory-dream.sh

# ── launchd (core) ───────────────────────────────────────

install-cron:  ## Install session harvest/review/dream launchd agents (macOS only)
	@if [ "$$(uname)" != "Darwin" ]; then echo "Skipping launchd (not macOS)"; exit 0; fi
	@mkdir -p $(LAUNCH_AGENTS_DIR)
	@$(PLIST_SED) launchd/com.rin.session-harvest.plist > $(LAUNCH_AGENTS_DIR)/com.rin.session-harvest.plist
	@$(PLIST_SED) launchd/com.rin.session-review.plist > $(LAUNCH_AGENTS_DIR)/com.rin.session-review.plist
	@$(PLIST_SED) launchd/com.rin.memory-dream.plist > $(LAUNCH_AGENTS_DIR)/com.rin.memory-dream.plist
	@launchctl load $(LAUNCH_AGENTS_DIR)/com.rin.session-harvest.plist 2>/dev/null || true
	@launchctl load $(LAUNCH_AGENTS_DIR)/com.rin.session-review.plist 2>/dev/null || true
	@launchctl load $(LAUNCH_AGENTS_DIR)/com.rin.memory-dream.plist 2>/dev/null || true
	@echo "Installed: session-harvest (10min), session-review (1h), memory-dream (24h)"

uninstall-cron:  ## Remove session harvest/review/dream launchd agents
	@launchctl unload $(LAUNCH_AGENTS_DIR)/com.rin.session-harvest.plist 2>/dev/null || true
	@launchctl unload $(LAUNCH_AGENTS_DIR)/com.rin.session-review.plist 2>/dev/null || true
	@launchctl unload $(LAUNCH_AGENTS_DIR)/com.rin.memory-dream.plist 2>/dev/null || true
	@rm -f $(LAUNCH_AGENTS_DIR)/com.rin.session-harvest.plist
	@rm -f $(LAUNCH_AGENTS_DIR)/com.rin.session-review.plist
	@rm -f $(LAUNCH_AGENTS_DIR)/com.rin.memory-dream.plist
	@echo "Uninstalled session harvest/review/dream agents"

# ── launchd (optional) ──────────────────────────────────

install-ollama:  ## Install Ollama serve launchd agent
	@mkdir -p $(LAUNCH_AGENTS_DIR)
	@launchctl bootout gui/$$(id -u) $(LAUNCH_AGENTS_DIR)/com.rin.ollama.plist 2>/dev/null || true
	@$(PLIST_SED) launchd/com.rin.ollama.plist > $(LAUNCH_AGENTS_DIR)/com.rin.ollama.plist
	@launchctl bootstrap gui/$$(id -u) $(LAUNCH_AGENTS_DIR)/com.rin.ollama.plist
	@echo "Installed: Ollama serve (KeepAlive)"

uninstall-ollama:  ## Remove Ollama serve launchd agent
	@launchctl bootout gui/$$(id -u) $(LAUNCH_AGENTS_DIR)/com.rin.ollama.plist 2>/dev/null || true
	@rm -f $(LAUNCH_AGENTS_DIR)/com.rin.ollama.plist
	@echo "Uninstalled Ollama serve agent"

# ── Database ────────────────────────────────────────────

install-db:  ## Start PostgreSQL (Docker: PG17 + pgvector + AGE)
	@docker compose up -d postgres
	@echo "Waiting for PostgreSQL..."
	@until docker compose exec -T postgres pg_isready -U postgres -q 2>/dev/null; do sleep 1; done
	@until docker compose exec -T postgres psql -U postgres -d rin_memory -c 'SELECT 1' -q 2>/dev/null; do sleep 1; done
	@mkdir -p $(HOME)/.rin
	@echo '{"dsn":"postgres://postgres:postgres@localhost:$(or $(RIN_PG_PORT),5434)/rin_memory"}' > $(HOME)/.rin/memory-config.json
	@echo "PostgreSQL ready: postgres://localhost:$(or $(RIN_PG_PORT),5434)/rin_memory"

uninstall-db:  ## Stop and remove PostgreSQL container + data
	@docker compose down postgres -v
	@echo "PostgreSQL removed"

# ── Memory (Go) ─────────────────────────────────────────

memory-go:  ## Build rin-memory-go (PostgreSQL + pgvector + AGE)
	@cd src/rin_memory_go && go build -o rin-memory-go .
	@echo "Built: src/rin_memory_go/rin-memory-go"

# ── Proxy ────────────────────────────────────────────────

proxy:  ## Build rin-proxy (Go)
	@cd src/rin_proxy && go build -o rin-proxy .
	@echo "Built: src/rin_proxy/rin-proxy"

install-proxy: proxy  ## Install rin-proxy launchd agent (GEMINI_API_KEY=... GLM_API_KEY=...)
	@test -n "$(GEMINI_API_KEY)" || { echo "Set GEMINI_API_KEY=<key>"; exit 1; }
	@mkdir -p $(LAUNCH_AGENTS_DIR)
	@mkdir -p $(HOME)/.rin
	@launchctl bootout gui/$$(id -u) $(LAUNCH_AGENTS_DIR)/com.rin.proxy.plist 2>/dev/null || true
	@$(PLIST_SED) launchd/com.rin.proxy.plist | \
		sed 's|__GEMINI_API_KEY__|$(GEMINI_API_KEY)|g; s|__GLM_API_KEY__|$(GLM_API_KEY)|g' \
		> $(LAUNCH_AGENTS_DIR)/com.rin.proxy.plist
	@launchctl bootstrap gui/$$(id -u) $(LAUNCH_AGENTS_DIR)/com.rin.proxy.plist
	@echo "Installed: rin-proxy (http://127.0.0.1:3456)"

team:  ## Team mode: Claude (lead) + provider teammates (gemini|glm|all)
	@./scripts/rin-team $(filter-out $@,$(MAKECMDGOALS))

cc:  ## Exit team mode: back to Claude-only
	@./scripts/rin-cc

uninstall-proxy:  ## Remove rin-proxy launchd agent
	@launchctl bootout gui/$$(id -u) $(LAUNCH_AGENTS_DIR)/com.rin.proxy.plist 2>/dev/null || true
	@rm -f $(LAUNCH_AGENTS_DIR)/com.rin.proxy.plist
	@echo "Uninstalled rin-proxy agent"

# ── Harness ──────────────────────────────────────────────

sync-harness:  ## Sync harness to target project (TARGET=<path>) or globally (TARGET=global)
	@if [ "$(TARGET)" = "global" ]; then \
		./scripts/sync-harness.sh --global; \
	elif [ -n "$(TARGET)" ]; then \
		./scripts/sync-harness.sh "$(TARGET)"; \
	else \
		echo "Usage: make sync-harness TARGET=<project-path>"; \
		echo "       make sync-harness TARGET=global"; \
		exit 1; \
	fi

# ── Test ─────────────────────────────────────────────────

test:  ## Run full pipeline test in Docker (build + unit tests + MCP server)
	@docker compose up --build --abort-on-container-exit --exit-code-from app

test-install:  ## Run install pipeline test in Docker (sync-mcp, statusline, harness, shell-setup)
	@docker compose up --build --abort-on-container-exit --exit-code-from app-install

# ── Help ─────────────────────────────────────────────────

help:  ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-20s %s\n", $$1, $$2}'
