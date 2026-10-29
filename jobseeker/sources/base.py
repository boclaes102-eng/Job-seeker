"""Shared helpers for job sources.

Every source returns a list of plain dicts with these keys:
    source, external_id, title, company, location, url, description,
    contract, salary, posted_at (ISO date string or None)
"""
from __future__ import annotations

import html
import re
from typing import Any, Protocol

import httpx

USER_AGENT = (
    "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 "
    "(KHTML, like Gecko) Chrome/130.0 Safari/537.36 JobSeeker/1.0"
)


class SourceError(RuntimeError):
    pass


class JobSource(Protocol):
    name: str
    label: str

    def configured(self) -> bool: ...

    def search(self, keyword: str, max_age_days: int) -> list[dict[str, Any]]: ...


def http_client(timeout: float = 20.0) -> httpx.Client:
    return httpx.Client(
        timeout=httpx.Timeout(timeout, connect=10.0),
        headers={"User-Agent": USER_AGENT, "Accept-Language": "en,nl;q=0.9,fr;q=0.8"},
        follow_redirects=True,
    )


_TAG = re.compile(r"<[^>]+>")
_WS = re.compile(r"[ \t\r\f\v]+")


def clean_text(raw: str | None) -> str:
    """Strip HTML tags and (double-)escaped entities into plain text."""
    if not raw:
        return ""
    text = raw
    for _ in range(3):  # some feeds escape twice: &amp;#39;
        unescaped = html.unescape(text)
        if unescaped == text:
            break
        text = unescaped
    text = re.sub(r"<\s*(br|/p|/li|/h\d)\s*/?>", "\n", text, flags=re.I)
    text = _TAG.sub(" ", text)
    text = _WS.sub(" ", text)
    text = re.sub(r"\n\s*\n+", "\n\n", text)
    return text.strip()
