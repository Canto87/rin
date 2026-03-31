#!/bin/bash
# Install pipeline verification test for project-rin-oss
# Tests that install steps produce correct artifacts.
# Runs in Docker with isolated HOME to avoid touching host config.
set -o pipefail

PASS=0
FAIL=0
TOTAL=6

# Isolated home — install steps write to this instead of real ~
export HOME="/tmp/test-home"
mkdir -p "$HOME"

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
echo "  RIN Install Pipeline Test"
echo "════════════════════════════════════════════════"
echo ""

export RIN_HOME=/app

# ── Step 1: Python venv creation ─────────────────────
echo "=== Step 1: Python venv ==="
python3 -m venv .venv 2>&1
if [ -x .venv/bin/python3 ]; then
    report 1 "Python venv created" "PASS"
else
    report 1 "Python venv creation failed" "FAIL"
fi

# ── Step 2: Go binary builds ─────────────────────────
echo ""
echo "=== Step 2: Go binary builds ==="
cd src/rin_memory_go && go build -o rin-memory-go . 2>&1 && cd /app
cd src/rin_proxy && go build -o rin-proxy . 2>&1 && cd /app
if [ -x src/rin_memory_go/rin-memory-go ] && [ -x src/rin_proxy/rin-proxy ]; then
    report 2 "Go binaries built (rin-memory-go + rin-proxy)" "PASS"
else
    report 2 "Go binary build failed" "FAIL"
fi

# ── Step 3: sync-mcp (MCP config → ~/.claude.json) ──
echo ""
echo "=== Step 3: sync-mcp ==="
RIN_HOME=/app python3 scripts/sync-mcp.py 2>&1

if [ -f "$HOME/.claude.json" ]; then
    # Check that rin-memory server is registered
    HAS_RIN_MEMORY=$(python3 -c "
import json
data = json.load(open('$HOME/.claude.json'))
servers = data.get('mcpServers', {})
print('yes' if 'rin-memory' in servers else 'no')
" 2>/dev/null)
    if [ "$HAS_RIN_MEMORY" = "yes" ]; then
        report 3 "sync-mcp (rin-memory registered in ~/.claude.json)" "PASS"
    else
        report 3 "sync-mcp (rin-memory not found in config)" "FAIL"
    fi
else
    report 3 "sync-mcp (~/.claude.json not created)" "FAIL"
fi

# ── Step 4: install-statusline ───────────────────────
echo ""
echo "=== Step 4: install-statusline ==="
mkdir -p "$HOME/.claude"
cp scripts/statusline.sh "$HOME/.claude/statusline-command.sh"
chmod +x "$HOME/.claude/statusline-command.sh"

# Write settings.json
python3 -c "
import json, os
p = os.path.expanduser('~/.claude/settings.json')
d = json.load(open(p)) if os.path.exists(p) else {}
d['statusLine'] = {'type': 'command', 'command': os.path.expanduser('~/.claude/statusline-command.sh')}
json.dump(d, open(p, 'w'), indent=2)" 2>/dev/null

if [ -x "$HOME/.claude/statusline-command.sh" ] && [ -f "$HOME/.claude/settings.json" ]; then
    HAS_STATUSLINE=$(python3 -c "
import json
d = json.load(open('$HOME/.claude/settings.json'))
print('yes' if d.get('statusLine', {}).get('type') == 'command' else 'no')
" 2>/dev/null)
    if [ "$HAS_STATUSLINE" = "yes" ]; then
        report 4 "install-statusline (script + settings.json)" "PASS"
    else
        report 4 "install-statusline (settings.json misconfigured)" "FAIL"
    fi
else
    report 4 "install-statusline (files missing)" "FAIL"
fi

# ── Step 5: install-harness-global ───────────────────
echo ""
echo "=== Step 5: install-harness-global ==="
./scripts/sync-harness.sh --global 2>&1

AGENT_COUNT=$(ls "$HOME/.claude/agents/"*.md 2>/dev/null | wc -l | tr -d ' ')
SKILL_COUNT=$(find "$HOME/.claude/skills/" -name "skill.md" 2>/dev/null | wc -l | tr -d ' ')
CMD_COUNT=$(ls "$HOME/.claude/commands/"*.md 2>/dev/null | wc -l | tr -d ' ')

echo "  Agents: $AGENT_COUNT, Skills: $SKILL_COUNT, Commands: $CMD_COUNT"

if [ "$AGENT_COUNT" -ge 2 ] && [ "$SKILL_COUNT" -ge 5 ] && [ "$CMD_COUNT" -ge 2 ]; then
    report 5 "install-harness-global (${AGENT_COUNT}A/${SKILL_COUNT}S/${CMD_COUNT}C)" "PASS"
else
    report 5 "install-harness-global (insufficient: ${AGENT_COUNT}A/${SKILL_COUNT}S/${CMD_COUNT}C)" "FAIL"
fi

# ── Step 6: shell-setup (PATH in rc file) ────────────
echo ""
echo "=== Step 6: shell-setup ==="
export SHELL=/bin/bash
touch "$HOME/.bashrc"
# Run shell-setup directly since make inherits HOME
make -C /app HOME="$HOME" shell-setup 2>&1

if grep -q "/app/scripts" "$HOME/.bashrc" 2>/dev/null; then
    report 6 "shell-setup (PATH added to .bashrc)" "PASS"
else
    echo "  .bashrc contents: $(cat "$HOME/.bashrc" 2>/dev/null)"
    report 6 "shell-setup (PATH not found in rc file)" "FAIL"
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
