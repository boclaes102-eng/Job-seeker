from datetime import date, timedelta

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


def _new_job(external_id):
    job_id, _ = db.upsert_job({"source": "ictjob", "external_id": external_id, "title": f"Pentester {external_id}",
                               "company": f"Co {external_id}", "location": "Gent",
                               "description": "We are looking for a pentester."})
    return job_id


def test_applying_sets_dates(client):
    job_id = _new_job("apply")
    job = client.patch(f"/api/jobs/{job_id}", json={"status": "applied"}).json()
    today = date.today()
    assert job["applied_on"] == today.isoformat()
    assert job["follow_up_on"] == (today + timedelta(days=db.FOLLOW_UP_AFTER_DAYS)).isoformat()


def test_applying_keeps_dates_you_set(client):
    job_id = _new_job("own-dates")
    job = client.patch(f"/api/jobs/{job_id}",
                       json={"status": "applied", "applied_on": "2026-01-05", "follow_up_on": ""}).json()
    assert job["applied_on"] == "2026-01-05"
    assert job["follow_up_on"] is None  # "" clears it, and an explicit value is never overwritten


def test_tracking_fields_save_and_clear(client):
    job_id = _new_job("fields")
    job = client.patch(f"/api/jobs/{job_id}", json={
        "contact": "  Sam, sam@example.com ", "notes": "Sent CV", "interview_at": "2026-11-06T14:00"}).json()
    assert job["contact"] == "Sam, sam@example.com"
    assert (job["notes"], job["interview_at"]) == ("Sent CV", "2026-11-06T14:00")
    job = client.patch(f"/api/jobs/{job_id}", json={"contact": ""}).json()
    assert job["contact"] is None
    assert job["notes"] == "Sent CV"  # fields left out of the request are untouched


@pytest.mark.parametrize("body", [{"applied_on": "05/01/2026"}, {"interview_at": "2026-11-06"}])
def test_bad_dates_are_rejected(client, body):
    assert client.patch(f"/api/jobs/{_new_job('bad')}", json=body).status_code == 400


def test_agenda_and_followed_up(client):
    job_id = _new_job("agenda")
    yesterday = (date.today() - timedelta(days=1)).isoformat()
    client.patch(f"/api/jobs/{job_id}", json={"status": "applied", "follow_up_on": yesterday})
    assert job_id in [j["id"] for j in client.get("/api/agenda").json()["follow_ups"]]

    job = client.post(f"/api/jobs/{job_id}/followed-up").json()
    assert job["follow_up_on"] > date.today().isoformat()
    assert f"Followed up on {date.today().isoformat()}." in job["notes"]
    assert job_id not in [j["id"] for j in client.get("/api/agenda").json()["follow_ups"]]


def test_agenda_lists_upcoming_interviews_only(client):
    soon, past = _new_job("interview-soon"), _new_job("interview-past")
    tomorrow = (date.today() + timedelta(days=1)).isoformat()
    client.patch(f"/api/jobs/{soon}", json={"status": "interview", "interview_at": f"{tomorrow}T10:00"})
    client.patch(f"/api/jobs/{past}", json={"status": "interview", "interview_at": "2020-01-01T10:00"})
    ids = [j["id"] for j in client.get("/api/agenda").json()["interviews"]]
    assert soon in ids and past not in ids


def test_old_database_gets_tracking_columns(tmp_path, monkeypatch):
    import dataclasses
    import sqlite3
    path = tmp_path / "old.db"
    old_schema = "\n".join(line for line in db.SCHEMA.splitlines()
                           if line.split()[:1] not in ([f] for f in db.TRACKING_FIELDS))
    with sqlite3.connect(path) as conn:  # the schema from before tracking existed
        conn.executescript(old_schema)
    monkeypatch.setattr(db, "CONFIG", dataclasses.replace(db.CONFIG, db_path=path))
    db.init_db()
    with sqlite3.connect(path) as conn:
        cols = {r[1] for r in conn.execute("PRAGMA table_info(jobs)")}
    assert set(db.TRACKING_FIELDS) <= cols


def test_settings_roundtrip(client):
    r = client.put("/api/settings", json={"keywords": [" pentester ", ""], "max_distance_km": 40})
    assert r.status_code == 200
    s = client.get("/api/settings").json()
    assert s["keywords"] == ["pentester"]
    assert s["max_distance_km"] == 40


@pytest.mark.parametrize("a, b", [
    ("https://example.com/jobs/1?utm_source=x#apply", "https://example.com/jobs/1/"),
    ("https://be.jooble.org/desc/55?ckey=pentester", "https://be.jooble.org/desc/55"),
    ("https://www.linkedin.com/jobs/search/?currentJobId=123&keywords=soc",
     "https://www.linkedin.com/jobs/view/senior-soc-analyst-at-acme-123/"),
])
def test_canonical_url_matches_the_same_posting(a, b):
    assert db.canonical_url(a) == db.canonical_url(b)


def test_canonical_url_keeps_ids_in_the_query():
    assert db.canonical_url("https://x.be/job?id=1") != db.canonical_url("https://x.be/job?id=2")


def test_import_adds_a_new_job_and_scores_it(client):
    r = client.post("/api/jobs/import", json={
        "url": "https://careers.example.com/jobs/42?utm_source=linkedin", "title": "Penetration Tester",
        "company": "Example NV", "location": "Mechelen", "posted_at": date.today().isoformat(),
        "text": "We are looking for a pentester. You know OWASP, Burp Suite and Python. " * 3})
    assert r.status_code == 200
    body = r.json()
    assert body["created"] is True
    job = body["job"]
    assert job["source"] == "manual"
    assert job["url"] == "https://careers.example.com/jobs/42"
    assert job["full_description"].startswith("We are looking")
    assert job["grade"] is not None and "Burp Suite" in job["grade_details"]["matched_skills"]


def test_import_same_link_finds_the_job(client):
    found = client.post("/api/jobs/match", json={"url": "https://careers.example.com/jobs/42#top"}).json()["job"]
    assert found and found["title"] == "Penetration Tester"
    assert client.post("/api/jobs/match", json={"url": "https://other.example.com/1"}).json()["job"] is None


def test_import_updates_a_known_job(client):
    job_id = _new_job("enrich")
    r = client.post("/api/jobs/import", json={
        "job_id": job_id, "title": "ignored", "text": "Full text: SIEM, incident response and Python every day."})
    body = r.json()
    assert body["created"] is False and body["job"]["id"] == job_id
    assert "SIEM" in body["job"]["full_description"]


def test_import_needs_title_and_text(client):
    assert client.post("/api/jobs/import", json={"title": " ", "text": "x"}).status_code == 400
    bad_date = {"title": "T", "text": "x", "posted_at": "yesterday"}
    assert client.post("/api/jobs/import", json=bad_date).status_code == 400


def test_new_since_last_visit(client):
    client.post("/api/seen")
    assert client.get("/api/status").json()["counts"]["unseen"] == 0
    _new_job("after-seen")
    status = client.get("/api/status").json()
    assert status["counts"]["unseen"] == 1
    fresh = client.get("/api/jobs", params={"since": status["seen_until"]}).json()
    assert [j["external_id"] for j in fresh] == ["after-seen"]
    client.post("/api/seen")
    assert client.get("/api/status").json()["counts"]["unseen"] == 0
