"""Adzuna Belgium (aggregates many Belgian job boards). Free API key:
https://developer.adzuna.com/  ->  set ADZUNA_APP_ID and ADZUNA_APP_KEY in .env
"""
from __future__ import annotations

from typing import Any

from ..config import CONFIG
from .base import SourceError, clean_text, http_client

API = "https://api.adzuna.com/v1/api/jobs/be/search/{page}"


class AdzunaSource:
    name = "adzuna"
    label = "Adzuna Belgium (API)"

    def configured(self) -> bool:
        return bool(CONFIG.adzuna_app_id and CONFIG.adzuna_app_key)

    def search(self, keyword: str, max_age_days: int) -> list[dict[str, Any]]:
        jobs: list[dict[str, Any]] = []
        try:
            with http_client() as client:
                for page in (1, 2):
                    resp = client.get(API.format(page=page), params={
                        "app_id": CONFIG.adzuna_app_id,
                        "app_key": CONFIG.adzuna_app_key,
                        "what": keyword,
                        "max_days_old": max_age_days,
                        "results_per_page": 50,
                        "sort_by": "date",
                        "content-type": "application/json",
                    })
                    resp.raise_for_status()
                    results = resp.json().get("results", [])
                    jobs.extend(self._convert(r) for r in results)
                    if len(results) < 50:
                        break
        except Exception as exc:  # noqa: BLE001
            raise SourceError(f"Adzuna request failed: {exc}") from exc
        return [j for j in jobs if j]

    def _convert(self, r: dict[str, Any]) -> dict[str, Any] | None:
        if not r.get("id") or not r.get("title"):
            return None
        salary = None
        if r.get("salary_min") and r.get("salary_is_predicted") not in (1, "1"):
            salary = f"€{int(r['salary_min']):,}" + (
                f" – €{int(r['salary_max']):,}" if r.get("salary_max") else ""
            )
        return {
            "source": self.name,
            "external_id": str(r["id"]),
            "title": clean_text(r["title"]),
            "company": (r.get("company") or {}).get("display_name"),
            "location": (r.get("location") or {}).get("display_name"),
            "url": r.get("redirect_url"),
            "description": clean_text(r.get("description")),
            "contract": " ".join(x for x in (r.get("contract_type"), r.get("contract_time")) if x) or None,
            "salary": salary,
            "posted_at": (r.get("created") or "")[:10] or None,
        }
