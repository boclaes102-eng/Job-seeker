"""Le Forem job offers (Wallonia + Brussels, many in Flanders too) via the
Open Data Wallonie-Bruxelles portal. Free, no API key.

https://www.odwb.be/explore/dataset/offres-d-emploi-forem/
The dataset has no free-text job description, so the description is built
from the structured fields (profession, sector, required experience, ...).
"""
from __future__ import annotations

import re
from datetime import date, timedelta
from typing import Any

from ..language import detect_language
from .base import SourceError, http_client

_GENDER_TAG = re.compile(r"\((?:[mhfvx]\s*/\s*){1,3}[mhfvx]\)", re.I)

API = "https://www.odwb.be/api/explore/v2.1/catalog/datasets/offres-d-emploi-forem/records"


def _join(value: Any) -> str:
    if value is None:
        return ""
    if isinstance(value, list):
        return ", ".join(str(v) for v in value if v)
    return str(value)


class ForemSource:
    name = "forem"
    label = "Le Forem (open data)"

    def configured(self) -> bool:
        return True

    def search(self, keyword: str, max_age_days: int) -> list[dict[str, Any]]:
        since = (date.today() - timedelta(days=max_age_days)).isoformat()
        safe_kw = keyword.replace('"', " ")
        params = {
            "where": f'search("{safe_kw}") AND datedebutdiffusion >= date\'{since}\'',
            "order_by": "datedebutdiffusion DESC",
            "limit": 100,
        }
        try:
            with http_client() as client:
                resp = client.get(API, params=params)
                resp.raise_for_status()
                records = resp.json().get("results", [])
        except Exception as exc:  # noqa: BLE001
            raise SourceError(f"Forem request failed: {exc}") from exc

        jobs = []
        for r in records:
            offer_id = r.get("numerooffreforem")
            title = r.get("titreoffre")
            if not offer_id or not title:
                continue
            parts = [
                f"Profession: {_join(r.get('metier'))}",
                f"Required experience: {_join(r.get('experiencerequise'))}",
                f"Sector: {_join(r.get('secteurs'))}",
                f"Languages: {_join(r.get('langues'))}",
                f"Education level: {_join(r.get('niveauxetudes'))}",
                f"Contract: {_join(r.get('typecontrat'))}, {_join(r.get('regimetravail'))}",
                f"Driving licence: {_join(r.get('permisdeconduire'))}",
            ]
            description = "\n".join(p for p in parts if not p.endswith(": ") and not p.endswith(": , "))
            location = _join(r.get("lieuxtravaillocalite")) or _join(r.get("lieuxtravailregion"))
            # Forem adds "(H/F/X)" to every title and its field values are always French,
            # so detect the posting language from the bare title only.
            required_langs = _join(r.get("langues")).lower()
            language = detect_language(_GENDER_TAG.sub(" ", title))
            if language == "unknown" and required_langs and "fran" in required_langs \
                    and "anglais" not in required_langs and "erlandais" not in required_langs:
                language = "fr"
            jobs.append({
                "source": self.name,
                "external_id": str(offer_id),
                "title": title.strip(),
                "company": _join(r.get("nomemployeur")) or None,
                "location": location or None,
                "url": r.get("url") or f"https://www.leforem.be/recherche-offres/offre-detail/{offer_id}",
                "description": description,
                "contract": _join(r.get("typecontrat")) or None,
                "salary": None,
                "posted_at": r.get("datedebutdiffusion"),
                "language": language,
                "requires_french": "fran" in required_langs,
            })
        return jobs
