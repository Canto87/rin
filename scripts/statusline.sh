#!/bin/bash
# Claude Code statusline for RIN
# Shows: Model | Ctx% | Session usage | Week usage | Memory doc count
# Usage data comes from Claude Code's stdin JSON (rate_limits field).

RED=$'\033[31m'
YELLOW=$'\033[33m'
GREEN=$'\033[32m'
CYAN=$'\033[36m'
DIM=$'\033[2m'
RESET=$'\033[0m'

input=$(cat)

# ── Parse rate limits (from stdin, no API call needed) ─

five_hour_pct=$(echo "$input" | jq -r '(.rate_limits.five_hour.used_percentage // 0) | floor' 2>/dev/null)
five_hour_reset=$(echo "$input" | jq -r '.rate_limits.five_hour.resets_at // empty' 2>/dev/null)
seven_day_pct=$(echo "$input" | jq -r '(.rate_limits.seven_day.used_percentage // 0) | floor' 2>/dev/null)
seven_day_reset=$(echo "$input" | jq -r '.rate_limits.seven_day.resets_at // empty' 2>/dev/null)

[ -z "$five_hour_pct" ] || [ "$five_hour_pct" = "null" ] && five_hour_pct=0
[ -z "$seven_day_pct" ] || [ "$seven_day_pct" = "null" ] && seven_day_pct=0

format_reset_time() {
    local ts="$1" format="$2"
    if [ -n "$ts" ] && [ "$ts" != "null" ] && [ "$ts" != "0" ]; then
        LC_ALL=C date -r "$ts" "$format" 2>/dev/null
    fi
}

session_reset_display=$(format_reset_time "$five_hour_reset" "+%-I:%M%p" | sed 's/AM/am/; s/PM/pm/')
week_reset_display=$(format_reset_time "$seven_day_reset" "+%b%d %-I:%M%p" | sed 's/AM/am/; s/PM/pm/')

# ── Context window ────────────────────────────────────

model_name=$(echo "$input" | jq -r '.model.display_name')
context_pct=$(echo "$input" | jq -r '(.context_window.used_percentage // 0) | floor' 2>/dev/null)
[ -z "$context_pct" ] || [ "$context_pct" = "null" ] && context_pct=0

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
    GO_BIN=""
    for candidate in \
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

session_reset_str=""
[ -n "$session_reset_display" ] && session_reset_str=" (${session_reset_display})"
week_reset_str=""
[ -n "$week_reset_display" ] && week_reset_str=" (${week_reset_display})"

# ── Output ────────────────────────────────────────────

mem_str=""
if [ -n "$mem_count" ] && [ "$mem_count" != "?" ]; then
    mem_str=" ${DIM}|${RESET} ${DIM}記憶${RESET} ${CYAN}${mem_count}${RESET}"
fi

printf "${CYAN}凛${RESET} ${DIM}%s${RESET} ${DIM}|${RESET} Ctx: ${ctx_color}%d%%${RESET} ${DIM}|${RESET} Session: ${session_color}%d%%${RESET}${DIM}%s${RESET} ${DIM}|${RESET} Week: ${week_color}%d%%${RESET}${DIM}%s${RESET}%s" \
    "$model_name" "$context_pct" "$five_hour_pct" "$session_reset_str" "$seven_day_pct" "$week_reset_str" "$mem_str"
