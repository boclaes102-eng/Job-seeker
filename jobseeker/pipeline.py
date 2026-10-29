"""One search run: fetch every source for every keyword, store, score against the CV."""
from __future__ import annotations

import logging
import threading
from concurrent.futures import ThreadPoolExecutor, as_completed
from typing import Any

from . import db
from .matching import score_jobs
from .sources import SOURCES, SourceError

log = logging.getLogger(__name__)

_lock = threading.Lock()
STATUS: dict[str, Any] = {
    "running": False, "phase": "idle", "message": "", "started_at": None,
    "finished_at": None, "new_jobs": 0, "errors": [], "progress": 0, "total": 0,
}


def _set(**kw: Any) -> None:
    STATUS.update(kw)


def start_search_async() -> bool:
    """Start a run in a background thread. False if one is already running."""
    if not _lock.acquire(blocking=False):
        return False
    _set(running=True, phase="starting", message="Starting search...", errors=[])
    threading.Thread(target=_run_locked, daemon=True, name="job-search").start()
    return True


def _run_locked() -> None:
    try:
        run_search()
    except Exception as exc:  # noqa: BLE001
        log.exception("Search run failed")
        STATUS["errors"].append(f"Run failed: {exc}")
        _set(phase="failed", message=str(exc))
    finally:
        _set(running=False, finished_at=db.now_iso())
        _lock.release()


def run_search() -> dict[str, Any]:
    settings = db.get_settings()
    _set(running=True, phase="fetching", message="Searching job sites...", started_at=db.now_iso(),
         finished_at=None, new_jobs=0, errors=[], progress=0, total=0)

    active = [s for name, s in SOURCES.items() if settings["sources"].get(name, True) and s.configured()]
    tasks = [(src, kw) for src in active for kw in settings["keywords"]]
    _set(total=len(tasks))
    new_ids: list[int] = []

    with ThreadPoolExecutor(max_workers=6) as pool:
        futures = {pool.submit(src.search, kw, int(settings["max_age_days"])): (src, kw) for src, kw in tasks}
        for i, fut in enumerate(as_completed(futures), 1):
            src, kw = futures[fut]
            try:
                for job in fut.result():
                    job_id, created = db.upsert_job(job)
                    if created and job_id:
                        new_ids.append(job_id)
            except SourceError as exc:
                STATUS["errors"].append(f"{src.label} / '{kw}': {exc}")
            _set(progress=i, new_jobs=len(new_ids), message=f"Fetched {i}/{len(tasks)} searches")

    _set(phase="scoring", message="Scoring jobs against your CV...")
    score_jobs()  # all of them: freshness changes every day, not only for new jobs

    counts = db.job_counts()
    _set(phase="done", message=(
        f"Done: {len(new_ids)} new jobs. {counts['good']} good matches (65+), "
        f"{counts['language_filtered']} left out by your language settings."
    ))
    return dict(STATUS)


def start_auto_search_loop(hours: int) -> None:
    """Run a search shortly after start-up and then every `hours` hours."""
    if hours <= 0:
        return

    def loop() -> None:
        stop = threading.Event()
        while not stop.wait(5 if STATUS["started_at"] is None else hours * 3600):
            start_search_async()

    threading.Thread(target=loop, daemon=True, name="auto-search").start()
