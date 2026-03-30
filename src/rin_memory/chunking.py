"""Document chunking for fine-grained vector search."""

import re

_MAX_CHUNK = 1200
_OVERLAP = 100


def chunk_document(title: str, content: str) -> list[dict]:
    """Split a document into chunks for vector embedding.

    Each chunk gets a title prefix for context. Returns list of
    {"chunk_index": int, "text": str}.

    Strategy:
    - Short documents (<=_MAX_CHUNK chars): single chunk
    - Markdown headings (## / ###): split by section
    - Long sections: split by paragraph boundary with overlap
    """
    full = f"{title}\n{content}"
    if len(full) <= _MAX_CHUNK:
        return [{"chunk_index": 0, "text": full}]

    sections = _split_by_headings(content)
    chunks: list[dict] = []
    idx = 0

    for section in sections:
        text = f"{title}\n{section}"
        if len(text) <= _MAX_CHUNK:
            chunks.append({"chunk_index": idx, "text": text})
            idx += 1
        else:
            for part in _split_by_paragraphs(section):
                chunks.append({"chunk_index": idx, "text": f"{title}\n{part}"})
                idx += 1

    return chunks if chunks else [{"chunk_index": 0, "text": full[:_MAX_CHUNK]}]


def _split_by_headings(content: str) -> list[str]:
    """Split content on markdown headings (## or ###)."""
    parts = re.split(r"(?=^#{2,3}\s)", content, flags=re.MULTILINE)
    return [p.strip() for p in parts if p.strip()]


def _split_by_paragraphs(text: str) -> list[str]:
    """Split long text at paragraph boundaries with overlap."""
    paragraphs = re.split(r"\n{2,}", text)
    chunks: list[str] = []
    current = ""

    for para in paragraphs:
        candidate = f"{current}\n\n{para}".strip() if current else para.strip()
        if len(candidate) <= _MAX_CHUNK:
            current = candidate
        else:
            if current:
                chunks.append(current)
            # Overlap: take tail of previous chunk
            if current and _OVERLAP:
                overlap = current[-_OVERLAP:]
                current = f"{overlap}\n\n{para}".strip()
            else:
                current = para.strip()

    if current:
        chunks.append(current)

    return chunks
