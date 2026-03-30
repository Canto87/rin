"""Tests for rin-memory-recall.py — session start memory loading."""

import os
import sqlite3
import subprocess
import sys

RECALL_SCRIPT = os.path.join(
    os.path.dirname(__file__), "..", "scripts", "rin-memory-recall.py"
)

SCHEMA = """\
CREATE TABLE IF NOT EXISTS documents (
    id TEXT PRIMARY KEY, kind TEXT NOT NULL, title TEXT NOT NULL,
    content TEXT NOT NULL, summary TEXT, tags TEXT, source TEXT,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT, archived INTEGER DEFAULT 0, project TEXT
);
CREATE TABLE IF NOT EXISTS metadata (key TEXT PRIMARY KEY, value TEXT NOT NULL);
INSERT INTO metadata (key, value) VALUES ('schema_version', '2');
"""


def _setup_db(tmp_path, rows):
    """Create a memory.db with given rows and return the dir path."""
    db_path = str(tmp_path / "memory.db")
    db = sqlite3.connect(db_path)
    db.executescript(SCHEMA)
    for r in rows:
        db.execute(
            "INSERT INTO documents (id, kind, title, content, summary, project, archived, created_at) "
            "VALUES (?, ?, ?, ?, ?, ?, ?, datetime('now'))",
            (r["id"], r["kind"], r["title"], r["content"], r.get("summary"), r.get("project"), r.get("archived", 0)),
        )
    db.commit()
    db.close()
    return str(tmp_path)


def _run_recall(memory_dir, project=None):
    """Run recall script and return stdout."""
    env = {**os.environ, "RIN_MEMORY_DIR": memory_dir}
    if project:
        env["RIN_PROJECT"] = project
    else:
        env.pop("RIN_PROJECT", None)
    result = subprocess.run(
        [sys.executable, RECALL_SCRIPT],
        capture_output=True, text=True, env=env,
    )
    return result.stdout, result.returncode


# ── Active Tasks ─────────────────────────────────────────────


def test_active_tasks_displayed(tmp_path):
    """Active tasks should appear at the top of recall output."""
    mem_dir = _setup_db(tmp_path, [
        {"id": "task_abc123", "kind": "active_task", "title": "Phase 3 implementation", "content": "Current status: half done. Remaining: write tests"},
    ])
    out, rc = _run_recall(mem_dir)
    assert rc == 0
    assert "### Active Tasks (unfinished)" in out
    assert "[task_abc123]" in out
    assert "**Phase 3 implementation**" in out


def test_archived_active_tasks_excluded(tmp_path):
    """Archived active tasks should not appear."""
    mem_dir = _setup_db(tmp_path, [
        {"id": "done1", "kind": "active_task", "title": "Completed task", "content": "done", "archived": 1},
    ])
    out, _ = _run_recall(mem_dir)
    # No active tasks → no section header
    assert "Active Tasks" not in out


def test_active_tasks_limit_3(tmp_path):
    """Only the 3 most recent active tasks should appear."""
    mem_dir = _setup_db(tmp_path, [
        {"id": f"t{i}", "kind": "active_task", "title": f"Task {i}", "content": f"content {i}"}
        for i in range(5)
    ])
    out, _ = _run_recall(mem_dir)
    # Should have exactly 3 task lines
    task_lines = [l for l in out.splitlines() if l.startswith("- [t")]
    assert len(task_lines) == 3


# ── Preferences ──────────────────────────────────────────────


def test_preferences_displayed(tmp_path):
    """Preferences should appear after Team Patterns."""
    mem_dir = _setup_db(tmp_path, [
        {"id": "pref1", "kind": "preference", "title": "Code style: concise variable names", "content": "Operator prefers short variable names"},
    ])
    out, rc = _run_recall(mem_dir)
    assert rc == 0
    assert "### Operator Preferences" in out
    assert "**Code style: concise variable names**" in out


# ── Project filtering ────────────────────────────────────────


def test_project_filter_active_tasks(tmp_path):
    """Active tasks should respect project filtering."""
    mem_dir = _setup_db(tmp_path, [
        {"id": "t_a", "kind": "active_task", "title": "Project A task", "content": "a", "project": "proj_a"},
        {"id": "t_b", "kind": "active_task", "title": "Project B task", "content": "b", "project": "proj_b"},
        {"id": "t_g", "kind": "active_task", "title": "Global task", "content": "g", "project": None},
    ])
    out, _ = _run_recall(mem_dir, project="proj_a")
    assert "Project A task" in out
    assert "Global task" in out      # NULL project visible everywhere
    assert "Project B task" not in out


# ── DB close before routing queries (regression) ─────────────


def test_routing_logs_after_all_queries(tmp_path):
    """Routing logs should work (db.close() must be after all queries)."""
    import json
    mem_dir = _setup_db(tmp_path, [
        {"id": "r1", "kind": "routing_log", "title": "log", "content": json.dumps({"model": "glm-5", "success": True})},
        {"id": "t1", "kind": "active_task", "title": "Task", "content": "wip"},
        {"id": "p1", "kind": "preference", "title": "Pref", "content": "desc"},
    ])
    out, rc = _run_recall(mem_dir)
    assert rc == 0
    # All three sections should be present
    assert "Active Tasks" in out
    assert "Preferences" in out
    assert "Routing Stats" in out


# ── Empty DB ──────────────────────────────────────────────────


def test_empty_db_exits_silently(tmp_path):
    """DB with no documents should produce no output."""
    mem_dir = _setup_db(tmp_path, [])
    out, rc = _run_recall(mem_dir)
    assert out == ""
    assert rc == 0
