#!/bin/bash
# rin-proxy-test.sh — Run Claude Code via proxy (for Agent Teams testing)
#
# Usage:
#   ./scripts/rin-proxy-test.sh          # Start proxy + launch Claude Code
#   ./scripts/rin-proxy-test.sh --proxy  # Start proxy only (in a separate terminal)

set -euo pipefail

RIN_HOME="$(cd "$(dirname "$0")/.." && pwd)"
PROXY_BIN="${RIN_HOME}/src/rin_proxy/rin-proxy"
PROXY_PORT=3456

# Check proxy build
if [ ! -f "$PROXY_BIN" ]; then
    echo "Building rin-proxy..."
    (cd "${RIN_HOME}/src/rin_proxy" && go build -o rin-proxy .)
fi

# Check if proxy is already running
if curl -s "http://127.0.0.1:${PROXY_PORT}/health" >/dev/null 2>&1; then
    echo "rin-proxy already running on :${PROXY_PORT}"
else
    echo "Starting rin-proxy on :${PROXY_PORT}..."
    "$PROXY_BIN" &
    PROXY_PID=$!
    sleep 1

    if ! curl -s "http://127.0.0.1:${PROXY_PORT}/health" >/dev/null 2>&1; then
        echo "Failed to start rin-proxy"
        exit 1
    fi
    echo "rin-proxy started (PID: ${PROXY_PID})"

    # Proxy-only mode
    if [ "${1:-}" = "--proxy" ]; then
        echo "Proxy running. Press Ctrl+C to stop."
        wait "$PROXY_PID"
        exit 0
    fi

    # Stop proxy when Claude Code exits
    trap "kill $PROXY_PID 2>/dev/null" EXIT
fi

echo ""
echo "═══════════════════════════════════════════"
echo "  Claude Code via rin-proxy"
echo ""
echo "  Model mapping:"
echo "    opus/sonnet → Claude (passthrough)"
echo "    haiku       → Gemini 2.5 Flash"
echo "    /model gemini-flash  (direct selection also works)"
echo "    /model gemini-pro    (Gemini 2.5 Pro)"
echo ""
echo "  Agent Teams:"
echo "    'Create a team. Use haiku for teammates.'"
echo "    → Teammates run on Gemini Flash"
echo "═══════════════════════════════════════════"
echo ""

# Run Claude Code via the proxy
# - ANTHROPIC_BASE_URL: all API requests go through the proxy
# - ANTHROPIC_DEFAULT_HAIKU_MODEL: maps haiku alias to gemini-flash
#   (proxy routes gemini-flash to the Gemini API)
ANTHROPIC_BASE_URL="http://127.0.0.1:${PROXY_PORT}" \
ANTHROPIC_DEFAULT_HAIKU_MODEL="gemini-flash" \
    claude "$@"
