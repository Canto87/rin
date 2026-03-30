#!/usr/bin/env python3
"""Session picker for RIN startup.

Shows recent sessions and lets user choose:
  Enter  → New session
  Number → Select → resume or context load

Output (stdout, one line):
  "new"                → New session
  "resume:<uuid>"      → Resume session (--resume)
  "context:<source>"   → Load context only (RIN_LOAD_SESSION)

UI output goes to stderr (redirected to /dev/tty by caller).
Requires only stdlib (sqlite3, json, os, pathlib).
"""

import json
import os
import sys
import time
from datetime import datetime, timedelta, timezone
from pathlib import Path

# ── Constants ──────────────────────────────────────────

RIN_HOME = Path(os.environ.get("RIN_HOME", Path(__file__).resolve().parent.parent))
HARVEST_STATE = RIN_HOME / "memory" / ".harvest-state.json"
CLAUDE_PROJECTS_DIR = Path.home() / ".claude" / "projects"
PROJECT_SLUG = os.environ.get("RIN_PROJECT", "")

KST = timezone(timedelta(hours=9))
MAX_SESSIONS = 20
ACTIVE_THRESHOLD_SEC = 300  # 5 minutes
MIN_JSONL_SIZE = 5000  # Skip tiny files

# ── ANSI ───────────────────────────────────────────────

CYAN = "\033[36m"
DIM = "\033[2m"
WHITE = "\033[1;37m"
GREEN = "\033[32m"
BOLD = "\033[1m"
RESET = "\033[0m"


def eprint(*args, **kwargs):
    """Print to stderr (UI display)."""
    print(*args, file=sys.stderr, **kwargs)


def human_size(n: int) -> str:
    """Bytes → human-readable string."""
    v = float(n)
    for unit in ("B", "K", "MB", "GB"):
        if abs(v) < 1024:
            return f"{v:.0f}{unit}" if (v >= 10 or unit == "B") else f"{v:.1f}{unit}"
        v /= 1024
    return f"{v:.0f}TB"


# ── Data Sources ───────────────────────────────────────


def get_jsonl_files() -> dict[str, dict]:
    """UUID → {path, mtime, size} for current project's JSONL files."""
    if not PROJECT_SLUG:
        return {}
    project_dir = CLAUDE_PROJECTS_DIR / PROJECT_SLUG
    if not project_dir.is_dir():
        return {}

    result = {}
    for p in project_dir.glob("*.jsonl"):
        try:
            st = p.stat()
        except OSError:
            continue
        if st.st_size < MIN_JSONL_SIZE:
            continue
        result[p.stem] = {
            "path": p,
            "mtime": st.st_mtime,
            "size": st.st_size,
        }
    return result


def get_harvest_state() -> dict:
    """UUID → harvest info from .harvest-state.json."""
    if not HARVEST_STATE.exists():
        return {}
    try:
        return json.loads(HARVEST_STATE.read_text()).get("processed", {})
    except (json.JSONDecodeError, OSError):
        return {}


def get_renamed_slug(jsonl_path: Path) -> str | None:
    """Compare first and last slug in JSONL; return last slug if renamed."""
    try:
        first_slug = None
        # First slug: only first 20 lines
        with open(jsonl_path) as f:
            for i, line in enumerate(f):
                if i >= 20:
                    break
                obj = json.loads(line)
                s = obj.get("slug")
                if s:
                    first_slug = s
                    break
        if not first_slug:
            return None
        # Last slug: read last 8KB
        size = jsonl_path.stat().st_size
        last_slug = None
        with open(jsonl_path, "rb") as f:
            f.seek(max(0, size - 8192))
            tail = f.read().decode("utf-8", errors="ignore")
        for line in reversed(tail.splitlines()):
            try:
                obj = json.loads(line)
                s = obj.get("slug")
                if s:
                    last_slug = s
                    break
            except (json.JSONDecodeError, ValueError):
                continue
        if last_slug and first_slug != last_slug:
            return last_slug.replace("-", " ")
    except Exception:
        pass
    return None


