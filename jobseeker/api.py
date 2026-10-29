"""FastAPI app: JSON API under /api plus the static single-page frontend."""
from __future__ import annotations

import logging
from contextlib import asynccontextmanager
from datetime import date, datetime, timedelta
from typing import Any

from fastapi import Body, FastAPI, HTTPException
from fastapi.responses import FileResponse
from fastapi.staticfiles import StaticFiles
from pydantic import BaseModel

from . import db
from .config import CONFIG, ROOT_DIR
from .cv import CVMissingError, load_cv
from .enrich import fetch_description
from .matching import score_jobs
from .pipeline import STATUS, start_auto_search_loop, start_search_async
from .skills import SKILL_BY_NAME
from .sources import SOURCES

logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(name)s: %(message)s")
FRONTEND = ROOT_DIR / "frontend"


@asynccontextmanager
async def lifespan(_: FastAPI):
    db.init_db()
    try:
        score_jobs()  # CV.pdf or the scoring rules may have changed since the last run
    except CVMissingError:
        pass
    start_auto_search_loop(CONFIG.auto_search_hours)
    yield


app = FastAPI(title="Job Seeker", lifespan=lifespan)
app.mount("/static", StaticFiles(directory=FRONTEND), name="static")


@app.get("/", include_in_schema=False)
def index() -> FileResponse:
    return FileResponse(FRONTEND / "index.html")


@app.get("/api/status")
def status() -> dict[str, Any]:
    try:
        load_cv()
        cv_ok = True
    except CVMissingError:
        cv_ok = False
    return {
        "cv_loaded": cv_ok,
        "sources": {n: {"label": s.label, "configured": s.configured()} for n, s in SOURCES.items()},
        "search": STATUS,
        "counts": db.job_counts(),
        "seen_until": db.seen_until(),
    }


@app.post("/api/seen")
def mark_seen() -> dict[str, Any]:
    """You have looked at everything found so far."""
    return {"seen_until": db.mark_seen()}


@app.get("/api/settings")
def get_settings() -> dict[str, Any]:
    return db.get_settings()


@app.put("/api/settings")
def put_settings(values: dict[str, Any] = Body(...)) -> dict[str, Any]:
    if "keywords" in values:
        values["keywords"] = [k.strip() for k in values["keywords"] if k and k.strip()]
    if "job_languages" in values:
        values["job_languages"] = [lang for lang in values["job_languages"] if lang in {"en", "nl", "fr"}]
    saved = db.save_settings(values)
    score_jobs()  # home town, distance or experience may have changed
    return saved


@app.get("/api/cv")
def cv_skills() -> dict[str, Any]:
    """What the app read from CV.pdf: the skills used for matching."""
    try:
        profile = load_cv()
    except CVMissingError as exc:
        raise HTTPException(404, str(exc)) from exc
    skills = sorted(profile.skills, key=lambda n: (-SKILL_BY_NAME[n].weight, n))
    return {"path": str(CONFIG.cv_pdf_path), "skills": skills}


@app.post("/api/search")
def search() -> dict[str, Any]:
    started = start_search_async()
    return {"started": started, "status": STATUS}


@app.get("/api/jobs")
def jobs(
    status: str | None = None,
    source: str | None = None,
    min_grade: int | None = None,
    q: str | None = None,
    since: str | None = None,
    sort: str = "grade",
) -> list[dict[str, Any]]:
    return db.list_jobs(status=status or None, source=source or None, min_grade=min_grade, q=q or None,
                        since=since or None, sort=sort)


# --------------------------------------------- jobs sent from your browser
class JobLookup(BaseModel):
    url: str | None = None
    title: str | None = None
    company: str | None = None


class JobImport(JobLookup):
    title: str
    location: str | None = None
    posted_at: str | None = None
    text: str
    job_id: int | None = None  # update this job with the full text instead of adding one


@app.post("/api/jobs/match")
def match_job(lookup: JobLookup) -> dict[str, Any]:
    """Is this posting already in your list? Checked before the add form is shown."""
    return {"job": db.find_match(lookup.url, lookup.title, lookup.company)}


