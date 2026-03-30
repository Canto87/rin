"""Embedding via Ollama mxbai-embed-large."""

import logging
import ollama as _ollama

MODEL = "mxbai-embed-large"
_MAX_CHARS = 1500  # ~400 tokens, safe margin for 512 token limit
_FALLBACK_CHARS = 800  # aggressive fallback on context length error

log = logging.getLogger(__name__)


def _truncate(text: str, limit: int = _MAX_CHARS) -> str:
    """Truncate text to fit within model's token limit."""
    if len(text) <= limit:
        return text
    return text[:limit]


async def embed(text: str) -> list[float]:
    """Embed a single text string. Retries with shorter input on context length error."""
    client = _ollama.AsyncClient()
    truncated = _truncate(text)
    try:
        resp = await client.embed(model=MODEL, input=truncated)
        return resp.embeddings[0]
    except _ollama.ResponseError as e:
        if "context length" in str(e) or "input length" in str(e):
            log.warning("embed: context length exceeded, retrying with %d chars", _FALLBACK_CHARS)
            truncated = _truncate(text, _FALLBACK_CHARS)
            resp = await client.embed(model=MODEL, input=truncated)
            return resp.embeddings[0]
        raise


async def embed_batch(texts: list[str]) -> list[list[float]]:
    """Embed multiple texts in a single call."""
    if not texts:
        return []
    client = _ollama.AsyncClient()
    truncated = [_truncate(t) for t in texts]
    try:
        resp = await client.embed(model=MODEL, input=truncated)
        return resp.embeddings
    except _ollama.ResponseError as e:
        if "context length" in str(e) or "input length" in str(e):
            log.warning("embed_batch: context length exceeded, retrying with %d chars", _FALLBACK_CHARS)
            truncated = [_truncate(t, _FALLBACK_CHARS) for t in texts]
            resp = await client.embed(model=MODEL, input=truncated)
            return resp.embeddings
        raise
