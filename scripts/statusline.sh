#!/bin/bash
# Claude Code statusline for RIN
# Shows: Model | Ctx% | Session usage | Week usage | Memory doc count
# Usage API is cached for 60s to avoid rate limits.
# Memory doc count is cached for 5min.

RED='\033[31m'
YELLOW='\033[33m'
GREEN='\033[32m'
CYAN='\033[36m'
DIM='\033[2m'
RESET='\033[0m'

input=$(cat)

# ── Usage API (cached 60s) ────────────────────────────

CACHE_FILE="$HOME/.claude/usage-cache.json"
CACHE_MAX_AGE=60

fetch_usage() {
    local token
    # Try macOS Keychain first, then fallback to credentials file
    token=$(security find-generic-password -s "Claude Code-credentials" -w 2>/dev/null | jq -r '.claudeAiOauth.accessToken // empty' 2>/dev/null)
    if [ -z "$token" ]; then
        token=$(jq -r '.claudeAiOauth.accessToken // empty' "$HOME/.claude/.credentials.json" 2>/dev/null)
    fi
    if [ -n "$token" ]; then
        local config_file=$(mktemp)
        chmod 600 "$config_file"
        cat > "$config_file" << EOF
-s
-H "Authorization: Bearer $token"
-H "User-Agent: claude-code/2.1.87"
-H "anthropic-beta: oauth-2025-04-20"
EOF
        curl -K "$config_file" "https://api.anthropic.com/api/oauth/usage" 2>/dev/null
        rm -f "$config_file"
    fi
}

usage_json=""
if [ -f "$CACHE_FILE" ]; then
    if stat -f %m "$CACHE_FILE" >/dev/null 2>&1; then
        cache_mtime=$(stat -f %m "$CACHE_FILE" 2>/dev/null || echo 0)
    else
        cache_mtime=$(stat -c %Y "$CACHE_FILE" 2>/dev/null || echo 0)
    fi
    cache_age=$(($(date +%s) - cache_mtime))
    if [ "$cache_age" -lt "$CACHE_MAX_AGE" ]; then
        usage_json=$(cat "$CACHE_FILE")
    fi
fi

if [ -z "$usage_json" ] || [ "$usage_json" = "null" ]; then
    usage_json=$(fetch_usage)
    if [ -n "$usage_json" ] && [ "$usage_json" != "null" ]; then
        (umask 077 && echo "$usage_json" > "$CACHE_FILE")
    fi
fi

# ── Parse usage ───────────────────────────────────────

five_hour_pct=$(echo "$usage_json" | jq -r '(.five_hour.utilization // 0) | floor' 2>/dev/null)
five_hour_reset=$(echo "$usage_json" | jq -r '.five_hour.resets_at // ""' 2>/dev/null)
seven_day_pct=$(echo "$usage_json" | jq -r '(.seven_day.utilization // 0) | floor' 2>/dev/null)
seven_day_reset=$(echo "$usage_json" | jq -r '.seven_day.resets_at // ""' 2>/dev/null)
seven_day_sonnet_pct=$(echo "$usage_json" | jq -r '(.seven_day_sonnet.utilization // 0) | floor' 2>/dev/null)

[ -z "$five_hour_pct" ] || [ "$five_hour_pct" = "null" ] && five_hour_pct=0
[ -z "$seven_day_pct" ] || [ "$seven_day_pct" = "null" ] && seven_day_pct=0
[ -z "$seven_day_sonnet_pct" ] || [ "$seven_day_sonnet_pct" = "null" ] && seven_day_sonnet_pct=0

format_reset_time() {
    local iso_time="$1" format="$2"
    if [ -n "$iso_time" ] && [ "$iso_time" != "null" ]; then
        local clean_time=$(echo "$iso_time" | sed 's/\.[0-9]*//; s/+00:00$/Z/; s/Z$//')
        local unix_ts=$(date -j -u -f "%Y-%m-%dT%H:%M:%S" "$clean_time" "+%s" 2>/dev/null)
        if [ -n "$unix_ts" ]; then
            LC_ALL=C date -r "$unix_ts" "$format" 2>/dev/null
        fi
    fi
}

session_reset_display=$(format_reset_time "$five_hour_reset" "+%-I:%M%p" | sed 's/AM/am/; s/PM/pm/')
week_reset_display=$(format_reset_time "$seven_day_reset" "+%b%d %-I:%M%p" | sed 's/AM/am/; s/PM/pm/')

# ── Context window ────────────────────────────────────

model_name=$(echo "$input" | jq -r '.model.display_name')
usage=$(echo "$input" | jq '.context_window.current_usage')
context_size=$(echo "$input" | jq -r '.context_window.context_window_size')

context_pct=0
if [ "$usage" != "null" ]; then
    input_tokens=$(echo "$usage" | jq -r '.input_tokens')
    cache_creation=$(echo "$usage" | jq -r '.cache_creation_input_tokens')
    cache_read=$(echo "$usage" | jq -r '.cache_read_input_tokens')
    current_context=$((input_tokens + cache_creation + cache_read))
    if [ "$context_size" != "null" ] && [ "$context_size" -gt 0 ]; then
        context_pct=$((current_context * 100 / context_size))
    fi
