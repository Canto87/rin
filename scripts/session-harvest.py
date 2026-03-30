#!/usr/bin/env python3
"""Claude Code session auto-harvest script.

Automatically converts stale (unchanged for 5+ min) + sufficiently large (>10KB) +
unprocessed session JSONL files into meeting notes markdown.

Usage:
  python3 scripts/session-harvest.py             # Normal run (max 20)
  python3 scripts/session-harvest.py --backfill   # Remove batch limit
"""

import argparse
import importlib.util
import json
import sys
import time
from datetime import datetime, timezone, timedelta
from pathlib import Path

# ── Constants ──────────────────────────────────────────────

PROJECT_DIR = Path(__file__).resolve().parent.parent
SESSIONS_DIR = PROJECT_DIR / "memory" / "sessions"
STATE_FILE = PROJECT_DIR / "memory" / ".harvest-state.json"
CLAUDE_PROJECTS_DIR = Path.home() / ".claude" / "projects"

STALE_MINUTES = 5
MIN_SIZE_BYTES = 10 * 1024  # 10KB
BATCH_LIMIT = 20

KST = timezone(timedelta(hours=9))

RIN_MEMORY_DIR = Path.home() / ".rin"

# ── session-notes.py import ──────────────────────────────

_session_notes = None


def _get_session_notes():
    """Lazy import the session-notes.py module."""
    global _session_notes
    if _session_notes is None:
        spec = importlib.util.spec_from_file_location(
            "session_notes",
            str(PROJECT_DIR / "scripts" / "session-notes.py"),
        )
        if spec is None or spec.loader is None:
            raise ImportError("Failed to load session-notes.py")
        _session_notes = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(_session_notes)
    return _session_notes


# ── State Management ──────────────────────────────────────


def load_state() -> dict:
    """Load state from STATE_FILE. Returns defaults if not found."""
    if STATE_FILE.exists():
        try:
            return json.loads(STATE_FILE.read_text())
        except (json.JSONDecodeError, OSError):
            pass
    return {"version": 1, "processed": {}, "pending_ingest": [], "last_run": None}


def save_state(state: dict) -> None:
    """Save state to STATE_FILE."""
    STATE_FILE.parent.mkdir(parents=True, exist_ok=True)
    STATE_FILE.write_text(json.dumps(state, indent=2, ensure_ascii=False) + "\n")


# ── RIN Project Registry ──────────────────────────────────

RIN_PROJECTS_FILE = RIN_MEMORY_DIR / "rin-projects.json"


def load_rin_projects() -> set[str]:
    """Load project slug list registered by the rin script."""
    if RIN_PROJECTS_FILE.exists():
        try:
            data = json.loads(RIN_PROJECTS_FILE.read_text())
            return set(data.keys())
        except (json.JSONDecodeError, OSError):
            pass
    return set()


# ── Harvest Target Discovery ──────────────────────────────


def find_harvestable(state: dict) -> list[Path]:
    """Return list of harvestable JSONL files (RIN projects only, including re-harvest).

    New harvest conditions:
    - Not in state["processed"]
    - mtime is STALE_MINUTES or older
    - File size >= MIN_SIZE_BYTES
    - Project is registered in rin-projects.json

    Re-harvest conditions:
    - In state["processed"] but file size has grown (session resumed)
    - mtime is STALE_MINUTES or older (inactive again)
    """
    if not CLAUDE_PROJECTS_DIR.exists():
        return []

    rin_projects = load_rin_projects()
    if not rin_projects:
        return []

    now = time.time()
    candidates = []

    for project_dir in CLAUDE_PROJECTS_DIR.iterdir():
        if not project_dir.is_dir():
            continue

        # Skip if not a RIN project
        if project_dir.name not in rin_projects:
            continue

        for jsonl_path in project_dir.glob("*.jsonl"):
            session_id = jsonl_path.stem
            stat = jsonl_path.stat()

            # Still an active session (not stale yet)
            if now - stat.st_mtime < STALE_MINUTES * 60:
                continue

            # Too small
            if stat.st_size < MIN_SIZE_BYTES:
                continue

            prev = state["processed"].get(session_id)
            if prev:
                # Already processed — re-harvest if file has grown
                prev_size = prev.get("jsonl_size", 0)
                if stat.st_size <= prev_size:
                    continue

            candidates.append(jsonl_path)

    # Sort by mtime (oldest first)
    candidates.sort(key=lambda p: p.stat().st_mtime)
    return candidates


