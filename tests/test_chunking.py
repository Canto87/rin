"""Tests for document chunking."""

from rin_memory.chunking import chunk_document


def test_short_document_single_chunk():
    """Short documents should produce a single chunk."""
    chunks = chunk_document("Short title", "Brief content.")
    assert len(chunks) == 1
    assert chunks[0]["chunk_index"] == 0
    assert "Short title" in chunks[0]["text"]
    assert "Brief content." in chunks[0]["text"]


def test_long_document_splits_by_headings():
    """Documents with markdown headings should split by section."""
    content = "Intro paragraph.\n\n"
    content += "## Section A\n" + "A content. " * 100 + "\n\n"
    content += "## Section B\n" + "B content. " * 100 + "\n\n"
    content += "### Subsection C\n" + "C content. " * 100

    chunks = chunk_document("My Doc", content)
    assert len(chunks) > 1
    # Each chunk should have the title prefix
    for chunk in chunks:
        assert "My Doc" in chunk["text"]
    # Chunk indices should be sequential
    indices = [c["chunk_index"] for c in chunks]
    assert indices == list(range(len(chunks)))


def test_title_prefix_in_all_chunks():
    """Every chunk must contain the document title for context."""
    content = "## Part 1\n" + "x " * 800 + "\n\n## Part 2\n" + "y " * 800
    chunks = chunk_document("Important Title", content)
    for chunk in chunks:
        assert chunk["text"].startswith("Important Title\n")


def test_very_long_section_splits_by_paragraph():
    """A single long section (no sub-headings) should split at paragraph boundaries."""
    paragraphs = [f"Paragraph {i}. " + "word " * 150 for i in range(10)]
    content = "\n\n".join(paragraphs)

    chunks = chunk_document("Long Section", content)
    assert len(chunks) > 1


def test_empty_content_single_chunk():
    """Empty content should still return at least one chunk."""
    chunks = chunk_document("Title Only", "")
    assert len(chunks) == 1
    assert "Title Only" in chunks[0]["text"]


def test_chunk_indices_contiguous():
    """Chunk indices should be 0, 1, 2, ... with no gaps."""
    content = "## A\n" + "text " * 300 + "\n## B\n" + "text " * 300 + "\n## C\n" + "text " * 300
    chunks = chunk_document("Test", content)
    for i, chunk in enumerate(chunks):
        assert chunk["chunk_index"] == i
