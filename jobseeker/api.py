"""FastAPI app: JSON API under /api plus the static single-page frontend."""
from __future__ import annotations

import logging
from contextlib import asynccontextmanager
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
    }


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
    sort: str = "grade",
) -> list[dict[str, Any]]:
    return db.list_jobs(status=status or None, source=source or None, min_grade=min_grade, q=q or None, sort=sort)


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


@app.patch("/api/jobs/{job_id}")
def patch_job(job_id: int, patch: JobPatch) -> dict[str, Any]:
    _job_or_404(job_id)
    if patch.status and patch.status not in db.JOB_STATUSES:
        raise HTTPException(400, f"Unknown status {patch.status}")
    db.update_job(job_id, **patch.model_dump(exclude_none=True))
    if patch.full_description is not None:
        db.refresh_language(job_id)
        score_jobs([job_id])
    return _job_or_404(job_id)


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
