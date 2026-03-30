#!/usr/bin/env python3
"""Claude Code session JSONL to meeting notes markdown converter.

Usage:
  python3 scripts/session-notes.py                     # Latest session (all projects)
  python3 scripts/session-notes.py <session-id>        # Specific session
  python3 scripts/session-notes.py <path-to-jsonl>     # Direct path

Output: memory/sessions/YYYY-MM-DD-HHMMSS.md
"""

from __future__ import annotations

import json
import sys
import os
from pathlib import Path
from datetime import datetime, timezone, timedelta

PROJECT_DIR = Path(__file__).resolve().parent.parent
SESSIONS_DIR = PROJECT_DIR / "memory" / "sessions"
STATE_FILE = PROJECT_DIR / "memory" / ".harvest-state.json"
CLAUDE_PROJECTS_DIR = Path.home() / ".claude" / "projects"

KST = timezone(timedelta(hours=9))


def find_jsonl(arg: str | None = None) -> Path:
    """Determine JSONL file path. Scans all project directories."""
    if arg and os.path.isfile(arg):
        return Path(arg)

    if not CLAUDE_PROJECTS_DIR.exists():
        raise FileNotFoundError(f"Claude projects directory not found: {CLAUDE_PROJECTS_DIR}")

    if arg:
        # Treat as session-id — search across all projects
        for project_dir in CLAUDE_PROJECTS_DIR.iterdir():
            if not project_dir.is_dir():
                continue
            candidate = project_dir / f"{arg}.jsonl"
            if candidate.exists():
                return candidate
        raise FileNotFoundError(f"Session file not found: {arg}")

    # Latest JSONL — scan all projects
    all_jsonl = []
    for project_dir in CLAUDE_PROJECTS_DIR.iterdir():
        if not project_dir.is_dir():
            continue
        all_jsonl.extend(project_dir.glob("*.jsonl"))

    if not all_jsonl:
        raise FileNotFoundError(f"No JSONL files found: {CLAUDE_PROJECTS_DIR}")

    all_jsonl.sort(key=lambda p: p.stat().st_mtime, reverse=True)
    return all_jsonl[0]


def parse_timestamp(ts: str) -> datetime:
    """ISO timestamp → KST datetime."""
    dt = datetime.fromisoformat(ts.replace("Z", "+00:00"))
    return dt.astimezone(KST)


def extract_text(content) -> str | None:
    """Extract text from message content."""
    if isinstance(content, str):
        return content.strip()
    if isinstance(content, list):
        texts = []
        for block in content:
            if isinstance(block, dict) and block.get("type") == "text":
                text = block.get("text", "").strip()
                if text:
                    texts.append(text)
        return "\n".join(texts) if texts else None
    return None


def has_tool_use(content) -> bool:
    """Check if content contains tool_use."""
    if isinstance(content, list):
        return any(isinstance(b, dict) and b.get("type") == "tool_use" for b in content)
    return False


def is_tool_result(content) -> bool:
    """Check if content is a tool_result."""
    if isinstance(content, list):
        return any(
            isinstance(b, dict) and b.get("type") == "tool_result" for b in content
        )
    return False


NOISE_PATTERNS = [
    "<command-name>",
    "<local-command-",
    "<system-reminder>",
    "This session is being continued from a previous conversation",
    "[Request interrupted by user for tool use]",
    "[Request interrupted by user]",
    "Prompt is too long",
]


def is_noise(text: str) -> bool:
    """Filter noise such as system messages and commands."""
    return any(p in text for p in NOISE_PATTERNS)


def parse_session(jsonl_path: Path) -> list[dict]:
    """Parse JSONL into a list of conversation messages."""
    messages = []
    seen_uuids = set()

    with open(jsonl_path) as f:
        for line in f:
            line = line.strip()
            if not line:
                continue
            try:
                entry = json.loads(line)
            except json.JSONDecodeError:
                continue

            entry_type = entry.get("type")
            uuid = entry.get("uuid")

            # Deduplicate (same assistant message may appear in multiple streaming lines)
            if uuid and uuid in seen_uuids and entry_type == "assistant":
                # Update with more complete content
                msg = entry.get("message", {})
                content = msg.get("content")
                text = extract_text(content)
                if text:
                    for m in messages:
                        if m.get("uuid") == uuid and not m.get("text"):
                            m["text"] = text
                continue
            if uuid:
                seen_uuids.add(uuid)

            if entry_type == "user":
                msg = entry.get("message", {})
                content = msg.get("content")
                if is_tool_result(content):
                    continue
                text = extract_text(content)
                if not text or is_noise(text):
                    continue
                messages.append(
                    {
                        "role": "user",
                        "text": text,
                        "timestamp": entry.get("timestamp", ""),
                        "uuid": uuid,
                    }
                )

            elif entry_type == "assistant":
                msg = entry.get("message", {})
                content = msg.get("content")
                text = extract_text(content)
                tool_use = has_tool_use(content)
                if text and not is_noise(text):
                    messages.append(
                        {
                            "role": "assistant",
                            "text": text,
                            "timestamp": entry.get("timestamp", ""),
                            "uuid": uuid,
                            "tool_use": tool_use,
                        }
                    )
                elif tool_use and not text:
                    messages.append(
                        {
                            "role": "assistant",
                            "text": None,
                            "timestamp": entry.get("timestamp", ""),
                            "uuid": uuid,
                            "tool_use": True,
                        }
                    )

    return messages


