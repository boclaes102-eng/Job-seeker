"""Read your CV straight from CV.pdf and work out which skills it shows.

The PDF is re-read automatically whenever it changes, so replacing CV.pdf
with a newer version is all you need to do to update the matching.
"""
from __future__ import annotations

import threading
from dataclasses import dataclass

import pymupdf

from .config import CONFIG
from .skills import find_skills


class CVMissingError(RuntimeError):
    pass


@dataclass(frozen=True)
class CVProfile:
    text: str
    skills: frozenset[str]
    mtime: float


_cache: CVProfile | None = None
_lock = threading.Lock()


def load_cv() -> CVProfile:
    global _cache
    path = CONFIG.cv_pdf_path
    if not path.exists():
        raise CVMissingError(f"CV not found at {path}. Put your CV in the project folder as CV.pdf.")
    mtime = path.stat().st_mtime
    with _lock:
        if _cache is None or _cache.mtime != mtime:
            with pymupdf.open(path) as doc:
                text = "\n".join(page.get_text() for page in doc)
            # PDFs often split words over lines ("AI-\npowered"): join them back.
            text = text.replace("-\n", "-").replace("\n", " ")
            _cache = CVProfile(text=text, skills=frozenset(find_skills(text)), mtime=mtime)
        return _cache
