"""Rebuild LanceDB vectors from SQLite documents.

Usage: python -m rin_memory.rebuild_vectors
"""

import asyncio
import json
import logging
import shutil
import sqlite3
from pathlib import Path

logging.basicConfig(level=logging.INFO, format="%(message)s")
log = logging.getLogger(__name__)

DATA_DIR = Path.home() / ".rin"
DB_PATH = DATA_DIR / "memory.db"
LANCE_PATH = DATA_DIR / "vectors"


async def rebuild():
    import lancedb

    from .chunking import chunk_document
    from .embedding import embed_batch

    # 1. Read all active documents from SQLite
    db = sqlite3.connect(str(DB_PATH))
    db.row_factory = sqlite3.Row
    rows = db.execute(
        "SELECT id, title, content FROM documents WHERE archived = 0"
    ).fetchall()
    db.close()
    log.info("Found %d active documents", len(rows))

    # 2. Remove old LanceDB data
    lance_dir = LANCE_PATH / "vectors.lance"
    if lance_dir.exists():
        shutil.rmtree(lance_dir)
        log.info("Removed old vectors.lance")

    # 3. Rebuild
    lance_db = lancedb.connect(str(LANCE_PATH))
    table = None
    total_chunks = 0
    errors = 0

    for i, row in enumerate(rows):
        doc_id = row["id"]
        try:
            chunks = chunk_document(row["title"], row["content"])
            texts = [c["text"] for c in chunks]
            vectors = await embed_batch(texts)
            data = [
                {"doc_id": doc_id, "chunk_index": c["chunk_index"], "vector": v}
                for c, v in zip(chunks, vectors)
            ]

            if table is None:
                table = lance_db.create_table("vectors", data=data)
            else:
                table.add(data)

            total_chunks += len(chunks)
        except Exception as e:
            errors += 1
            log.warning("  [%d/%d] FAIL %s: %s", i + 1, len(rows), doc_id, e)
            continue

        if (i + 1) % 50 == 0:
            log.info("  [%d/%d] %d chunks so far", i + 1, len(rows), total_chunks)

    log.info("Done: %d docs → %d chunks (%d errors)", len(rows), total_chunks, errors)


if __name__ == "__main__":
    asyncio.run(rebuild())