def is_teammate_session(jsonl_path: Path) -> bool:
    """Read first few lines of JSONL to determine if it's a teammate session."""
    try:
        with open(jsonl_path) as f:
            for line in f:
                obj = json.loads(line)
                if obj.get("type") != "user":
                    continue
                msg = obj.get("message", {})
                content = msg.get("content", "") if isinstance(msg, dict) else ""
                if isinstance(content, list):
                    for c in content:
                        if isinstance(c, dict) and c.get("type") == "text":
                            if "<teammate-message" in c.get("text", ""):
                                return True
                            return False
                elif isinstance(content, str):
                    if "<teammate-message" in content:
                        return True
                    return False
    except Exception:
        pass
    return False


# ── Matching ───────────────────────────────────────────


def build_session_list() -> list[dict]:
    """Build combined session list from JSONL files + DB entries."""
    jsonl_files = get_jsonl_files()
    harvest = get_harvest_state()

    now = time.time()

    # Sort JSONL by mtime desc (most recently used first)
    # Titles are looked up directly from harvest-state by UUID, so sort changes don't mix titles
    sorted_jsonl = sorted(
        jsonl_files.items(),
        key=lambda x: x[1]["mtime"],
        reverse=True,
    )

    sessions = []

    for uuid, jinfo in sorted_jsonl:
        if len(sessions) >= MAX_SESSIONS:
            break

        mtime_dt = datetime.fromtimestamp(jinfo["mtime"], tz=KST)
        is_active = (now - jinfo["mtime"]) < ACTIVE_THRESHOLD_SEC

        # Look up title directly from harvest-state (UUID-based, accurate)
        h_info = harvest.get(uuid, {})
        title = h_info.get("title")

        # Hide teammate sessions
        if h_info and title is None:
            continue
        if not h_info and is_teammate_session(jinfo["path"]):
            continue

        # Renamed slug takes highest priority as title
        renamed = get_renamed_slug(jinfo["path"])
        if renamed:
            title = renamed

        sessions.append(
            {
                "uuid": uuid,
                "title": title,
                "date": mtime_dt,
                "size": jinfo["size"],
                "active": is_active,
                "can_resume": True,
            }
        )

    return sessions


# ── Display ────────────────────────────────────────────


def display_sessions(sessions: list[dict]) -> None:
    """Display session list to stderr."""
    eprint(f"\n  {WHITE}RIN Sessions{RESET}\n")

    if not sessions:
        eprint(f"  {DIM}No recent sessions{RESET}\n")
        return

    for i, s in enumerate(sessions, 1):
        date_str = s["date"].strftime("%m-%d %H:%M")
        title = s["title"] or f"(session {s['uuid'][:8]}...)"

        # Status
        if s["active"]:
            status = f"  {GREEN}(active){RESET}"
        elif s["size"]:
            status = f"  {DIM}{human_size(s['size'])}{RESET}"
        else:
            status = f"  {DIM}(context only){RESET}"

        eprint(
            f"  {DIM}[{RESET}{BOLD}{i}{RESET}{DIM}]{RESET}"
            f" {date_str}  {title}{status}"
        )

    eprint("")


# ── User Input ─────────────────────────────────────────


def prompt_choice(sessions: list[dict]) -> str:
    """Get user choice. Returns protocol string for stdout."""
    eprint(f"  Select number {DIM}(Enter = new session){RESET}: ", end="")
    sys.stderr.flush()

    try:
        tty = open("/dev/tty", "r")
        choice = tty.readline().strip()
        tty.close()
    except (EOFError, KeyboardInterrupt, OSError):
        return "new"

    if not choice:
        return "new"

    try:
        idx = int(choice) - 1
    except ValueError:
        return "new"

    if idx < 0 or idx >= len(sessions):
        eprint(f"  {DIM}Invalid number{RESET}")
        return "new"

    s = sessions[idx]
    title = s["title"] or f"session {s['uuid'][:8]}..."

    eprint(f"  {CYAN}→{RESET} {title}")
    return f"resume:{s['uuid']}"


# ── Main ───────────────────────────────────────────────


def main():
    sessions = build_session_list()
    display_sessions(sessions)
    result = prompt_choice(sessions)
    print(result)  # stdout — captured by shell


if __name__ == "__main__":
    main()
