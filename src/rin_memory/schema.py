"""SQLite schema for rin-memory."""

SCHEMA_VERSION = 3

DDL = """\
CREATE TABLE IF NOT EXISTS documents (
    id          TEXT PRIMARY KEY,
    kind        TEXT NOT NULL,
    title       TEXT NOT NULL,
    content     TEXT NOT NULL,
    summary     TEXT,
    tags        TEXT,
    source      TEXT,
    created_at  TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at  TEXT,
    archived    INTEGER DEFAULT 0,
    project     TEXT
);

CREATE TABLE IF NOT EXISTS relations (
    from_id  TEXT NOT NULL REFERENCES documents(id),
    to_id    TEXT NOT NULL REFERENCES documents(id),
    type     TEXT NOT NULL,
    PRIMARY KEY (from_id, to_id)
);

CREATE TABLE IF NOT EXISTS metadata (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_documents_kind ON documents(kind);
CREATE INDEX IF NOT EXISTS idx_documents_tags ON documents(tags);
CREATE INDEX IF NOT EXISTS idx_relations_from ON relations(from_id);
CREATE INDEX IF NOT EXISTS idx_relations_to ON relations(to_id);
"""

_FTS5_DDL = """\
CREATE VIRTUAL TABLE IF NOT EXISTS documents_fts USING fts5(
    title, content, tags,
    content='documents', content_rowid='rowid',
    tokenize='trigram'
);
"""

_FTS5_FALLBACK_DDL = """\
CREATE VIRTUAL TABLE IF NOT EXISTS documents_fts USING fts5(
    title, content, tags,
    content='documents', content_rowid='rowid',
    tokenize='unicode61'
);
"""

_FTS5_TRIGGERS = [
    # Auto-sync on INSERT
    """CREATE TRIGGER IF NOT EXISTS documents_ai AFTER INSERT ON documents BEGIN
        INSERT INTO documents_fts(rowid, title, content, tags)
        VALUES (new.rowid, new.title, new.content, new.tags);
    END""",
    # Auto-sync on DELETE
    """CREATE TRIGGER IF NOT EXISTS documents_ad AFTER DELETE ON documents BEGIN
        INSERT INTO documents_fts(documents_fts, rowid, title, content, tags)
        VALUES ('delete', old.rowid, old.title, old.content, old.tags);
    END""",
    # Auto-sync on UPDATE
    """CREATE TRIGGER IF NOT EXISTS documents_au AFTER UPDATE ON documents BEGIN
        INSERT INTO documents_fts(documents_fts, rowid, title, content, tags)
        VALUES ('delete', old.rowid, old.title, old.content, old.tags);
        INSERT INTO documents_fts(rowid, title, content, tags)
        VALUES (new.rowid, new.title, new.content, new.tags);
    END""",
]

MIGRATIONS = {
    2: [
        "ALTER TABLE documents ADD COLUMN project TEXT",
        "CREATE INDEX IF NOT EXISTS idx_documents_project ON documents(project)",
    ],
}


async def init_db(db_path: str):
    """Initialize SQLite database with schema and apply pending migrations."""
    import aiosqlite

    db = await aiosqlite.connect(db_path)
    db.row_factory = aiosqlite.Row
    await db.executescript(DDL)

    # Check current version and apply migrations
    cursor = await db.execute("SELECT value FROM metadata WHERE key = 'schema_version'")
    row = await cursor.fetchone()
    current = int(row["value"]) if row else 1

    for ver in sorted(MIGRATIONS):
        if current < ver:
            for sql in MIGRATIONS[ver]:
                try:
                    await db.execute(sql)
                except Exception:
                    pass  # Column may already exist
            current = ver

    # v3: FTS5 full-text search
    if current < 3:
        await _setup_fts5(db)
        current = 3

    await db.execute(
        "INSERT OR REPLACE INTO metadata (key, value) VALUES (?, ?)",
        ("schema_version", str(SCHEMA_VERSION)),
    )
    await db.commit()
    return db


async def _setup_fts5(db):
    """Create FTS5 virtual table with trigram tokenizer, fallback to unicode61."""
    # Drop existing FTS table if any (clean re-creation)
    try:
        await db.execute("DROP TABLE IF EXISTS documents_fts")
    except Exception:
        pass

    # Try trigram first (better for Korean substring matching)
    try:
        await db.executescript(_FTS5_DDL)
    except Exception:
        await db.executescript(_FTS5_FALLBACK_DDL)

    # Create sync triggers
    for trigger_sql in _FTS5_TRIGGERS:
        await db.execute(trigger_sql)

    # Populate FTS from existing data
    await db.execute(
        "INSERT INTO documents_fts(rowid, title, content, tags) "
        "SELECT rowid, title, content, tags FROM documents"
    )
    await db.commit()
