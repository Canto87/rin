#!/bin/bash
# Session review: RIN summarizes harvested transcripts + extracts knowledge → memory_store
# If there are no unprocessed sessions, RIN is not launched (zero tokens)

RIN_HOME="${RIN_HOME:-$(cd "$(dirname "$0")/.." && pwd)}"
HARVEST_STATE="$RIN_HOME/memory/.harvest-state.json"
MAX_BATCH=5

if [ ! -f "$HARVEST_STATE" ]; then
  exit 0
fi

# Extract sid + notes file path of unsummarized sessions (up to MAX_BATCH)
session_pairs=()
while IFS= read -r line; do
  [ -n "$line" ] && session_pairs+=("$line")
done < <("$RIN_HOME/.venv/bin/python" -c "
import json
state = json.loads(open('$HARVEST_STATE').read())
items = sorted(state.get('processed', {}).items(), key=lambda x: x[1].get('harvested_at', ''))
for sid, info in items:
    if info.get('summarized_at') is None and info.get('notes_path'):
        print(f'{sid}\t{info[\"notes_path\"]}')
" | head -n $MAX_BATCH)

notes_files=()
session_ids=()
for pair in "${session_pairs[@]}"; do
  IFS=$'\t' read -r sid notes <<< "$pair"
  session_ids+=("$sid")
  notes_files+=("$notes")
done

if [ ${#notes_files[@]} -eq 0 ]; then
  exit 0
fi

echo "[$(date -u +%Y-%m-%dT%H:%M:%S)] Processing ${#notes_files[@]} sessions"

# RIN context
RIN_CTX="$RIN_HOME/context/rin-context.md"
if [ ! -f "$RIN_CTX" ]; then
  exit 1
fi

# Assemble prompt (temp file — avoid ARG_MAX)
TMPFILE=$(mktemp /tmp/rin-review-XXXXXX)
trap 'rm -f "$TMPFILE"' EXIT

cat > "$TMPFILE" << 'INSTRUCTIONS'
Below are recent session transcripts. Read each transcript and:

1. **session_summary** (memory_store, kind='session_summary'): Session title + 2-3 sentence summary. Include key topics in tags.
2. **Structured knowledge extraction** (memory_store only for applicable items):
   - arch_decision: Architectural decisions and their rationale
   - domain_knowledge: Domain knowledge, external service quirks, troubleshooting records
   - team_pattern: Team workflow patterns, communication rules
3. **active_task** (memory_store, kind='active_task'): Tasks that were still incomplete when the session ended. Record the current state and what remains so the next session can continue.

Use memory_search to check for duplicates against existing memories, then memory_store only new content.
For sessions with nothing to extract, store only the session_summary.

After processing each session, output the session title in the following format, one line per session (SID is in the header):
REVIEW_TITLE|<SID>|<one-line title summarizing the entire session (60 chars max)>
INSTRUCTIONS

for i in "${!notes_files[@]}"; do
  f="${notes_files[$i]}"
  sid="${session_ids[$i]}"
  full_path="$RIN_HOME/$f"
  if [ -f "$full_path" ]; then
    echo "--- SESSION [SID:$sid]: $f ---" >> "$TMPFILE"
    head -c 15000 "$full_path" >> "$TMPFILE"
    echo "" >> "$TMPFILE"
  fi
done

SYSTEM_PROMPT="$(cat "$RIN_CTX")

You are RIN. Follow the identity and decision boundaries above."

CLAUDE_BIN="${CLAUDE_BIN:-$(command -v claude 2>/dev/null || echo "$HOME/.local/bin/claude")}"

review_output=$("$CLAUDE_BIN" -p "$(cat "$TMPFILE")" \
  --append-system-prompt "$SYSTEM_PROMPT" \
  --allowedTools "mcp__rin-memory__memory_store,mcp__rin-memory__memory_search,mcp__rin-memory__memory_lookup" \
  --permission-mode bypassPermissions \
  --no-session-persistence)

exit_code=$?

if [ $exit_code -eq 0 ]; then
  # Mark processed sessions with summarized_at + update titles
  notes_joined=$(printf '%s\n' "${notes_files[@]}")
  "$RIN_HOME/.venv/bin/python" << PYEOF
import json
from datetime import datetime, timezone
state = json.loads(open('$HARVEST_STATE').read())
now = datetime.now(timezone.utc).isoformat()
files = set("""$notes_joined""".strip().split('\n'))
count = 0
for sid, info in state.get('processed', {}).items():
    if info.get('notes_path') in files:
        info['summarized_at'] = now
        info['summary_method'] = 'rin-review'
        count += 1

# Parse review titles from Claude output
review_titles = {}
for line in """$review_output""".strip().split('\n'):
    if line.startswith('REVIEW_TITLE|'):
        parts = line.split('|', 2)
        if len(parts) == 3:
            sid_key = parts[1].strip()
            title = parts[2].strip()
            if sid_key and title:
                review_titles[sid_key] = title

title_count = 0
for sid, title in review_titles.items():
    if sid in state.get('processed', {}):
        state['processed'][sid]['title'] = title
        title_count += 1

with open('$HARVEST_STATE', 'w') as f:
    f.write(json.dumps(state, indent=2, ensure_ascii=False) + '\n')
print(f'Marked {count} sessions as summarized, updated {title_count} titles')
PYEOF
  echo "[$(date -u +%Y-%m-%dT%H:%M:%S)] Review complete"
else
  echo "[$(date -u +%Y-%m-%dT%H:%M:%S)] Review failed (claude exit $exit_code)"
fi
