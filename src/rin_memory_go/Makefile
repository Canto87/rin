.PHONY: check build setup-db migrate reembed install test clean help

BIN       := rin-memory-go
PG_DB     := rin_memory
PSQL      := /opt/homebrew/opt/postgresql@17/bin/psql
PG_BIN    := /opt/homebrew/opt/postgresql@17/bin
OLLAMA    := ollama
MODEL     := mxbai-embed-large
RIN_HOME  := $(shell cd ../.. && pwd)

# ── Check ───────────────────────────────────────────────

check:  ## Check prerequisites
	@printf "Checking prerequisites...\n"
	@# Go
	@command -v go >/dev/null 2>&1 || { printf "  ✗ go not found\n"; exit 1; }
	@printf "  ✓ go %s\n" "$$(go version | awk '{print $$3}' | sed 's/go//')"
	@# PostgreSQL
	@test -x "$(PSQL)" || { printf "  ✗ psql not found (brew install postgresql@17)\n"; exit 1; }
	@printf "  ✓ psql (%s)\n" "$$($(PSQL) --version | awk '{print $$NF}')"
	@# PostgreSQL running
	@$(PG_BIN)/pg_isready -q 2>/dev/null || { printf "  ✗ PostgreSQL not running (brew services start postgresql@17)\n"; exit 1; }
	@printf "  ✓ PostgreSQL running\n"
	@# Extensions
	@test -f /opt/homebrew/share/postgresql@17/extension/vector.control || { printf "  ✗ pgvector not found (brew install pgvector)\n"; exit 1; }
	@printf "  ✓ pgvector\n"
	@test -f /opt/homebrew/share/postgresql@17/extension/age.control || { printf "  ✗ AGE not found (build from source)\n"; exit 1; }
	@printf "  ✓ AGE\n"
	@# pg_trgm is bundled with PostgreSQL — always available
	@printf "  ✓ pg_trgm (bundled)\n"
	@# Ollama
	@if command -v $(OLLAMA) >/dev/null 2>&1; then \
		printf "  ✓ ollama\n"; \
	else \
		printf "  ⚠ ollama not found — embedding will fail (brew install ollama)\n"; \
	fi
	@printf "All checks passed.\n"

# ── Build ───────────────────────────────────────────────

build:  ## Build the binary
	@go build -o $(BIN) .
	@printf "Built: %s\n" "$(BIN)"

# ── Database ────────────────────────────────────────────

setup-db:  ## Create database + run schema migration
	@# Create DB if not exists
	@$(PSQL) -lqt 2>/dev/null | cut -d\| -f1 | grep -qw $(PG_DB) || \
		$(PG_BIN)/createdb $(PG_DB) && printf "Database: $(PG_DB)\n"
	@# Schema migration runs on first connection via embedded schema.sql
	@# Just verify by starting the server briefly
	@test -x ./$(BIN) || $(MAKE) build
	@printf '{"jsonrpc":"2.0","method":"initialize","id":1,"params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"setup","version":"1"}}}' \
		| timeout 3 ./$(BIN) >/dev/null 2>&1 || true
	@# Verify tables
	@$(PSQL) -qtAX $(PG_DB) -c "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema='public' AND table_name IN ('documents','document_vectors','relations');" \
		| grep -q 3 && printf "Schema: OK (3 tables)\n" || { printf "Schema: FAIL\n"; exit 1; }
	@# Verify extensions
	@$(PSQL) -qtAX $(PG_DB) -c "SELECT extname FROM pg_extension WHERE extname IN ('vector','pg_trgm','age') ORDER BY extname;" \
		| tr '\n' ',' | grep -q "age,pg_trgm,vector" \
		&& printf "Extensions: OK (age, pg_trgm, vector)\n" \
		|| printf "Extensions: some missing (run server once to auto-create)\n"

# ── Migration ───────────────────────────────────────────

migrate:  ## Migrate data from SQLite → PostgreSQL
	@test -f $(HOME)/.rin/memory.db || { printf "No SQLite DB at ~/.rin/memory.db\n"; exit 1; }
	@command -v python3 >/dev/null 2>&1 || { printf "python3 required for migration\n"; exit 1; }
	@pip3 show psycopg2-binary >/dev/null 2>&1 || pip3 install psycopg2-binary
	@python3 $(RIN_HOME)/scripts/migrate-to-pg.py check
	@printf "\nRun migration? [y/N] " && read ans && [ "$$ans" = "y" ] && \
		python3 $(RIN_HOME)/scripts/migrate-to-pg.py migrate || printf "Skipped.\n"

# ── Embeddings ──────────────────────────────────────────

reembed:  ## Re-generate embeddings for documents without vectors
	@test -x ./$(BIN) || $(MAKE) build
	@./$(BIN) reembed

# ── Install ─────────────────────────────────────────────

install: check build setup-db  ## Full install: check + build + setup DB + sync MCP
	@# Sync MCP config
	@RIN_HOME="$(RIN_HOME)" python3 $(RIN_HOME)/scripts/sync-mcp.py 2>/dev/null || true
	@printf "\n══════════════════════════════════════════════\n"
	@printf "  rin-memory-go install complete.\n"
	@printf "  Binary: %s/%s\n" "$$(pwd)" "$(BIN)"
	@printf "  DB:     $(PG_DB)\n"
	@printf "══════════════════════════════════════════════\n"

# ── Test ────────────────────────────────────────────────

test: build  ## Run E2E smoke test
	@printf "Testing...\n"
	@# Tool count
	@python3 -c "\
	import subprocess, json; \
	p = subprocess.Popen(['./$(BIN)'], stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, \
	    env={'PATH':'/usr/bin:/bin:/usr/local/bin','HOME':'$(HOME)','RIN_PROJECT':'test'}); \
	send = lambda m: (p.stdin.write(json.dumps(m).encode()+b'\n'), p.stdin.flush()); \
	recv = lambda: next((json.loads(l) for l in iter(p.stdout.readline, b'') if b'\"id\"' in l), None); \
	send({'jsonrpc':'2.0','method':'initialize','id':1,'params':{'protocolVersion':'2025-03-26','capabilities':{},'clientInfo':{'name':'t','version':'1'}}}); \
	recv(); \
	send({'jsonrpc':'2.0','method':'notifications/initialized'}); \
	send({'jsonrpc':'2.0','method':'tools/list','id':2,'params':{}}); \
	r = recv(); \
	n = len(r.get('result',{}).get('tools',[])); \
	p.terminate(); \
	print(f'  tools/list: {n} tools {\"✓\" if n==17 else \"✗\"}'); \
	exit(0 if n==17 else 1)"
	@# Recall subcommand
	@RIN_PROJECT=test ./$(BIN) recall >/dev/null 2>&1 && printf "  recall:     ✓\n" || printf "  recall:     ✗\n"
	@# Insert-log subcommand
	@echo '{"task_description":"test","model":"test","level":"L1","mode":"solo","agent_count":1,"success":true,"subagent_type":"test"}' \
		| RIN_PROJECT=test ./$(BIN) insert-log 2>/dev/null && printf "  insert-log: ✓\n" || printf "  insert-log: ✗\n"
	@printf "Done.\n"

# ── Clean ───────────────────────────────────────────────

clean:  ## Remove binary
	@rm -f $(BIN)
	@printf "Cleaned.\n"

# ── Help ────────────────────────────────────────────────

help:  ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-14s %s\n", $$1, $$2}'
