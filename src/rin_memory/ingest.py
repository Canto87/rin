"""Markdown file parser for memory ingestion."""

import re
from pathlib import Path

# Section title → document kind mapping
KIND_MAP = {
    "session journal": "session_journal",
    "architectural decisions": "arch_decision",
    "domain knowledge": "domain_knowledge",
    "team patterns": "team_pattern",
}


def parse_markdown_sections(file_path: str) -> list[dict]:
    """Parse a markdown file into sections based on headings.

    Returns a list of dicts with keys: title, content, kind (optional), tags (optional).
    """
    content = Path(file_path).read_text(encoding="utf-8")
    sections = []
    current_title = None
    current_lines: list[str] = []
    parent_title = None

    for line in content.split("\n"):
        heading = re.match(r"^(#{1,3})\s+(.+)$", line)
        if heading:
            # Save previous section
            if current_title and current_lines:
                text = "\n".join(current_lines).strip()
                if text:
                    sections.append(_make_section(current_title, text, parent_title))

            level = len(heading.group(1))
            title = heading.group(2).strip()

            if level <= 2:
                parent_title = title
            current_title = title
            current_lines = []
        else:
            current_lines.append(line)

    # Save last section
    if current_title and current_lines:
        text = "\n".join(current_lines).strip()
        if text:
            sections.append(_make_section(current_title, text, parent_title))

    return sections


def _make_section(title: str, content: str, parent_title: str | None) -> dict:
    """Create a section dict, inferring kind from title hierarchy."""
    section: dict = {"title": title, "content": content}

    # Infer kind from parent or self title
    for keyword, kind in KIND_MAP.items():
        check = (parent_title or "").lower()
        if keyword in check or keyword in title.lower():
            section["kind"] = kind
            break

    # Extract date tags from title (e.g. "2026-02-21 — ...")
    date_match = re.match(r"(\d{4}-\d{2}-\d{2})", title)
    if date_match:
        section["tags"] = [date_match.group(1)]
        section["source"] = f"session:{date_match.group(1)}"

    return section
