-- rin-memory PostgreSQL schema
-- Required extensions: vector, pg_trgm, age

CREATE EXTENSION IF NOT EXISTS vector;
CREATE EXTENSION IF NOT EXISTS pg_trgm;
CREATE EXTENSION IF NOT EXISTS age;
LOAD 'age';

-- Documents table
CREATE TABLE IF NOT EXISTS documents (
    id          TEXT PRIMARY KEY,
    kind        TEXT NOT NULL,
    title       TEXT NOT NULL,
    content     TEXT NOT NULL,
    summary     TEXT,
    tags        TEXT[],
    source      TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ,
    archived    BOOLEAN NOT NULL DEFAULT FALSE,
    project     TEXT,
    tsv         TSVECTOR
);

-- Document vectors (pgvector)
CREATE TABLE IF NOT EXISTS document_vectors (
    id          BIGSERIAL PRIMARY KEY,
    doc_id      TEXT NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    chunk_index INTEGER NOT NULL,
    embedding   vector(1024),
    UNIQUE(doc_id, chunk_index)
);

-- Relations between documents
CREATE TABLE IF NOT EXISTS relations (
    from_id     TEXT NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    to_id       TEXT NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    rel_type    TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (from_id, to_id)
);

-- Indexes
CREATE INDEX IF NOT EXISTS idx_docs_kind ON documents(kind) WHERE NOT archived;
CREATE INDEX IF NOT EXISTS idx_docs_project ON documents(project) WHERE NOT archived;
CREATE INDEX IF NOT EXISTS idx_docs_tags ON documents USING GIN(tags);
CREATE INDEX IF NOT EXISTS idx_docs_tsv ON documents USING GIN(tsv);
CREATE INDEX IF NOT EXISTS idx_docs_trgm ON documents USING GIN(title gin_trgm_ops);
CREATE INDEX IF NOT EXISTS idx_vectors_hnsw ON document_vectors
    USING hnsw (embedding vector_cosine_ops) WITH (m=16, ef_construction=64);

-- AGE graph
SELECT ag_catalog.create_graph('rin_memory');