def merge_assistant_messages(messages: list[dict]) -> list[dict]:
    """Merge consecutive assistant messages into one."""
    merged = []
    for msg in messages:
        if merged and merged[-1]["role"] == "assistant" and msg["role"] == "assistant":
            prev = merged[-1]
            if msg["text"]:
                if prev["text"]:
                    prev["text"] += "\n" + msg["text"]
                else:
                    prev["text"] = msg["text"]
            if msg.get("tool_use"):
                prev["tool_use"] = True
        else:
            merged.append(dict(msg))
    return merged


def format_notes(messages: list[dict], session_id: str) -> str:
    """Convert conversation messages to meeting notes markdown."""
    if not messages:
        return "# Session Notes\n\n(empty session)"

    first_ts = parse_timestamp(messages[0]["timestamp"])
    last_ts = parse_timestamp(messages[-1]["timestamp"])
    date_str = first_ts.strftime("%Y-%m-%d")
    start_time = first_ts.strftime("%H:%M")
    end_time = last_ts.strftime("%H:%M")

    lines = [
        f"# Session Notes — {date_str}",
        "",
        f"- **Time**: {start_time} ~ {end_time} KST",
        f"- **Participants**: Operator, RIN",
        f"- **Session**: `{session_id}`",
        "",
        "---",
        "",
    ]

    for msg in messages:
        ts = parse_timestamp(msg["timestamp"])
        time_str = ts.strftime("%H:%M")

        if msg["role"] == "user":
            lines.append(f"**[{time_str}] Operator**:{msg['text']}")
            lines.append("")
        elif msg["role"] == "assistant":
            if msg["text"]:
                text = msg["text"]
                # Truncate long responses
                if len(text) > 500:
                    text = text[:500] + "..."
                prefix = f"**[{time_str}] RIN**: "
                if msg.get("tool_use"):
                    prefix = f"**[{time_str}] RIN** _(performing task)_: "
                lines.append(f"{prefix}{text}")
            elif msg.get("tool_use"):
                lines.append(f"**[{time_str}] RIN**: _(investigating code / performing task)_")
            lines.append("")

    return "\n".join(lines)


GREETING_PATTERNS = {"hi", "hello", "hey", "안녕", "ㅎㅇ", "ㅇㅇ"}

TITLE_NOISE = [
    "<teammate-message",
    "<task-notification",
    "Base directory for this skill:",
    "Implement the following plan:",
    "Continue from where you left off",
]


def extract_title(messages: list[dict]) -> str | None:
    """Extract the last substantive user message as the title."""
    import re

    for msg in reversed(messages):
        if msg["role"] != "user":
            continue
        text = msg["text"].strip()
        # Remove XML tags
        text = re.sub(r"<[^>]+>", "", text).strip()
        if len(text) < 8:
            continue
        if any(p in msg["text"] for p in TITLE_NOISE):
            continue
        first_word = text.split()[0].lower().rstrip("~!.,")
        if first_word in GREETING_PATTERNS:
            continue
        # Remove markdown headers, first line only
        first_line = text.split("\n")[0].strip().lstrip("#").strip()
        return first_line[:60] if len(first_line) >= 8 else text[:60]
    return None


def process_jsonl(jsonl_path: Path) -> tuple[str, str, int, str | None]:
    """JSONL to meeting notes conversion. Returns (notes_text, filename, msg_count, title)."""
    session_id = jsonl_path.stem
    messages = parse_session(jsonl_path)
    messages = merge_assistant_messages(messages)
    notes = format_notes(messages, session_id)
    title = extract_title(messages)
    if messages:
        first_ts = parse_timestamp(messages[0]["timestamp"])
        filename = first_ts.strftime("%Y-%m-%d-%H%M%S") + ".md"
    else:
        filename = f"{session_id[:8]}.md"
    return notes, filename, len(messages), title


def mark_processed(
    session_id: str,
    notes_path: str,
    source: str = "manual",
    ingested_at: str | None = None,
) -> None:
    """Record session processing state to .harvest-state.json."""
    state = {"version": 1, "processed": {}, "pending_ingest": [], "last_run": None}
    if STATE_FILE.exists():
        try:
            state = json.loads(STATE_FILE.read_text())
        except (json.JSONDecodeError, OSError):
            pass

    now = datetime.now(KST).isoformat()
    state["processed"][session_id] = {
        "harvested_at": now,
        "notes_path": notes_path,
        "msg_count": None,  # caller can set if known
        "summarized_at": None,
        "ingested_at": ingested_at,
        "doc_ids": [],
        "source": source,
    }
    state["last_run"] = now

    STATE_FILE.parent.mkdir(parents=True, exist_ok=True)
    STATE_FILE.write_text(json.dumps(state, indent=2, ensure_ascii=False) + "\n")


def main():
    arg = sys.argv[1] if len(sys.argv) > 1 else None
    jsonl_path = find_jsonl(arg)
    session_id = jsonl_path.stem

    print(f"Parsing: {jsonl_path.name}")

    notes, filename, msg_count, title = process_jsonl(jsonl_path)

    print(f"Extracted {msg_count} messages")

    SESSIONS_DIR.mkdir(parents=True, exist_ok=True)
    output_path = SESSIONS_DIR / filename
    output_path.write_text(notes)

    print(f"Saved: {output_path.relative_to(PROJECT_DIR)}")

    # Record to state file (ignore failures)
    try:
        mark_processed(session_id, str(output_path.relative_to(PROJECT_DIR)))
    except Exception:
        pass


if __name__ == "__main__":
    main()
