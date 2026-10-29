"""SQLite persistence. One file (data/jobs.db), no ORM, no server."""
from __future__ import annotations

import hashlib
import json
import re
import sqlite3
from collections.abc import Iterator
from contextlib import contextmanager
from datetime import UTC, datetime
from typing import Any

from .config import CONFIG
from .language import detect_language, mentions_french

SCHEMA = """
CREATE TABLE IF NOT EXISTS jobs (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    source           TEXT NOT NULL,
    external_id      TEXT NOT NULL,
    dedup_key        TEXT NOT NULL,
    title            TEXT NOT NULL,
    company          TEXT,
    location         TEXT,
    url              TEXT,
    description      TEXT,
    full_description TEXT,
    contract         TEXT,
    salary           TEXT,
    posted_at        TEXT,
    fetched_at       TEXT NOT NULL,
    grade            INTEGER,
    grade_json       TEXT,
    graded_at        TEXT,
    status           TEXT NOT NULL DEFAULT 'new',
    language         TEXT,
    requires_french  INTEGER NOT NULL DEFAULT 0,
    UNIQUE (source, external_id)
);
CREATE INDEX IF NOT EXISTS idx_jobs_dedup ON jobs (dedup_key);
CREATE INDEX IF NOT EXISTS idx_jobs_grade ON jobs (grade);

CREATE TABLE IF NOT EXISTS settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
"""

DEFAULT_SETTINGS: dict[str, Any] = {
    "keywords": [
        "cybersecurity",
        "security engineer",
        "security analyst",
        "penetration tester",
        "pentester",
        "ethical hacker",
        "SOC analyst",
        "vulnerability management",
        "full stack developer",
        "IoT engineer",
    ],
    # Distance is measured from whichever home is nearest to the job.
    "home_towns": ["Langdorp", "Antwerp"],
    "max_distance_km": 60,
    "experience_years": 3,
    "max_age_days": 30,
    "sources": {"forem": True, "ictjob": True, "adzuna": True, "jooble": True},
    # Only show postings written in these languages ("unknown" is always kept).
    "job_languages": ["en", "nl"],
    # Also drop English/Dutch postings that ask for French.
    "hide_french_required": True,
}

JOB_STATUSES = {"new", "saved", "applied", "interview", "rejected", "hidden"}


def now_iso() -> str:
    return datetime.now(UTC).replace(microsecond=0).isoformat()


@contextmanager
def connect() -> Iterator[sqlite3.Connection]:
    conn = sqlite3.connect(CONFIG.db_path, timeout=30)
    conn.row_factory = sqlite3.Row
    try:
        yield conn
        conn.commit()
    finally:
        conn.close()


def init_db() -> None:
    with connect() as conn:
        conn.execute("PRAGMA journal_mode = WAL")
        # Migrate databases from older versions of the app.
        cols = {r["name"] for r in conn.execute("PRAGMA table_info(jobs)")}
        if cols and "language" not in cols:
            conn.execute("ALTER TABLE jobs ADD COLUMN language TEXT")
        if cols and "requires_french" not in cols:
            conn.execute("ALTER TABLE jobs ADD COLUMN requires_french INTEGER NOT NULL DEFAULT 0")
        conn.executescript(SCHEMA)
        conn.execute("UPDATE jobs SET status = 'saved' WHERE status = 'cv_generated'")
        for r in conn.execute("SELECT * FROM jobs WHERE language IS NULL").fetchall():
            lang, fr = classify_language(dict(r))
            conn.execute("UPDATE jobs SET language = ?, requires_french = ? WHERE id = ?", (lang, fr, r["id"]))


# ---------------------------------------------------------------- settings
def get_settings() -> dict[str, Any]:
    with connect() as conn:
        rows = conn.execute("SELECT key, value FROM settings").fetchall()
    stored = {r["key"]: json.loads(r["value"]) for r in rows}
    merged = json.loads(json.dumps(DEFAULT_SETTINGS))
    merged.update({k: v for k, v in stored.items() if k in DEFAULT_SETTINGS})
    merged["sources"] = {**DEFAULT_SETTINGS["sources"], **stored.get("sources", {})}
    return merged