# ── Individual Harvest ─────────────────────────────────────


def harvest_one(jsonl_path: Path, state: dict) -> bool:
    """Convert a single JSONL file to meeting notes.

    Returns:
        True: success, False: failure
    """
    try:
        session_notes = _get_session_notes()
        notes, filename, msg_count, title = session_notes.process_jsonl(jsonl_path)

        SESSIONS_DIR.mkdir(parents=True, exist_ok=True)
        output_path = SESSIONS_DIR / filename
        output_path.write_text(notes)

        session_id = jsonl_path.stem
        project_slug = jsonl_path.parent.name  # Claude Code's project dir name
        now = datetime.now(KST).isoformat()
        prev = state["processed"].get(session_id)
        is_reharvest = prev is not None

        state["processed"][session_id] = {
            "harvested_at": now,
            "notes_path": str(output_path.relative_to(PROJECT_DIR)),
            "msg_count": msg_count,
            "jsonl_size": jsonl_path.stat().st_size,
            "summarized_at": None,  # Reset on re-harvest to re-run review
            "source": "reharvest" if is_reharvest else "harvest",
            "project": project_slug,
            "title": title,
        }

        if is_reharvest:
            print(
                f"[REHARVEST] {jsonl_path.name}: {prev.get('msg_count', '?')} → {msg_count} msgs"
            )

        return True

    except Exception as e:
        print(f"[ERROR] {jsonl_path.name}: {e}", file=sys.stderr)
        return False


# ── main ─────────────────────────────────────────────────


def backfill_titles(state: dict) -> int:
    """Backfill titles from JSONL for harvest-state entries missing a title."""
    session_notes = _get_session_notes()
    updated = 0

    for sid, info in state.get("processed", {}).items():
        if info.get("title"):
            continue
        project = info.get("project", "")
        jsonl = CLAUDE_PROJECTS_DIR / project / f"{sid}.jsonl"
        if not jsonl.exists():
            continue
        try:
            msgs = session_notes.parse_session(jsonl)
            msgs = session_notes.merge_assistant_messages(msgs)
            title = session_notes.extract_title(msgs)
            if title:
                info["title"] = title
                updated += 1
        except Exception:
            pass

    return updated


def main():
    parser = argparse.ArgumentParser(
        description="Auto-harvest Claude Code session JSONL files",
    )
    parser.add_argument(
        "--backfill",
        action="store_true",
        help="Remove batch limit (20), harvest all unprocessed sessions",
    )
    parser.add_argument(
        "--backfill-titles",
        action="store_true",
        help="Extract and backfill titles from JSONL for entries missing them",
    )
    args = parser.parse_args()

    state = load_state()

    if args.backfill_titles:
        updated = backfill_titles(state)
        save_state(state)
        print(f"Backfilled {updated} titles")
        return

    candidates = find_harvestable(state)

    if not candidates:
        print("Harvested 0/0 sessions")
        state["last_run"] = datetime.now(KST).isoformat()
        save_state(state)
        return

    limit = len(candidates) if args.backfill else BATCH_LIMIT
    targets = candidates[:limit]

    success = 0
    for jsonl_path in targets:
        if harvest_one(jsonl_path, state):
            success += 1

    state["last_run"] = datetime.now(KST).isoformat()
    save_state(state)

    print(f"Harvested {success}/{len(targets)} sessions")


if __name__ == "__main__":
    main()
