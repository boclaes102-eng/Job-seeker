"""Static configuration loaded from environment variables / .env.

Preferences you change from the UI (keywords, home town, ...) live in the
database instead, see ``db.get_settings``.
"""
from __future__ import annotations

import os
from dataclasses import dataclass
from pathlib import Path

from dotenv import load_dotenv

ROOT_DIR = Path(__file__).resolve().parent.parent
load_dotenv(ROOT_DIR / ".env")


def _int(name: str, default: int) -> int:
    try:
        return int(os.getenv(name, default))
    except ValueError:
        return default


@dataclass(frozen=True)
class Config:
    data_dir: Path
    db_path: Path
    cv_pdf_path: Path

    adzuna_app_id: str
    adzuna_app_key: str
    jooble_api_key: str
    jooble_host: str

    auto_search_hours: int
    host: str
    port: int


def load_config() -> Config:
    data_dir = Path(os.getenv("DATA_DIR", ROOT_DIR / "data")).resolve()
    data_dir.mkdir(parents=True, exist_ok=True)
    cv_pdf = Path(os.getenv("CV_PDF", "CV.pdf"))
    return Config(
        data_dir=data_dir,
        db_path=data_dir / "jobs.db",
        cv_pdf_path=(cv_pdf if cv_pdf.is_absolute() else ROOT_DIR / cv_pdf).resolve(),
        adzuna_app_id=os.getenv("ADZUNA_APP_ID", ""),
        adzuna_app_key=os.getenv("ADZUNA_APP_KEY", ""),
        jooble_api_key=os.getenv("JOOBLE_API_KEY", ""),
        jooble_host=os.getenv("JOOBLE_HOST", "be.jooble.org"),
        auto_search_hours=_int("AUTO_SEARCH_HOURS", 0),
        host=os.getenv("HOST", "127.0.0.1"),
        port=_int("PORT", 8000),
    )


CONFIG = load_config()