def save_settings(values: dict[str, Any]) -> dict[str, Any]:
    with connect() as conn:
        for key, value in values.items():
            if key not in DEFAULT_SETTINGS:
                continue
            conn.execute(
                "INSERT INTO settings (key, value) VALUES (?, ?) "
                "ON CONFLICT(key) DO UPDATE SET value = excluded.value",
                (key, json.dumps(value)),
            )
    return get_settings()


# ---------------------------------------------------------------- language
def classify_language(job: dict[str, Any]) -> tuple[str, int]:
    """(posting language, asks-for-French flag) for a job dict."""
    text = " ".join(filter(None, [job.get("title"), job.get("full_description") or job.get("description")]))
    lang = job.get("language") or detect_language(text)
    requires_fr = bool(job.get("requires_french")) or mentions_french(text)
    return lang, int(requires_fr)


def refresh_language(job_id: int) -> None:
    """Re-detect language and French requirement after a full description was added."""
    job = get_job(job_id)
    if not job or not job.get("full_description"):
        return
    lang = detect_language(f"{job['title']} {job['full_description']}")
    requires_fr = mentions_french(job["full_description"]) or (job["source"] == "forem" and job["requires_french"])
    update_job(job_id, language=lang if lang != "unknown" else job.get("language"),
               requires_french=int(bool(requires_fr)))


def language_clause(settings: dict[str, Any] | None = None) -> tuple[str, list[Any]]:
    """SQL condition that keeps only jobs in the wanted languages."""
    settings = settings or get_settings()
    allowed = [lang for lang in settings.get("job_languages", ["en", "nl"]) if lang in {"en", "nl", "fr"}]
    allowed.append("unknown")
    sql = f"(language IS NULL OR language IN ({','.join('?' * len(allowed))}))"
    if settings.get("hide_french_required", True):
        sql += " AND requires_french = 0"
    return sql, allowed


# -------------------------------------------------------------------- jobs
_GENDER_TAG = re.compile(r"\((?:[mhfvx]\s*/\s*){1,3}[mhfvx]\)", re.I)


def _norm(text: str | None) -> str:
    text = _GENDER_TAG.sub(" ", text or "")  # "(H/F/X)", "(m/v/x)", ...
    return re.sub(r"[^a-z0-9]+", " ", text.lower()).strip()


def make_dedup_key(title: str, company: str | None) -> str:
    return hashlib.sha1(f"{_norm(title)}|{_norm(company)}".encode()).hexdigest()


def _row_to_job(row: sqlite3.Row) -> dict[str, Any]:
    job = dict(row)
    job["grade_details"] = json.loads(job["grade_json"]) if job.get("grade_json") else None
    job.pop("grade_json", None)
    return job


def upsert_job(job: dict[str, Any]) -> tuple[int | None, bool]:
    """Insert a job if it is new. Returns (id, created).

    Jobs already known from the same source are refreshed. The same posting
    coming from a *different* source (same title + company) is skipped so the
    list does not fill up with duplicates.
    """
    dedup = make_dedup_key(job["title"], job.get("company"))
    lang, requires_fr = classify_language(job)
    with connect() as conn:
        existing = conn.execute(
            "SELECT id FROM jobs WHERE source = ? AND external_id = ?",
            (job["source"], job["external_id"]),
        ).fetchone()
        if existing:
            conn.execute(
                "UPDATE jobs SET title=?, company=?, location=?, url=?, description=?, "
                "contract=?, salary=?, posted_at=COALESCE(?, posted_at), "
                "language=COALESCE(language, ?), requires_french=MAX(requires_french, ?) WHERE id=?",
                (job["title"], job.get("company"), job.get("location"), job.get("url"),
                 job.get("description"), job.get("contract"), job.get("salary"),
                 job.get("posted_at"), lang, requires_fr, existing["id"]),
            )
            return existing["id"], False
        dup = conn.execute("SELECT id FROM jobs WHERE dedup_key = ?", (dedup,)).fetchone()
        if dup:
            return dup["id"], False
        cur = conn.execute(
            "INSERT INTO jobs (source, external_id, dedup_key, title, company, location, url, "
            "description, contract, salary, posted_at, fetched_at, language, requires_french) "
            "VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)",
            (job["source"], job["external_id"], dedup, job["title"], job.get("company"),
             job.get("location"), job.get("url"), job.get("description"), job.get("contract"),
             job.get("salary"), job.get("posted_at"), now_iso(), lang, requires_fr),
        )
        return cur.lastrowid, True


