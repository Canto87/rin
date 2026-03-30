#!/usr/bin/env python3
"""Summarize harvested session transcripts with Gemini Flash -> store in rin-memory.

NOTE: Since the harvest step now ingests directly, this script is only
used when manual re-summarization is needed.

Usage:
  python3 scripts/session-summarize.py           # Process unsummarized sessions (max 10)
  python3 scripts/session-summarize.py --all     # Process all unsummarized sessions
"""

from __future__ import annotations

import argparse
import asyncio
import json
import subprocess
import sys
from datetime import datetime, timedelta, timezone
from pathlib import Path

# -- Constants and paths -------------------------------------------

PROJECT_DIR = Path(__file__).resolve().parent.parent
STATE_FILE = PROJECT_DIR / "memory" / ".harvest-state.json"
BATCH_LIMIT = 10
KST = timezone(timedelta(hours=9))

RIN_MEMORY_DIR = Path.home() / ".rin"
RIN_MEMORY_SRC = PROJECT_DIR / "src"
RIN_MEMORY_VENV = PROJECT_DIR / ".venv"

# -- Gemini Flash summarization prompt -----------------------------

SUMMARIZE_PROMPT = """The following is a development session transcript. Please summarize it in JSON format.

Rules:
- summary: Summarize the key content of the session in 2-3 sentences
- decisions: List of technical decisions made during the session (empty array if none)
- topics: List of main topics/keywords
- action_items: Items requiring follow-up work (empty array if none)

Output only JSON. No other text.

{
  "summary": "...",
  "decisions": ["..."],
  "topics": ["..."],
  "action_items": ["..."]
}

---
Transcript:
"""

# -- State management ----------------------------------------------


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


# -- Unsummarized session discovery --------------------------------


def find_unsummarized(state: dict) -> list[tuple[str, dict]]:
    """Find unsummarized sessions with existing notes_path in state['processed'].

    Returns:
        List of (session_id, session_info) tuples, sorted by harvested_at (oldest first).
    """
    results = []
    for session_id, info in state.get("processed", {}).items():
        if info.get("summarized_at") is not None:
            continue
        notes_path = info.get("notes_path")
        if not notes_path:
            continue
        full_path = PROJECT_DIR / notes_path
        if not full_path.exists():
            continue
        results.append((session_id, info))

    # Sort by harvested_at (oldest first)
    results.sort(key=lambda x: x[1].get("harvested_at", ""))
    return results


# -- Gemini Flash summarization ------------------------------------


def summarize_with_gemini(notes_text: str) -> dict | None:
    """Summarize transcript with Gemini Flash."""
    prompt = SUMMARIZE_PROMPT + notes_text[:8000]  # Token limit
    try:
        result = subprocess.run(
            ["gemini", "-p", prompt],
            capture_output=True,
            text=True,
            timeout=60,
        )
        if result.returncode != 0:
            return None
        # Extract JSON (Gemini may wrap in ```json block)
        output = result.stdout.strip()
        if output.startswith("```"):
            lines = output.split("\n")
            output = "\n".join(lines[1:-1])
        return json.loads(output)
    except (subprocess.TimeoutExpired, FileNotFoundError, json.JSONDecodeError) as e:
        print(f"  Gemini failed: {e}", file=sys.stderr)
        return None


# -- Heuristic summary (Gemini fallback) ---------------------------


def heuristic_summary(notes_text: str, session_id: str) -> dict:
    """Heuristic summary when Gemini fails."""
    lines = notes_text.strip().split("\n")
    # Extract topic from first user message
    topic = session_id[:8]
    for line in lines:
        if "Operator**:" in line:
            topic = line.split(":", 1)[-1].strip()[:100]
            break
    msg_count = sum(1 for ln in lines if "**:" in ln)
    return {
        "summary": f"Development session ({msg_count} messages). Topic: {topic}",
        "decisions": [],
        "topics": [topic[:50]],
        "action_items": [],
    }


# -- rin-memory storage --------------------------------------------


async def ingest_to_memory(
    session_id: str, notes_path: str, summary: dict
) -> list[str]:
    """Store summary in rin-memory."""
    # Add venv site-packages + src
    venv_sp = RIN_MEMORY_VENV / "lib"
    if venv_sp.exists():
        for sp in venv_sp.glob("python*/site-packages"):
            sys.path.insert(0, str(sp))
            break
    sys.path.insert(0, str(RIN_MEMORY_SRC))
    try:
        from rin_memory.store import MemoryStore
    except ImportError:
        print("  rin-memory import failed", file=sys.stderr)
        return []

    doc_ids: list[str] = []
    store = MemoryStore(
        str(RIN_MEMORY_DIR / "memory.db"),
        str(RIN_MEMORY_DIR / "vectors"),
    )
    try:
        await store.initialize()

        # Store session summary
        content = f"## Summary\n{summary['summary']}\n\n"
        if summary.get("decisions"):
            content += (
                "## Decisions\n"
                + "\n".join(f"- {d}" for d in summary["decisions"])
                + "\n\n"
            )
        if summary.get("action_items"):
            content += (
                "## Action Items\n"
                + "\n".join(f"- {a}" for a in summary["action_items"])
                + "\n\n"
            )
        content += f"## Meta\n- Session: `{session_id}`\n- Source: `{notes_path}`"

        topics = summary.get("topics", [])
        title_topic = topics[0] if topics else session_id[:8]

        doc_id = await store.store_document(
            kind="session_summary",
            title=f"Session Summary: {title_topic}",
            content=content,
            tags=topics + ["auto:session-summarize"],
            source="auto:session-summarize",
        )
        if doc_id:
            doc_ids.append(str(doc_id))
    except Exception as e:
        print(f"  Memory storage failed: {e}", file=sys.stderr)
    finally:
        await store.close()

    return doc_ids


