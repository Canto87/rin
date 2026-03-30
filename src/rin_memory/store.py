"""Data access layer combining SQLite (structured) + LanceDB (vectors)."""

import json
import logging
import uuid
from datetime import datetime, timezone

log = logging.getLogger(__name__)

import aiosqlite
import lancedb

from .chunking import chunk_document
from .embedding import embed, embed_batch
from .schema import init_db

_RRF_K = 60  # RRF smoothing constant


class MemoryStore:
    """Unified store: SQLite for structured data, LanceDB for vector search."""

    def __init__(self, db_path: str, lance_path: str):
        self.db_path = db_path
        self.lance_path = lance_path
        self._sqlite: aiosqlite.Connection | None = None
        self._lance_db = None
        self._vectors = None

    async def initialize(self):
        self._sqlite = await init_db(self.db_path)
        self._lance_db = lancedb.connect(self.lance_path)
        try:
            self._vectors = self._lance_db.open_table("vectors")
        except Exception:
            self._vectors = None  # Created on first insert

    async def close(self):
        if self._sqlite:
            await self._sqlite.close()

    # ── store ────────────────────────────────────────────────────────

    async def store_document(
        self,
        kind: str,
        title: str,
        content: str,
        tags: list[str] | None = None,
        source: str | None = None,
        summary: str | None = None,
        project: str | None = None,
    ) -> str:
        doc_id = uuid.uuid4().hex[:12]
        now = datetime.now(timezone.utc).isoformat()

        await self._sqlite.execute(
            "INSERT INTO documents (id, kind, title, content, summary, tags, source, created_at, project)"
            " VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)",
            (
                doc_id,
                kind,
                title,
                content,
                summary,
                json.dumps(tags or []),
                source,
                now,
                project,
            ),
        )
        await self._sqlite.commit()

        try:
            chunks = chunk_document(title, content)
            texts = [c["text"] for c in chunks]
            vectors = await embed_batch(texts)
            self._upsert_vectors(doc_id, chunks, vectors)
        except Exception as e:
            log.warning(
                "store_document: embedding failed for %s, document saved without vector: %s",
                doc_id,
                e,
            )

        return doc_id

    # ── search (hybrid: vector + FTS) ─────────────────────────────

    async def search(
        self,
        query: str,
        kind: str | None = None,
        project: str | None = None,
        limit: int = 5,
        detail: str = "summary",
    ) -> list[dict]:
        query_vector = await embed(query)

        # Run both searches
        vec_ranking = self._vector_search(query_vector, limit * 3)
        fts_ranking = await self._fts_search(query, kind, project, limit * 3)

        # Merge with RRF
        merged = self._rrf_merge(vec_ranking, fts_ranking)

        if not merged:
            return []

        # Take top candidates
        top_ids = [doc_id for doc_id, _ in sorted(merged.items(), key=lambda x: x[1], reverse=True)][:limit * 2]

        # Hydrate from SQLite
        return await self._hydrate_results(top_ids, kind, project, limit, detail, merged)

    def _vector_search(self, query_vector: list[float], fetch_limit: int) -> dict[str, int]:
        """Vector search returning {doc_id: rank} (1-based, lower = better)."""
        if self._vectors is None:
            return {}

        try:
            results = self._vectors.search(query_vector).limit(fetch_limit).to_list()
        except Exception as e:
            log.warning("_vector_search failed: %s", e)
            return {}

        if not results:
            return {}

        # Group by doc_id, take best (min distance) chunk per doc
        best: dict[str, float] = {}
        for r in results:
            doc_id = r["doc_id"]
            dist = r["_distance"]
            if doc_id not in best or dist < best[doc_id]:
                best[doc_id] = dist

        # Convert to ranking (1 = best)
        sorted_ids = sorted(best, key=lambda d: best[d])
        return {doc_id: rank + 1 for rank, doc_id in enumerate(sorted_ids)}

    async def _fts_search(
        self,
        query: str,
        kind: str | None,
        project: str | None,
        fetch_limit: int,
    ) -> dict[str, int]:
        """FTS5 keyword search returning {doc_id: rank}.

        Trigram tokenizer cannot match tokens < 3 chars, so short tokens
        fall back to SQL LIKE on the documents table directly.
        """
        tokens = [t.replace('"', '""') for t in query.split() if len(t.strip()) >= 2]
        if not tokens:
            return {}

        fts_tokens = [t for t in tokens if len(t) >= 3]
        short_tokens = [t for t in tokens if len(t) < 3]

        ranking: dict[str, int] = {}
        rank_counter = 1

        # 1) FTS MATCH for tokens >= 3 chars (trigram works)
        if fts_tokens and await self._has_fts():
            match_expr = " OR ".join(f'"{t}"' for t in fts_tokens)
            sql = (
                "SELECT d.id, bm25(documents_fts) AS score"
                " FROM documents_fts f"
                " JOIN documents d ON d.rowid = f.rowid"
                f" WHERE documents_fts MATCH '{match_expr}'"
                " AND d.archived = 0"
            )
            params: list = []
            if kind:
                sql += " AND d.kind = ?"
                params.append(kind)
            if project:
                sql += " AND (d.project = ? OR d.project IS NULL)"
                params.append(project)
            sql += " ORDER BY score LIMIT ?"
            params.append(fetch_limit)

            try:
                cursor = await self._sqlite.execute(sql, params)
                for row in await cursor.fetchall():
                    ranking[row["id"]] = rank_counter
                    rank_counter += 1
            except Exception as e:
                log.warning("FTS search failed (query=%r): %s", query, e)

        # 2) LIKE fallback for tokens < 3 chars (trigram can't match these)
        if short_tokens:
            like_clauses = []
            like_params: list = []
            for t in short_tokens:
                like_clauses.append("(d.title LIKE ? OR d.content LIKE ?)")
                like_params.extend([f"%{t}%", f"%{t}%"])

            sql = (
                "SELECT d.id FROM documents d"
                f" WHERE d.archived = 0 AND ({' OR '.join(like_clauses)})"
            )
            if kind:
                sql += " AND d.kind = ?"
                like_params.append(kind)
            if project:
                sql += " AND (d.project = ? OR d.project IS NULL)"
                like_params.append(project)
            sql += " ORDER BY d.created_at DESC LIMIT ?"
            like_params.append(fetch_limit)

            try:
                cursor = await self._sqlite.execute(sql, like_params)
                for row in await cursor.fetchall():
                    if row["id"] not in ranking:
                        ranking[row["id"]] = rank_counter
                        rank_counter += 1
            except Exception as e:
                log.warning("LIKE fallback failed (query=%r): %s", query, e)

        return ranking

    async def _has_fts(self) -> bool:
        """Check if FTS5 table exists."""
        try:
            cursor = await self._sqlite.execute(
                "SELECT name FROM sqlite_master WHERE type='table' AND name='documents_fts'"
            )
            return await cursor.fetchone() is not None
        except Exception:
            return False

    @staticmethod
    def _rrf_merge(vec_ranking: dict[str, int], fts_ranking: dict[str, int]) -> dict[str, float]:
        """Reciprocal Rank Fusion: combine two rankings into unified scores."""
        scores: dict[str, float] = {}
        all_ids = set(vec_ranking) | set(fts_ranking)

        for doc_id in all_ids:
            score = 0.0
            if doc_id in vec_ranking:
                score += 1.0 / (_RRF_K + vec_ranking[doc_id])
            if doc_id in fts_ranking:
                score += 1.0 / (_RRF_K + fts_ranking[doc_id])
            scores[doc_id] = score

        return scores

    async def _hydrate_results(
        self,
        doc_ids: list[str],
        kind: str | None,
        project: str | None,
        limit: int,
        detail: str,
        scores: dict[str, float],
    ) -> list[dict]:
        """Fetch full documents from SQLite and attach relevance scores."""
        if not doc_ids:
            return []

        placeholders = ",".join("?" * len(doc_ids))
        sql = f"SELECT * FROM documents WHERE id IN ({placeholders}) AND archived = 0"
        params: list = list(doc_ids)

        if kind:
            sql += " AND kind = ?"
            params.append(kind)
        if project:
            sql += " AND (project = ? OR project IS NULL)"
            params.append(project)

        cursor = await self._sqlite.execute(sql, params)
        rows = await cursor.fetchall()

        docs = []
        for row in rows:
            doc = self._format_document(row, detail)
            doc["relevance"] = round(scores.get(row["id"], 0.0), 4)
            docs.append(doc)

        docs.sort(key=lambda d: d["relevance"], reverse=True)
        return docs[:limit]

    # ── get by id ──────────────────────────────────────────────────────

    async def get_by_id(self, doc_id: str, detail: str = "full") -> dict | None:
        cursor = await self._sqlite.execute(
            "SELECT * FROM documents WHERE id = ? AND archived = 0", (doc_id,)
        )
        row = await cursor.fetchone()
        return self._format_document(row, detail) if row else None

    # ── lookup (structured filter) ───────────────────────────────────

    async def lookup(
        self,
        kind: str | None = None,
        tags: list[str] | None = None,
        project: str | None = None,
        limit: int = 10,
        detail: str = "summary",
    ) -> list[dict]:
        conditions = ["archived = 0"]
        params: list = []

        if kind:
            conditions.append("kind = ?")
            params.append(kind)

        if project:
            conditions.append("(project = ? OR project IS NULL)")
            params.append(project)

        where = " AND ".join(conditions)
        cursor = await self._sqlite.execute(
            f"SELECT * FROM documents WHERE {where} ORDER BY created_at DESC LIMIT ?",
            params + [limit * 2 if tags else limit],
        )
        rows = await cursor.fetchall()

        docs = []
        for row in rows:
            if tags:
                doc_tags = json.loads(row["tags"] or "[]")
                if not any(t in doc_tags for t in tags):
                    continue
            docs.append(self._format_document(row, detail))

        return docs[:limit]

    # ── update ───────────────────────────────────────────────────────

    async def update_document(
        self,
        doc_id: str,
        content: str | None = None,
        title: str | None = None,
        tags: list[str] | None = None,
        archive: bool | None = None,
    ) -> bool:
        cursor = await self._sqlite.execute(
            "SELECT id FROM documents WHERE id = ?", (doc_id,)
        )
        if not await cursor.fetchone():
            return False

        now = datetime.now(timezone.utc).isoformat()
        updates = ["updated_at = ?"]
        params: list = [now]

        if content is not None:
            updates.append("content = ?")
            params.append(content)
        if title is not None:
            updates.append("title = ?")
            params.append(title)
        if tags is not None:
            updates.append("tags = ?")
            params.append(json.dumps(tags))
        if archive is not None:
            updates.append("archived = ?")
            params.append(1 if archive else 0)

        params.append(doc_id)
        await self._sqlite.execute(
            f"UPDATE documents SET {', '.join(updates)} WHERE id = ?", params
        )
        await self._sqlite.commit()

        # Re-embed if content or title changed
        if content is not None or title is not None:
            cursor = await self._sqlite.execute(
                "SELECT title, content FROM documents WHERE id = ?", (doc_id,)
            )
            row = await cursor.fetchone()
            self._delete_vectors(doc_id)
            try:
                chunks = chunk_document(row["title"], row["content"])
                texts = [c["text"] for c in chunks]
                vectors = await embed_batch(texts)
                self._upsert_vectors(doc_id, chunks, vectors)
            except Exception as e:
                log.warning("update_document: re-embedding failed for %s: %s", doc_id, e)

        return True

    # ── relate ───────────────────────────────────────────────────────

    async def add_relation(self, from_id: str, to_id: str, rel_type: str) -> bool:
        try:
            await self._sqlite.execute(
                "INSERT OR REPLACE INTO relations (from_id, to_id, type) VALUES (?, ?, ?)",
                (from_id, to_id, rel_type),
            )
            await self._sqlite.commit()
            return True
        except Exception:
            return False

    async def get_relations(self, doc_id: str) -> list[dict]:
        cursor = await self._sqlite.execute(
            "SELECT r.from_id, r.to_id, r.type, d.title, d.kind"
            " FROM relations r"
            " JOIN documents d ON d.id = CASE WHEN r.from_id = ? THEN r.to_id ELSE r.from_id END"
            " WHERE r.from_id = ? OR r.to_id = ?",
            (doc_id, doc_id, doc_id),
        )
        rows = await cursor.fetchall()
        return [
            {
                "from_id": row["from_id"],
                "to_id": row["to_id"],
                "type": row["type"],
                "related_title": row["title"],
                "related_kind": row["kind"],
            }
            for row in rows
        ]

    # ── vector helpers ───────────────────────────────────────────────

    def _upsert_vectors(self, doc_id: str, chunks: list[dict], vectors: list[list[float]]):
        """Store chunk-level vectors for a document."""
        data = [
            {"doc_id": doc_id, "chunk_index": c["chunk_index"], "vector": v}
            for c, v in zip(chunks, vectors)
        ]
        if self._vectors is None:
            try:
                self._vectors = self._lance_db.open_table("vectors")
            except Exception:
                self._vectors = self._lance_db.create_table("vectors", data=data)
                return

        try:
            self._vectors.add(data)
        except Exception:
            # Stale handle (e.g. table rebuilt by another process) — reopen and retry
            try:
                self._vectors = self._lance_db.open_table("vectors")
                self._vectors.add(data)
            except Exception as e:
                log.warning("_upsert_vectors failed after reopen: %s", e)
                self._vectors = None

    def _delete_vectors(self, doc_id: str):
        """Delete all chunk vectors for a document."""
        if self._vectors is not None:
            self._vectors.delete(f"doc_id = '{doc_id}'")

    # ── progressive disclosure ───────────────────────────────────────

    @staticmethod
    def _format_document(row, detail: str = "summary") -> dict:
        """Format a document row with progressive detail levels.

        summary: id, kind, title, tags, created_at, project  (~minimal tokens)
        detail:  + source, summary, updated_at
        full:    + content
        """
        doc = {
            "id": row["id"],
            "kind": row["kind"],
            "title": row["title"],
            "tags": json.loads(row["tags"] or "[]"),
            "created_at": row["created_at"],
            "project": row["project"],
        }

        if detail in ("detail", "full"):
            doc["source"] = row["source"]
            doc["summary"] = row["summary"]
            doc["updated_at"] = row["updated_at"]

        if detail == "full":
            doc["content"] = row["content"]

        return doc