def get_job(job_id: int) -> dict[str, Any] | None:
    with connect() as conn:
        row = conn.execute("SELECT * FROM jobs WHERE id = ?", (job_id,)).fetchone()
    return _row_to_job(row) if row else None


def list_jobs(
    *,
    status: str | None = None,
    source: str | None = None,
    min_grade: int | None = None,
    q: str | None = None,
    sort: str = "grade",
    limit: int = 500,
) -> list[dict[str, Any]]:
    lang_sql, lang_params = language_clause()
    where: list[str] = [lang_sql]
    params: list[Any] = list(lang_params)
    if status:
        where.append("status = ?")
        params.append(status)
    else:
        where.append("status != 'hidden'")
    if source:
        where.append("source = ?")
        params.append(source)
    if min_grade is not None:
        where.append("grade >= ?")
        params.append(min_grade)
    if q:
        where.append("(title LIKE ? OR company LIKE ? OR description LIKE ? OR location LIKE ?)")
        params.extend([f"%{q}%"] * 4)
    order = {
        "grade": "grade IS NULL, grade DESC, posted_at DESC",
        "date": "posted_at IS NULL, posted_at DESC, grade DESC",
    }.get(sort, "grade IS NULL, grade DESC, posted_at DESC")
    sql = "SELECT * FROM jobs WHERE " + " AND ".join(where) + f" ORDER BY {order} LIMIT ?"
    params.append(limit)
    with connect() as conn:
        rows = conn.execute(sql, params).fetchall()
    return [_row_to_job(r) for r in rows]


def update_job(job_id: int, **fields: Any) -> None:
    allowed = {"status", "full_description", "grade", "grade_json", "graded_at", "language", "requires_french"}
    sets = {k: v for k, v in fields.items() if k in allowed}
    if not sets:
        return
    if "grade_json" in sets and sets["grade_json"] is not None and not isinstance(sets["grade_json"], str):
        sets["grade_json"] = json.dumps(sets["grade_json"])
    cols = ", ".join(f"{k} = ?" for k in sets)
    with connect() as conn:
        conn.execute(f"UPDATE jobs SET {cols} WHERE id = ?", (*sets.values(), job_id))


def all_jobs() -> list[dict[str, Any]]:
    with connect() as conn:
        rows = conn.execute("SELECT * FROM jobs").fetchall()
    return [_row_to_job(r) for r in rows]


def job_counts() -> dict[str, int]:
    lang_sql, lang_params = language_clause()
    with connect() as conn:
        row = conn.execute(
            "SELECT COUNT(*), SUM(grade >= 65), SUM(grade >= 80) "
            f"FROM jobs WHERE status != 'hidden' AND {lang_sql}", lang_params,
        ).fetchone()
        filtered = conn.execute(f"SELECT COUNT(*) FROM jobs WHERE NOT ({lang_sql})", lang_params).fetchone()[0]
        by_status = dict(conn.execute(
            f"SELECT status, COUNT(*) FROM jobs WHERE {lang_sql} GROUP BY status", lang_params,
        ).fetchall())
    return {"total": row[0] or 0, "good": row[1] or 0, "strong": row[2] or 0,
            "language_filtered": filtered or 0,
            "by_status": {s: by_status.get(s, 0) for s in JOB_STATUSES}}