@app.post("/api/jobs/import")
def import_job(item: JobImport) -> dict[str, Any]:
    """Add a job you found yourself, or give a known job its full text. Either way it is scored again."""
    title, text = item.title.strip(), item.text.strip()
    if not title or not text:
        raise HTTPException(400, "A job needs a title and the posting text.")
    posted = (item.posted_at or "").strip()[:10] or None
    if posted:
        try:
            date.fromisoformat(posted)
        except ValueError:
            raise HTTPException(400, "posted_at must look like YYYY-MM-DD") from None

    if item.job_id is not None:
        job, created = _job_or_404(item.job_id), False
    else:
        url = db.canonical_url(item.url) or None
        company = (item.company or "").strip() or None
        job_id, created = db.upsert_job({
            "source": "manual", "external_id": url or f"manual:{db.make_dedup_key(title, company)}",
            "title": title, "company": company, "location": (item.location or "").strip() or None,
            "url": url, "description": text[:500], "posted_at": posted,
        })
        job = _job_or_404(job_id)
    fields: dict[str, Any] = {"full_description": text[:30000]}
    if posted and not job.get("posted_at"):
        fields["posted_at"] = posted
    db.update_job(job["id"], **fields)
    db.refresh_language(job["id"])
    score_jobs([job["id"]])
    return {"created": created, "job": _job_or_404(job["id"])}


def _job_or_404(job_id: int) -> dict[str, Any]:
    job = db.get_job(job_id)
    if not job:
        raise HTTPException(404, "Job not found")
    return job


@app.get("/api/jobs/{job_id}")
def job(job_id: int) -> dict[str, Any]:
    return _job_or_404(job_id)


class JobPatch(BaseModel):
    status: str | None = None
    full_description: str | None = None
    # Tracking fields: send "" or null to clear one.
    applied_on: str | None = None
    follow_up_on: str | None = None
    interview_at: str | None = None
    contact: str | None = None
    notes: str | None = None


_DATE_FIELDS = {"applied_on": "%Y-%m-%d", "follow_up_on": "%Y-%m-%d", "interview_at": "%Y-%m-%dT%H:%M"}


def _tracking_values(patch: JobPatch) -> dict[str, Any]:
    values = {}
    for field in patch.model_fields_set & set(db.TRACKING_FIELDS):
        value = (getattr(patch, field) or "").strip() or None
        if value and field in _DATE_FIELDS:
            try:
                datetime.strptime(value, _DATE_FIELDS[field])
            except ValueError:
                raise HTTPException(400, f"{field} must look like {_DATE_FIELDS[field]}") from None
        values[field] = value
    return values


@app.patch("/api/jobs/{job_id}")
def patch_job(job_id: int, patch: JobPatch) -> dict[str, Any]:
    job = _job_or_404(job_id)
    if patch.status and patch.status not in db.JOB_STATUSES:
        raise HTTPException(400, f"Unknown status {patch.status}")
    fields = _tracking_values(patch)
    if patch.status:
        fields["status"] = patch.status
    if patch.full_description is not None:
        fields["full_description"] = patch.full_description
    # Marking a job applied starts the clock: today's date and a first follow-up a week later.
    if patch.status == "applied" and not job.get("applied_on") and "applied_on" not in fields:
        today = date.today()
        fields["applied_on"] = today.isoformat()
        if not job.get("follow_up_on") and "follow_up_on" not in fields:
            fields["follow_up_on"] = (today + timedelta(days=db.FOLLOW_UP_AFTER_DAYS)).isoformat()
    db.update_job(job_id, **fields)
    if patch.full_description is not None:
        db.refresh_language(job_id)
        score_jobs([job_id])
    return _job_or_404(job_id)


@app.post("/api/jobs/{job_id}/followed-up")
def followed_up(job_id: int) -> dict[str, Any]:
    """You sent a follow-up today: note it and schedule the next one."""
    job = _job_or_404(job_id)
    today = date.today()
    line = f"Followed up on {today.isoformat()}."
    notes = f"{job['notes']}\n{line}" if job.get("notes") else line
    db.update_job(job_id, notes=notes,
                  follow_up_on=(today + timedelta(days=db.FOLLOW_UP_AFTER_DAYS)).isoformat())
    return _job_or_404(job_id)


@app.get("/api/agenda")
def agenda() -> dict[str, Any]:
    return {"today": date.today().isoformat(), **db.agenda(date.today().isoformat())}


@app.post("/api/jobs/{job_id}/fetch-description")
def fetch_full_description(job_id: int) -> dict[str, Any]:
    job = _job_or_404(job_id)
    text = fetch_description(job.get("url") or "")
    if not text:
        raise HTTPException(
            422, "Could not read the full description from that site. Open the posting and paste it instead."
        )
    db.update_job(job_id, full_description=text)
    db.refresh_language(job_id)
    score_jobs([job_id])
    return _job_or_404(job_id)
