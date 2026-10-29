from jobseeker.cv import CVProfile
from jobseeker.db import DEFAULT_SETTINGS
from jobseeker.matching import score_job
from jobseeker.skills import find_skills

CV = CVProfile(
    text="",
    skills=frozenset({"Penetration testing", "OWASP", "Burp Suite", "SIEM", "Python", "Linux"}),
    mtime=0,
)
SETTINGS = dict(DEFAULT_SETTINGS)
WEIGHTS = {"role": 40, "skills": 30, "coverage": 15, "location": 15}


def job(title, description="", location="Antwerpen"):
    return {"title": title, "description": description, "location": location}


def test_find_skills_uses_aliases():
    assert {"Penetration testing", "Burp Suite"} <= find_skills("Experienced pentester who knows Burp.")


def test_target_role_beats_other_field():
    security = score_job(job("Penetration Tester", "Pentesting with Burp Suite and OWASP. Python."), CV, SETTINGS)
    other = score_job(job("Accountant", "Bookkeeping and payroll."), CV, SETTINGS)
    assert security["score"] > other["score"]
    assert other["score"] <= 20


def test_related_skill_counts():
    g = score_job(job("SOC Analyst", "Monitoring with Splunk."), CV, SETTINGS)
    assert "Splunk, via SIEM" in g["related_skills"]


def test_lead_title_scores_lower():
    base = score_job(job("Security Engineer", "Pentesting, OWASP, Python."), CV, SETTINGS)
    lead = score_job(job("Lead Security Engineer", "Pentesting, OWASP, Python."), CV, SETTINGS)
    assert lead["score"] < base["score"]


def test_parts_add_up_to_score_without_adjustments():
    # The UI shows each part as points out of its weight; with no seniority or
    # experience adjustments those points must add up to the score (rounding aside).
    g = score_job(job("Security Engineer", "Pentesting, OWASP, Burp Suite, SIEM, Python."), CV, SETTINGS)
    assert set(g["parts"]) == set(WEIGHTS)
    points = sum(g["parts"][k] * w for k, w in WEIGHTS.items())
    assert abs(points - g["score"]) <= 3


def test_score_shape():
    g = score_job(job("Full Stack Developer", "React and Python", location="Gent"), CV, SETTINGS)
    assert 0 <= g["score"] <= 100
    assert g["recommendation"] in {"strong_apply", "apply", "maybe", "skip"}
    assert g["distance_km"] is not None
