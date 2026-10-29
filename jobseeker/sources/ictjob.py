"""ictjob.be: Belgium's IT job board, via its public RSS feed. No API key.

The feed returns the 50 latest matches per keyword, with a shortened
description. Paste or fetch the full posting for a more accurate score.
"""
from __future__ import annotations

import re
import xml.etree.ElementTree as ET
from datetime import UTC, datetime, timedelta
from email.utils import parsedate_to_datetime
from typing import Any

from .base import SourceError, clean_text, http_client

FEED = "https://www.ictjob.be/en/rss"


def _split_title(raw: str) -> tuple[str, str | None, str | None]:
    """'Senior CyberArk Engineer - DigiTribe - Brussels' -> (title, company, city)."""
    parts = [p.strip() for p in raw.split(" - ")]
    if len(parts) >= 3:
        title = " - ".join(parts[:-2])
        company, city = parts[-2], parts[-1]
    elif len(parts) == 2:
        title, company, city = parts[0], parts[1], None
    else:
        return raw.strip(), None, None
    # Some recruiters prefix the title with "Company: ".
    if company and title.lower().startswith(company.lower() + ":"):
        title = title[len(company) + 1:].strip()
    return title, company, city


class IctjobSource:
    name = "ictjob"
    label = "ictjob.be (RSS)"

    def configured(self) -> bool:
        return True

    def search(self, keyword: str, max_age_days: int) -> list[dict[str, Any]]:
        try:
            with http_client() as client:
                resp = client.get(FEED, params={"keywords": keyword})
                resp.raise_for_status()
                root = ET.fromstring(resp.content)
        except Exception as exc:  # noqa: BLE001
            raise SourceError(f"ictjob request failed: {exc}") from exc

        cutoff = datetime.now(UTC) - timedelta(days=max_age_days)
        jobs = []
        for item in root.iter("item"):
            link = (item.findtext("link") or "").strip()
            raw_title = clean_text(item.findtext("title"))
            if not link or not raw_title:
                continue
            posted = None
            if item.findtext("pubDate"):
                try:
                    dt = parsedate_to_datetime(item.findtext("pubDate"))
                    if dt < cutoff:
                        continue
                    posted = dt.date().isoformat()
                except (TypeError, ValueError):
                    pass
            title, company, city = _split_title(raw_title)
            m = re.search(r"/(\d+-\d+)/?$", link)
            jobs.append({
                "source": self.name,
                "external_id": m.group(1) if m else link,
                "title": title,
                "company": company,
                "location": city,
                "url": link,
                "description": clean_text(item.findtext("description")),
                "contract": None,
                "salary": None,
                "posted_at": posted,
            })
        return jobs
