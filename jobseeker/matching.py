"""Score how well a job fits your CV, fully offline (no AI service, no cost).

score (0-100) = 40% role fit + 30% skill match + 15% skill coverage + 15% location,
then adjusted for seniority, required years of experience and internships.

- role fit: is the job title one of the roles your CV points to (security,
  full stack, IoT, ...)?
- skill match: how many of your CV skills the posting asks for, weighted by
  importance. A related tool counts half (Splunk when you know SIEM).
- skill coverage: of all the skills the posting names, how many you have.
- location: distance from the nearest of your home towns; remote/hybrid is a bonus.
"""
from __future__ import annotations

import math
import re
from typing import Any

from . import db
from .cv import CVProfile, load_cv
from .geo import distance_km, locate, locate_all, work_mode
from .skills import SKILL_BY_NAME, find_skills

# (pattern, fit, label). The best match on the title wins.
ROLE_FAMILIES: list[tuple[re.Pattern[str], float, str]] = [
    (re.compile(p, re.I), fit, label) for p, fit, label in [
        (r"pen ?test|penetration|ethical hack|offensive|red team|bug bounty", 1.0, "Offensive security"),
        (r"\bsoc\b|security operations|siem|blue team|incident response|csirt|threat", 0.95, "Security operations"),
        (r"vulnerab|kwetsba", 1.0, "Vulnerability management"),
        (r"cyber|security engineer|security analyst|security specialist|security consultant|"
         r"security expert|information security|infosec|it security|it-security|ict security|"
         r"security officer|informatieveiligheid|beveiliging|cybersec", 0.95, "Cybersecurity"),
        (r"devsecops|application security|appsec|cloud security|product security|network security",
         0.9, "Security engineering"),
        (r"security architect|security manager|grc|governance|compliance|iso 27001|privacy|dpo",
         0.6, "Security governance"),
        (r"full[ -]?stack", 0.85, "Full stack development"),
        (r"\biot\b|internet of things|embedded|firmware|hardware engineer|electronics|elektronica",
         0.75, "IoT / embedded"),
        (r"web ?developer|front[ -]?end|back[ -]?end|software (?:engineer|developer)|developer|"
         r"ontwikkelaar|programmeur|react|next\.?js|python|javascript|typescript", 0.7, "Software development"),
        (r"devops|cloud engineer|platform engineer|\bsre\b|site reliability", 0.55, "Cloud / DevOps"),
        (r"network engineer|system engineer|systems engineer|system administrator|sysadmin|"
         r"netwerkbeheerder|systeembeheerder|infrastructure engineer|infra", 0.5, "Infrastructure"),
        (r"lecturer|docent|teacher|trainer|instructor|lesgever", 0.45, "Teaching"),
        (r"it support|helpdesk|help desk|service desk|technician|technicus", 0.3, "IT support"),
        (r"\bict\b|\bit\b|engineer|analyst|consultant|ingenieur|architect", 0.25, "General IT"),
    ]
]
# Titles that are clearly not your field even though they contain a matching word.
NOT_MY_FIELD = re.compile(
    r"security guard|bewak|guard\b|gardien|agent de s[ée]curit[ée]|physical security|"
    r"food safety|safety officer|preventieadviseur|nurse|verpleeg|accountant|actuar|sales|verkoop|"
    r"chauffeur|driver|mechanic|monteur|welder|lasser|cleaner|schoonma|warehouse|magazijn|"
    r"recruiter|hr |payroll|boekhoud|kok\b|chef\b|cook\b|social security|account executive|"
    r"account manager|business developer|business development|sales engineer|presales|pre-sales|"
    r"commercial|marketing",
    re.I,
)

_GOVERNANCE = re.compile(r"\bgrc\b|governance|compliance|privacy|\bdpo\b|risk manager|auditor", re.I)
_HANDS_ON = re.compile(r"engineer|pen ?test|analyst|soc\b|technical", re.I)
_TOO_SENIOR = re.compile(r"\b(senior|sr\.?)\b", re.I)
_LEAD = re.compile(r"\b(lead|principal|head|manager|director|chief|ciso|cto|vp|teamlead|teamleader|"
                   r"verantwoordelijke|directeur)\b", re.I)
