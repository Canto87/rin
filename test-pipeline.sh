#!/bin/bash
# Full installation pipeline test for project-rin-oss
set -o pipefail

PASS=0
FAIL=0
TOTAL=8

report() {
    local step=$1 name=$2 result=$3
    if [ "$result" = "PASS" ]; then
        printf "\033[32m[PASS]\033[0m Step %d: %s\n" "$step" "$name"
        PASS=$((PASS + 1))
    else
        printf "\033[31m[FAIL]\033[0m Step %d: %s\n" "$step" "$name"
        FAIL=$((FAIL + 1))
    fi
}

echo ""
echo "════════════════════════════════════════════════"
echo "  RIN Installation Pipeline Test"
echo "════════════════════════════════════════════════"
echo ""

# ── Step 1: Python venv + pip install ─────────────────
echo "=== Step 1: Python setup (venv + pip install) ==="
python3 -m venv .venv && .venv/bin/pip install -e ".[dev]" -q 2>&1 | tail -3
if [ $? -eq 0 ]; then report 1 "Python setup (venv + pip install)" "PASS"
else report 1 "Python setup (venv + pip install)" "FAIL"; fi

# ── Step 2: Go memory server build ───────────────────
echo ""
echo "=== Step 2: Go memory server build ==="
cd src/rin_memory_go && go build -o rin-memory-go . 2>&1 && cd /app
if [ -x src/rin_memory_go/rin-memory-go ]; then
    report 2 "Go memory server build" "PASS"
else
    report 2 "Go memory server build" "FAIL"
fi

# ── Step 3: Go proxy build ───────────────────────────
echo ""
echo "=== Step 3: Go proxy build ==="
cd src/rin_proxy && go build -o rin-proxy . 2>&1 && cd /app
if [ -x src/rin_proxy/rin-proxy ]; then
    report 3 "Go proxy build" "PASS"
else
    report 3 "Go proxy build" "FAIL"
fi

# ── Step 4: PostgreSQL schema initialization ─────────
echo ""
echo "=== Step 4: PostgreSQL schema init ==="
# Write config so rin-memory-go can find the DB
mkdir -p ~/.rin
echo "{\"dsn\":\"$RIN_MEMORY_DSN\",\"ollama_url\":\"$OLLAMA_HOST\"}" > ~/.rin/memory-config.json

# Apply schema directly via psql
psql "$RIN_MEMORY_DSN" -f src/rin_memory_go/schema.sql 2>&1 || true

TABLE_COUNT=$(psql "$RIN_MEMORY_DSN" -qtAX -c \
    "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema='public' AND table_name IN ('documents','document_vectors','relations');" 2>/dev/null)
if [ "$TABLE_COUNT" = "3" ]; then
    report 4 "PostgreSQL schema init (3 tables created)" "PASS"
else
    report 4 "PostgreSQL schema init (tables=$TABLE_COUNT)" "FAIL"
fi

# ── Step 5: Ollama model pull + embedding test ───────
echo ""
echo "=== Step 5: Ollama embedding test ==="
# Pull model (this takes a while on first run)
echo "  Pulling mxbai-embed-large..."
curl -sf "${OLLAMA_HOST}/api/pull" -d '{"name":"mxbai-embed-large"}' | tail -1

EMBED_DIM=$(curl -sf "${OLLAMA_HOST}/api/embed" \
    -d '{"model":"mxbai-embed-large","input":"test embedding"}' | python3 -c "
import sys, json
data = json.load(sys.stdin)
print(len(data.get('embeddings', [[]])[0]))
" 2>/dev/null)

if [ "$EMBED_DIM" = "1024" ]; then
    report 5 "Ollama embedding test (dim=1024)" "PASS"
else
    report 5 "Ollama embedding test (dim=$EMBED_DIM)" "FAIL"
fi

# ── Step 6: pytest ───────────────────────────────────
echo ""
echo "=== Step 6: pytest ==="
.venv/bin/pytest tests/ -v 2>&1
if [ $? -eq 0 ]; then report 6 "pytest (all tests)" "PASS"
else report 6 "pytest (all tests)" "FAIL"; fi

# ── Step 7: scripts/rin banner check ─────────────────
echo ""
echo "=== Step 7: scripts/rin banner check ==="
OUTPUT=$(timeout 5 bash -c 'export RIN_HOME=/app RIN_PROJECT=test; bash scripts/rin --resume fake 2>&1' || true)
if echo "$OUTPUT" | grep -q "凛"; then
    report 7 "scripts/rin banner displayed" "PASS"
else
    report 7 "scripts/rin banner not displayed" "FAIL"
    echo "  Output: $(echo "$OUTPUT" | head -5)"
fi

# ── Step 8: rin-memory-go MCP tool listing ───────────
echo ""
echo "=== Step 8: rin-memory-go MCP server ==="
echo "  DSN: $RIN_MEMORY_DSN"
INIT='{"jsonrpc":"2.0","method":"initialize","id":1,"params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}'
NOTIF='{"jsonrpc":"2.0","method":"notifications/initialized"}'
TOOLS='{"jsonrpc":"2.0","method":"tools/list","id":2,"params":{}}'

TOOL_COUNT=$( { echo "$INIT"; sleep 1; echo "$NOTIF"; sleep 0.5; echo "$TOOLS"; sleep 3; } \
    | RIN_MEMORY_DSN="$RIN_MEMORY_DSN" timeout 15 src/rin_memory_go/rin-memory-go 2>/tmp/mcp-stderr \
    | python3 -c "
import sys, json
for line in sys.stdin:
    line = line.strip()
    if not line:
        continue
    try:
        msg = json.loads(line)
        if msg.get('id') == 2:
            tools = msg.get('result', {}).get('tools', [])
            print(len(tools))
            sys.exit(0)
    except:
        pass
" 2>/dev/null)

# Debug output on failure
if [ -z "$TOOL_COUNT" ] || [ "$TOOL_COUNT" -lt 5 ] 2>/dev/null; then
    echo "  MCP stderr: $(head -3 /tmp/mcp-stderr 2>/dev/null)"
fi

if [ -n "$TOOL_COUNT" ] && [ "$TOOL_COUNT" -ge 5 ]; then
    report 8 "rin-memory-go MCP server ($TOOL_COUNT tools registered)" "PASS"
else
    report 8 "rin-memory-go MCP server (tools=$TOOL_COUNT)" "FAIL"
fi

# ── Summary ──────────────────────────────────────────
echo ""
echo "════════════════════════════════════════════════"
if [ "$FAIL" -eq 0 ]; then
    printf "  \033[32mResults: $PASS/$TOTAL passed\033[0m\n"
else
    printf "  \033[31mResults: $PASS/$TOTAL passed, $FAIL/$TOTAL failed\033[0m\n"
fi
echo "════════════════════════════════════════════════"
echo ""

[ "$FAIL" -eq 0 ] && exit 0 || exit 1