# -- Individual session processing ---------------------------------


def process_one(session_id: str, info: dict, state: dict) -> bool:
    """Process a single session summary.

    Returns:
        True: success, False: failure
    """
    notes_path = info.get("notes_path", "")
    full_path = PROJECT_DIR / notes_path
    if not full_path.exists():
        print(f"  [{session_id[:8]}] Notes file not found: {notes_path}", file=sys.stderr)
        return False

    notes_text = full_path.read_text()
    if not notes_text.strip():
        print(f"  [{session_id[:8]}] Empty transcript, skipping")
        return False

    # 1. Try summarizing with Gemini
    summary = summarize_with_gemini(notes_text)
    method = "gemini"
    if summary is None:
        # 2. Fall back to heuristic on failure
        summary = heuristic_summary(notes_text, session_id)
        method = "heuristic"

    # 3. Store in rin-memory
    doc_ids = asyncio.run(ingest_to_memory(session_id, notes_path, summary))

    now = datetime.now(KST).isoformat()

    # 4. Update state
    entry = state["processed"].get(session_id, {})
    entry["summarized_at"] = now
    entry["summary_method"] = method

    if doc_ids:
        entry["ingested_at"] = now
        entry["doc_ids"] = doc_ids
        # Remove from pending_ingest
        if session_id in state.get("pending_ingest", []):
            state["pending_ingest"].remove(session_id)
    else:
        # Memory storage failed -> add to pending_ingest
        if session_id not in state.get("pending_ingest", []):
            state.setdefault("pending_ingest", []).append(session_id)

    state["processed"][session_id] = entry
    return True


# -- pending_ingest retry ------------------------------------------


def retry_pending(state: dict) -> int:
    """Retry memory storage for sessions in pending_ingest.

    Returns:
        Number of successful retries.
    """
    pending = list(state.get("pending_ingest", []))
    if not pending:
        return 0

    success = 0
    for session_id in pending:
        info = state.get("processed", {}).get(session_id)
        if not info:
            state["pending_ingest"].remove(session_id)
            continue

        notes_path = info.get("notes_path", "")
        full_path = PROJECT_DIR / notes_path
        if not full_path.exists():
            continue

        notes_text = full_path.read_text()
        if not notes_text.strip():
            continue

        # Reuse existing summary if available, otherwise heuristic
        summary_text = info.get("summary_cache")
        if summary_text:
            try:
                summary = json.loads(summary_text)
            except json.JSONDecodeError:
                summary = heuristic_summary(notes_text, session_id)
        else:
            summary = heuristic_summary(notes_text, session_id)

        doc_ids = asyncio.run(ingest_to_memory(session_id, notes_path, summary))
        if doc_ids:
            now = datetime.now(KST).isoformat()
            info["ingested_at"] = now
            info["doc_ids"] = doc_ids
            state["pending_ingest"].remove(session_id)
            success += 1

    return success


# -- main ----------------------------------------------------------


def main():
    parser = argparse.ArgumentParser(
        description="Summarize harvested session transcripts with Gemini Flash -> store in rin-memory",
    )
    parser.add_argument(
        "--all",
        action="store_true",
        help="Remove batch limit (10), process all unsummarized sessions",
    )
    args = parser.parse_args()

    state = load_state()

    # Retry pending_ingest
    retried = retry_pending(state)
    if retried:
        print(f"Pending retry: {retried} succeeded")

    # Find unsummarized sessions
    unsummarized = find_unsummarized(state)
    if not unsummarized:
        print("Summarized 0/0 sessions")
        state["last_run"] = datetime.now(KST).isoformat()
        save_state(state)
        return

    limit = len(unsummarized) if args.all else BATCH_LIMIT
    targets = unsummarized[:limit]

    success = 0
    for session_id, info in targets:
        print(f"  [{session_id[:8]}] Summarizing...")
        if process_one(session_id, info, state):
            method = state["processed"][session_id].get("summary_method", "?")
            doc_ids = state["processed"][session_id].get("doc_ids", [])
            status = f"method={method}"
            if doc_ids:
                status += f", docs={len(doc_ids)}"
            else:
                status += ", ingest=pending"
            print(f"  [{session_id[:8]}] OK ({status})")
            success += 1
        else:
            print(f"  [{session_id[:8]}] FAIL")

    state["last_run"] = datetime.now(KST).isoformat()
    save_state(state)

    total = len(targets)
    print(f"Summarized {success}/{total} sessions")


if __name__ == "__main__":
    main()
