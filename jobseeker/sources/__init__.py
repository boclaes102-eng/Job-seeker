"""Belgian job sources.

Not included on purpose: LinkedIn, Indeed and StepStone forbid scraping in
their terms of use, and the VDAB vacancy API is only open to contracted
partners (developer.vdab.be/openservices). Adzuna and Jooble aggregate many
of those boards' postings through official APIs instead.
"""
from .adzuna import AdzunaSource
from .base import JobSource, SourceError
from .forem import ForemSource
from .ictjob import IctjobSource
from .jooble import JoobleSource

SOURCES: dict[str, JobSource] = {
    s.name: s for s in (ForemSource(), IctjobSource(), AdzunaSource(), JoobleSource())
}

__all__ = ["SOURCES", "JobSource", "SourceError"]