_JUNIOR = re.compile(r"\b(junior|jr\.?|starter|graduate|young graduate|entry[- ]level|medior)\b", re.I)
_INTERN = re.compile(r"\b(intern|internship|stage|stagiair|trainee|student|jobstudent)\b", re.I)
_YEARS = re.compile(
    r"(\d{1,2})\s*\+?\s*(?:-|to|tot|à)?\s*(?:\d{1,2}\s*)?(?:years?|yrs|jaar|jaren)", re.I)
_EXPERIENCE_WORD = re.compile(r"experience|ervaring|expérience|relevant", re.I)
_MASTER = re.compile(r"master'?s? (?:degree|diploma)|masterdiploma|master in|\bmsc\b|universit\w+ degree", re.I)


def _required_years(text: str) -> int | None:
    years = []
    for m in _YEARS.finditer(text):
        window = text[max(0, m.start() - 80): m.end() + 80]
        n = int(m.group(1))
        if 1 <= n <= 20 and _EXPERIENCE_WORD.search(window):
            years.append(n)
    return max(years) if years else None


def _role_fit(title: str, body: str) -> tuple[float, str]:
    if NOT_MY_FIELD.search(title):
        return 0.0, "Other field"
    best = (0.0, "Other field")
    for pattern, fit, label in ROLE_FAMILIES:
        if pattern.search(title) and fit > best[0]:
            best = (fit, label)
    if _GOVERNANCE.search(title) and not _HANDS_ON.search(title):
        best = (min(best[0], 0.6), "Security governance")  # policy/compliance, not hands-on
    if best[0] < 0.5:
        # Vague title ("Consultant", "Engineer"): let the description hint at the role, at a discount.
        for pattern, fit, label in ROLE_FAMILIES[:9]:
            if pattern.search(body) and fit * 0.6 > best[0]:
                best = (fit * 0.6, label)
    return best


def home_places(settings: dict[str, Any]) -> list[tuple[str, tuple[float, float]]]:
    """Your home towns that could be located, e.g. [("Langdorp", ...), ("Antwerp", ...)]."""
    homes = [(name, locate(name)) for name in settings.get("home_towns") or ["Langdorp"]]
    found = [(name, coords) for name, coords in homes if coords]
    return found or [("Langdorp", locate("Langdorp"))]


def _location_score(location: str | None, text: str, homes: list[tuple[str, tuple[float, float]]],
                    max_km: float) -> tuple[float, float | None, str, str | None]:
    """Score + distance from the *nearest* of your homes (and which home that is)."""
    mode = work_mode(f"{location or ''}\n{text}")
    places = locate_all(location) if location else []
    km, nearest = None, None
    for name, home in homes:
        for p in places:
            d = distance_km(home, p)
            if km is None or d < km:
                km, nearest = d, name
    if mode == "remote":
        return 1.0, km, mode, nearest
    if km is None:
        score = 0.6
    elif km <= max_km * 0.5:
        score = 1.0
    elif km <= max_km:
        score = 0.85
    elif km <= max_km * 1.5:
        score = 0.55
    else:
        score = 0.25
    if mode == "hybrid":
        score = min(1.0, score + 0.1)
    return score, km, mode, nearest


