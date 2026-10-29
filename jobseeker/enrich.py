"""Best-effort download of the full job description from the posting page.

Many job sites embed a schema.org JobPosting in JSON-LD; that is tried first.
Sites that render with JavaScript (e.g. leforem.be) or block bots will fail;
then the user can paste the description into the app instead.
"""
from __future__ import annotations

import json
from typing import Any

from bs4 import BeautifulSoup

from .sources.base import clean_text, http_client

MAX_CHARS = 15000


def _find_job_posting(data: Any) -> dict[str, Any] | None:
    if isinstance(data, list):
        for item in data:
            found = _find_job_posting(item)
            if found:
                return found
    elif isinstance(data, dict):
        kind = data.get("@type")
        if kind == "JobPosting" or (isinstance(kind, list) and "JobPosting" in kind):
            return data
        for key in ("@graph", "mainEntity", "itemListElement"):
            if key in data:
                found = _find_job_posting(data[key])
                if found:
                    return found
    return None


def fetch_description(url: str, timeout: float = 15.0) -> str | None:
    if not url:
        return None
    try:
        with http_client(timeout=timeout) as client:
            resp = client.get(url)
            resp.raise_for_status()
            page = resp.text
    except Exception:  # noqa: BLE001
        return None

    soup = BeautifulSoup(page, "html.parser")
    for tag in soup.find_all("script", type="application/ld+json"):
        try:
            posting = _find_job_posting(json.loads(tag.string or ""))
        except (json.JSONDecodeError, TypeError):
            continue
        if posting and posting.get("description"):
            text = clean_text(posting["description"])
            if len(text) > 200:
                return text[:MAX_CHARS]

    for tag in soup(["script", "style", "noscript", "nav", "header", "footer", "form", "svg"]):
        tag.decompose()
    container = soup.find("main") or soup.find("article") or soup.body
    if not container:
        return None
    text = clean_text(container.get_text("\n"))
    lines = [ln.strip() for ln in text.splitlines() if len(ln.strip()) > 2]
    text = "\n".join(lines)
    return text[:MAX_CHARS] if len(text) > 400 else None
