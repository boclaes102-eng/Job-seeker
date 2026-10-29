"""Jooble (job aggregator with Belgian coverage). Free API key:
https://jooble.org/api/about  ->  set JOOBLE_API_KEY in .env
"""
from __future__ import annotations

from datetime import date, timedelta
from typing import Any

from ..config import CONFIG
from .base import SourceError, clean_text, http_client


class JoobleSource:
    name = "jooble"
    label = "Jooble (API)"

    def configured(self) -> bool:
        return bool(CONFIG.jooble_api_key)

    def search(self, keyword: str, max_age_days: int) -> list[dict[str, Any]]:
        url = f"https://{CONFIG.jooble_host}/api/{CONFIG.jooble_api_key}"
        # A Belgian key (be.jooble.org) already searches only Belgium; a key for another
        # country's index needs the location to narrow it down.
        location = "" if CONFIG.jooble_host.startswith("be.") else "Belgium"
        results: list[dict[str, Any]] = []
        try:
            with http_client() as client:
                for page in ("1", "2"):
                    resp = client.post(url, json={"keywords": keyword, "location": location, "page": page})
                    resp.raise_for_status()
                    batch = resp.json().get("jobs", [])
                    results.extend(batch)
                    if len(batch) < 20:
                        break
        except Exception as exc:  # noqa: BLE001
            raise SourceError(
                f"Jooble request failed ({exc}). Check that JOOBLE_HOST matches your key's country."
            ) from exc

        cutoff = (date.today() - timedelta(days=max_age_days)).isoformat()
        jobs = []
        for r in results:
            if not r.get("id") or not r.get("title"):
                continue
            posted = (r.get("updated") or "")[:10] or None
            if posted and posted < cutoff:
                continue
            jobs.append({
                "source": self.name,
                "external_id": str(r["id"]),
                "title": clean_text(r["title"]),
                "company": r.get("company") or None,
                "location": r.get("location") or None,
                "url": r.get("link"),
                "description": clean_text(r.get("snippet")),
                "contract": r.get("type") or None,
                "salary": r.get("salary") or None,
                "posted_at": posted,
            })
        return jobs