def score_job(job: dict[str, Any], cv: CVProfile, settings: dict[str, Any]) -> dict[str, Any]:
    title = job.get("title") or ""
    body = job.get("full_description") or job.get("description") or ""
    text = f"{title}\n{body}"

    # --- skills
    asked = find_skills(text)
    matched, related, missing = [], [], []
    match_w = 0.0
    for name in sorted(asked, key=lambda n: -SKILL_BY_NAME[n].weight):
        skill = SKILL_BY_NAME[name]
        if name in cv.skills:
            matched.append(name)
            match_w += skill.weight
        else:
            via = next((r for r in skill.related if r in cv.skills), None)
            if via:
                related.append(f"{name}, via {via}")
                match_w += skill.weight * 0.5
            else:
                missing.append(name)
    total_asked = len(matched) + len(related) + len(missing)
    if total_asked:
        skill_score = 1 - math.exp(-match_w / 6)      # ~6 weight points = good overlap
    else:
        skill_score = 0.3  # the snippet names no skills at all: unknown, not "no match"
    # Smoothed so one matching skill out of one is not treated as a perfect fit.
    coverage = (len(matched) + 0.5 * len(related) + 1) / (total_asked + 2)

    # --- role, location
    role, role_label = _role_fit(title, body)
    homes = home_places(settings)
    loc_score, km, mode, nearest_home = _location_score(
        job.get("location"), text, homes, float(settings.get("max_distance_km", 60)))

    score = 100 * (0.40 * role + 0.30 * skill_score + 0.15 * coverage + 0.15 * loc_score)

    # --- seniority & requirements
    notes, dealbreakers = [], []
    my_years = float(settings.get("experience_years", 3))
    req_years = _required_years(body)
    if req_years and req_years > my_years + 1:
        factor = max(0.55, 1 - 0.07 * (req_years - my_years - 1))
        score *= factor
        (dealbreakers if req_years >= my_years + 5 else notes).append(f"Asks {req_years}+ years of experience")
    if _LEAD.search(title):
        score *= 0.7
        notes.append("Lead / management role")
        seniority = "lead"
    elif _TOO_SENIOR.search(title):
        score *= 0.85 if not req_years else 1.0  # don't double-penalise when years were found
        seniority = "senior"
    elif _JUNIOR.search(title):
        score += 6
        seniority = "junior/medior"
    else:
        seniority = "unknown"
    if _INTERN.search(title):
        score *= 0.7
        notes.append("Internship / student job")
        seniority = "intern"
    if _MASTER.search(body):
        score *= 0.92
        notes.append("Mentions a master's degree")
    if role == 0.0:
        score = min(score, 20)

    score = int(round(max(0, min(100, score))))
    recommendation = ("strong_apply" if score >= 80 else "apply" if score >= 65
                      else "maybe" if score >= 45 else "skip")

    # --- readable summary
    parts = [f"{role_label} role" if role_label != "Other field" else "Not one of your target roles"]
    if km is not None:
        where = f"from {nearest_home}" if len(homes) > 1 else "from home"
        parts.append(f"~{max(5, int(round(km / 5) * 5))} km {where}")
    if mode != "unknown":
        parts.append(mode)
    summary = ", ".join(parts) + "."
    if total_asked:
        summary += f" You have {len(matched)} of the {total_asked} skills it mentions"
        summary += f" (+{len(related)} related)." if related else "."
    else:
        summary += " The posting text names no specific skills; open it to judge."
    if notes:
        summary += " " + "; ".join(notes) + "."

    return {
        "score": score,
        "recommendation": recommendation,
        "summary": summary,
        "matched_skills": matched,
        "related_skills": related,
        "missing_skills": missing,
        "dealbreakers": dealbreakers,
        "seniority": seniority,
        "job_language": job.get("language") or "unknown",
        "distance_km": None if km is None else round(km),
        "nearest_home": nearest_home,
        "work_mode": mode,
        "role": role_label,
        "parts": {"role": round(role, 2), "skills": round(skill_score, 2),
                  "coverage": round(coverage, 2), "location": round(loc_score, 2)},
    }


def score_jobs(job_ids: list[int] | None = None) -> int:
    """(Re)score jobs and store the result. Returns how many were scored."""
    cv = load_cv()
    settings = db.get_settings()
    count = 0
    for job in db.all_jobs():
        if job_ids is not None and job["id"] not in job_ids:
            continue
        result = score_job(job, cv, settings)
        db.update_job(job["id"], grade=result["score"], grade_json=result, graded_at=db.now_iso())
        count += 1
    return count
