#!/usr/bin/env python3
"""Quick memory recall for RIN session start.

Queries ~/.rin/memory.db directly via stdlib sqlite3.
No external dependencies needed — runs without venv.
Outputs markdown to stdout for system prompt injection.
"""

import os
import sqlite3
import sys

PROJECT = os.environ.get("RIN_PROJECT")
LOAD_SESSION = os.environ.get("RIN_LOAD_SESSION")

DB_PATH = os.path.join(
    os.environ.get("RIN_MEMORY_DIR", os.path.expanduser("~/.rin")),
    "memory.db",
)

if not os.path.exists(DB_PATH):
    sys.exit(0)

db = sqlite3.connect(DB_PATH)
db.row_factory = sqlite3.Row

# Project filter (only if RIN_PROJECT is set)
project_filter = ""
project_params = []
if PROJECT:
    project_filter = " AND (project = ? OR project IS NULL)"
    project_params = [PROJECT]

# ── Loaded Session Context ─────────────────────────
# When RIN_LOAD_SESSION is set, load detailed context from that session
loaded_session_docs = []
if LOAD_SESSION:
    loaded_session_docs = db.execute(
        "SELECT kind, title, content, summary FROM documents "
        "WHERE source = ? AND archived = 0 "
        "ORDER BY kind, created_at",
        (LOAD_SESSION,),
    ).fetchall()

# Active tasks (unfinished work from previous sessions)
active_tasks = db.execute(
    "SELECT id, title, content FROM documents "
    "WHERE kind = 'active_task' AND archived = 0"
    + project_filter
    + " ORDER BY created_at DESC LIMIT 3",
    project_params,
).fetchall()

# Recent session journals (last 3, titles only for token saving)
journals = db.execute(
    "SELECT title, summary FROM documents "
    "WHERE kind = 'session_journal' AND archived = 0"
    + project_filter
    + " ORDER BY created_at DESC LIMIT 3",
    project_params,
).fetchall()

# Recent architectural decisions (last 3, with brief content)
decisions = db.execute(
    "SELECT title, content FROM documents "
    "WHERE kind = 'arch_decision' AND archived = 0"
    + project_filter
    + " ORDER BY created_at DESC LIMIT 3",
    project_params,
).fetchall()

# Team patterns (all, titles only)
patterns = db.execute(
    "SELECT title, content FROM documents "
    "WHERE kind = 'team_pattern' AND archived = 0"
    + project_filter
    + " ORDER BY created_at DESC LIMIT 5",
    project_params,
).fetchall()

# Operator preferences
preferences = db.execute(
    "SELECT title, content FROM documents "
    "WHERE kind = 'preference' AND archived = 0"
    + project_filter
    + " ORDER BY created_at DESC LIMIT 5",
    project_params,
).fetchall()

# Routing stats (last 7 days)
routing_logs = db.execute(
    "SELECT content FROM documents "
    "WHERE kind = 'routing_log' AND archived = 0"
    + project_filter
    + " AND created_at >= datetime('now', '-7 days')"
    " ORDER BY created_at DESC LIMIT 50",
    project_params,
).fetchall()

db.close()

lines = ["## Recent Memory (auto-loaded)", ""]

if loaded_session_docs:
    # Extract date from source: "session:2026-02-23" → "2026-02-23"
    source_date = LOAD_SESSION[8:] if LOAD_SESSION and LOAD_SESSION.startswith("session:") else (LOAD_SESSION or "")
    lines.append(f"### Loaded Session Context ({source_date})")
    lines.append("")
    for doc in loaded_session_docs:
        kind = doc["kind"]
        title = doc["title"]
        content = doc["content"] or ""
        summary = doc["summary"] or ""
        if kind == "session_journal":
            lines.append(f"**{title}**")
            # Use summary if available, otherwise truncate content
            text = summary if summary else content[:500]
            if text:
                lines.append(text)
            lines.append("")
        elif kind == "arch_decision":
            lines.append(f"**[Decision] {title}**")
            lines.append(content[:300])
            lines.append("")
        else:
            lines.append(f"**[{kind}] {title}**")
            lines.append(content[:200])
            lines.append("")
    lines.append("---")
    lines.append("")

if active_tasks:
    lines.append("### Active Tasks (unfinished)")
    for t in active_tasks:
        content = t["content"][:200].replace("\n", " ")
        lines.append(f"- [{t['id']}] **{t['title']}**: {content}")
    lines.append("")

if journals:
    lines.append("### Recent Sessions")
    for j in journals:
        summary = j["summary"] or ""
        if summary:
            lines.append(f"- {j['title']} — {summary}")
        else:
            lines.append(f"- {j['title']}")
    lines.append("")

if decisions:
    lines.append("### Recent Decisions")
    for d in decisions:
        content = d["content"][:120].replace("\n", " ")
        lines.append(f"- **{d['title']}**: {content}...")
    lines.append("")

if patterns:
    lines.append("### Team Patterns")
    for p in patterns:
        content = p["content"][:200].replace("\n", " ")
        lines.append(f"- {content}")
    lines.append("")

if preferences:
    lines.append("### Operator Preferences")
    for p in preferences:
        content = p["content"][:150].replace("\n", " ")
        lines.append(f"- **{p['title']}**: {content}")
    lines.append("")

if routing_logs:
    import json as _json

    model_stats = {}
    for row in routing_logs:
        try:
            data = _json.loads(row["content"])
            m = data.get("model", "?")
            if m not in model_stats:
                model_stats[m] = {"ok": 0, "fail": 0}
            if data.get("success"):
                model_stats[m]["ok"] += 1
            else:
                model_stats[m]["fail"] += 1
        except (_json.JSONDecodeError, KeyError):
            pass

    if model_stats:
        lines.append("### Routing Stats (7d)")
        for m, s in model_stats.items():
            total = s["ok"] + s["fail"]
            rate = round(s["ok"] / total * 100) if total else 0
            lines.append(f"- {m}: {rate}% ({s['ok']}/{total})")
        lines.append("")

# Only output if there's actual content beyond the header
if len(lines) <= 2:
    sys.exit(0)

lines.append("Use `memory_search` for detailed context on any topic.")

print("\n".join(lines))