fi

# ── Memory doc count (cached 5min) ────────────────────

MEM_CACHE="/tmp/rin-mem-count"
MEM_CACHE_AGE=300
mem_count=""

if [ -f "$MEM_CACHE" ]; then
    if stat -f %m "$MEM_CACHE" >/dev/null 2>&1; then
        mem_mtime=$(stat -f %m "$MEM_CACHE" 2>/dev/null || echo 0)
    else
        mem_mtime=$(stat -c %Y "$MEM_CACHE" 2>/dev/null || echo 0)
    fi
    mem_age=$(($(date +%s) - mem_mtime))
    if [ "$mem_age" -lt "$MEM_CACHE_AGE" ]; then
        mem_count=$(cat "$MEM_CACHE")
    fi
fi

if [ -z "$mem_count" ]; then
    # Use rin-memory-go count (no psql dependency)
    # Search in common locations
    GO_BIN=""
    for candidate in \
        "$HOME/workspace/project-rin-oss/src/rin_memory_go/rin-memory-go" \
        "$HOME/workspace/project-rin/src/rin_memory_go/rin-memory-go" \
        "$(command -v rin-memory-go 2>/dev/null)"; do
        if [ -x "$candidate" ]; then
            GO_BIN="$candidate"
            break
        fi
    done
    if [ -n "$GO_BIN" ]; then
        mem_count=$("$GO_BIN" count 2>/dev/null)
        [ -n "$mem_count" ] && echo "$mem_count" > "$MEM_CACHE"
    fi
fi

# ── Colors ────────────────────────────────────────────

select_color() {
    local pct=$1
    if [ "$pct" -ge 80 ]; then echo "$RED"
    elif [ "$pct" -ge 50 ]; then echo "$YELLOW"
    else echo "$GREEN"; fi
}

ctx_color=$(select_color "$context_pct")
session_color=$(select_color "$five_hour_pct")
week_color=$(select_color "$seven_day_pct")
sonnet_color=$(select_color "$seven_day_sonnet_pct")

session_reset_str=""
[ -n "$session_reset_display" ] && session_reset_str=" (${session_reset_display})"
week_reset_str=""
[ -n "$week_reset_display" ] && week_reset_str=" (${week_reset_display})"

# ── Output ────────────────────────────────────────────

# API error
if [ "$five_hour_pct" = "0" ] && [ "$seven_day_pct" = "0" ] && { [ -z "$usage_json" ] || [ "$usage_json" = "null" ]; }; then
    if [ -n "$mem_count" ] && [ "$mem_count" != "?" ]; then
        printf "${CYAN}凛${RESET} ${DIM}%s${RESET} ${DIM}|${RESET} Ctx: ${ctx_color}%d%%${RESET} ${DIM}|${RESET} ${RED}Usage: API Error${RESET} ${DIM}|${RESET} ${DIM}記憶${RESET} ${CYAN}%s${RESET}" \
            "$model_name" "$context_pct" "$mem_count"
    else
        printf "${CYAN}凛${RESET} ${DIM}%s${RESET} ${DIM}|${RESET} Ctx: ${ctx_color}%d%%${RESET} ${DIM}|${RESET} ${RED}Usage: API Error${RESET}" \
            "$model_name" "$context_pct"
    fi
    exit 0
fi

# Normal output
if [ -n "$mem_count" ] && [ "$mem_count" != "?" ]; then
    printf "${CYAN}凛${RESET} ${DIM}%s${RESET} ${DIM}|${RESET} Ctx: ${ctx_color}%d%%${RESET} ${DIM}|${RESET} Session: ${session_color}%d%%${RESET}${DIM}%s${RESET} ${DIM}|${RESET} Week: All ${week_color}%d%%${RESET} / Sonnet ${sonnet_color}%d%%${RESET}${DIM}%s${RESET} ${DIM}|${RESET} ${DIM}記憶${RESET} ${CYAN}%s${RESET}" \
        "$model_name" "$context_pct" "$five_hour_pct" "$session_reset_str" "$seven_day_pct" "$seven_day_sonnet_pct" "$week_reset_str" "$mem_count"
else
    printf "${CYAN}凛${RESET} ${DIM}%s${RESET} ${DIM}|${RESET} Ctx: ${ctx_color}%d%%${RESET} ${DIM}|${RESET} Session: ${session_color}%d%%${RESET}${DIM}%s${RESET} ${DIM}|${RESET} Week: All ${week_color}%d%%${RESET} / Sonnet ${sonnet_color}%d%%${RESET}${DIM}%s${RESET}" \
        "$model_name" "$context_pct" "$five_hour_pct" "$session_reset_str" "$seven_day_pct" "$seven_day_sonnet_pct" "$week_reset_str"
fi
