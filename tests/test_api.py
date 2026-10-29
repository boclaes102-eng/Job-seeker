import pytest
from fastapi.testclient import TestClient

from jobseeker import db
from jobseeker.api import app
from jobseeker.matching import score_jobs


@pytest.fixture(scope="module")
def client():
    with TestClient(app) as c:  # runs the lifespan: init_db and scoring
        db.upsert_job({"source": "ictjob", "external_id": "t1", "title": "Security Engineer",
                       "company": "Acme", "location": "Antwerpen",
                       "description": "We are looking for a pentester with OWASP and Python experience."})
        db.upsert_job({"source": "ictjob", "external_id": "t2", "title": "Junior SOC Analyst",
                       "company": "Beta", "location": "Leuven",
                       "description": "You will work with SIEM tools and handle incident response."})
        score_jobs()
        yield c


def test_frontend_is_served(client):
    r = client.get("/")
    assert r.status_code == 200
    assert "Job Seeker" in r.text
    assert client.get("/static/app.js").status_code == 200


def test_status(client):
    s = client.get("/api/status").json()
    assert s["cv_loaded"] is True
    assert s["counts"]["total"] == 2
    assert s["counts"]["by_status"]["new"] == 2
    assert set(s["counts"]["by_status"]) == db.JOB_STATUSES


def test_cv_skills(client):
    skills = client.get("/api/cv").json()["skills"]
    assert "Penetration testing" in skills


def test_jobs_are_scored_and_sorted(client):
    jobs = client.get("/api/jobs").json()
    assert len(jobs) == 2
    assert all(j["grade"] is not None for j in jobs)
    assert jobs[0]["grade"] >= jobs[1]["grade"]


def test_status_change_updates_counts(client):
    job_id = client.get("/api/jobs").json()[0]["id"]
    assert client.patch(f"/api/jobs/{job_id}", json={"status": "applied"}).json()["status"] == "applied"
    counts = client.get("/api/status").json()["counts"]["by_status"]
    assert counts["applied"] == 1
    assert [j["id"] for j in client.get("/api/jobs?status=applied").json()] == [job_id]


def test_unknown_status_is_rejected(client):
    job_id = client.get("/api/jobs").json()[0]["id"]
    assert client.patch(f"/api/jobs/{job_id}", json={"status": "nope"}).status_code == 400


def test_missing_job_is_404(client):
    assert client.get("/api/jobs/999999").status_code == 404


def test_settings_roundtrip(client):
    r = client.put("/api/settings", json={"keywords": [" pentester ", ""], "max_distance_km": 40})
    assert r.status_code == 200
    s = client.get("/api/settings").json()
    assert s["keywords"] == ["pentester"]
    assert s["max_distance_km"] == 40
