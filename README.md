# Job Scout

A local-first job scouting tool. Scrapes LinkedIn + Adzuna concurrently, runs a
three-stage pipeline (hard title filter → deterministic pre-filter → LLM referee
on borderline cases), and surfaces ranked matches in a React UI. SQLite for
persistence; nothing leaves your machine except the LinkedIn / Adzuna fetches
and Ollama (local).

Built for **Bo Claes**, but the matcher is profile-driven — anyone can adapt it
by editing `profile.md`.

---

## Features

- **Three-stage scoring pipeline**
  1. **Hard title filter** — Senior / Lead / Principal / thesis / internship
     titles are dropped before anything else. They never enter the pipeline.
  2. **Deterministic pre-filter** — every unique scraped job gets a fast
     keyword score; only the top N proceed to hydration and LLM scoring.
     IT/ICT teaching jobs are guaranteed a slot regardless of score.
  3. **LLM referee** — Ollama (`llama3.1:8b`) is consulted only for borderline
     scores in the band `[35, 78]`. Outside that band the deterministic result
     stands. Cuts LLM calls by ~60% on a typical refresh.

- **Transparent score breakdown** — three components: Tech coverage (max 60) +
  Required-tech fit (max 20) + Role title (max 20). Deal-breakers (Senior role,
  5+ years required, security clearance, thesis, internship) cap the total at 25.
  Location is never scored.

- **Curated tech vocabulary** — candidate techs matched with whole-word regex
  that handles `.`, `#`, `/`, `&` boundaries (no more "go above and beyond"
  matching the Go language). Bidirectional aliasing: CI/CD ↔ GitHub Actions,
  Postgres ↔ PostgreSQL, etc.

- **Resilient LinkedIn scraping** — tries the Guest API first, falls back to
  public search HTML; sequential hydration with 2-second delay and 429 circuit
  breaker.

- **SQLite audit trail** — every scraped job is recorded with its deterministic
  score, whether it passed the pre-filter, final score, LLM reason, and run ID.
  Queryable via `/api/audit/runs`.

- **Deduplication** — primary dedup on URL, secondary on (title, company) to
  catch the same job posted under different links.

- **Email drafts** — one click generates a cover email with zero LLM
  hallucination. The best matching project and best matching work experience are
  selected from `profile.md` by counting confirmed-matched techs in each entry.
  Go assembles the email; project and experience paragraphs are verbatim from
  your profile.

- **React frontend** — served directly from the Go server (no separate dev
  server in production). Color-coded tech badges, seniority filter
  (All / Junior / Medior), source filter, real-time SSE progress bar.

---

## Architecture

```
Scrapers (LinkedIn Guest API + Adzuna REST)
         │
         ▼
Hard title filter  ←── drops Senior / Lead / thesis / internship titles
         │
         ▼
Deterministic pre-filter  ←── scores all unique jobs; keeps top N
  (IT/ICT teaching roles guaranteed a slot)
         │
         ▼
Hydration pool  ←── fetches full descriptions (LinkedIn sequential, 2s delay)
         │
         ▼
Scoring pool
  ├── Deterministic score  ←── always runs
  └── Ollama LLM (llama3.1:8b)  ←── only for band [35, 78]
         │
         ▼
SQLite  ←── jobs table + scrape_audit table
         │
         ▼
React UI (served from Go at :8080)
```

SSE events stream to the client as each phase progresses:
`scrape_start` → `source_done` → `dedup` → `pre_filter` → `hydrate_start`
→ `hydrate_progress` → `scoring` → `done`

---

## Scoring rubric

| Component | Max | How it's calculated |
|-----------|-----|---------------------|
| Tech coverage | 60 | Weighted sum of matched candidate techs (primary stack 7 pt, secondary 4 pt, tooling 1 pt), capped at 60 |
| Required-tech fit | 20 | Fraction of techs in the job's requirements section that the candidate has, raised to power 0.7 |
| Role title match | 20 | 20 pt for match in title, 8 pt for body-only mention |
| **Total** | **100** | |

**Deal-breakers:**

| Trigger | Action |
|---------|--------|
| Senior / Lead / Principal / Staff Engineer in **title** | Hard drop — never enters pipeline |
| Thesis student / internship in **title** | Hard drop — never enters pipeline |
| Senior role pattern in **description** | Cap score at 25 |
| 5+ years experience required | Cap score at 25 |
| Security clearance required | Cap score at 25 |
| Thesis / internship in **description** | Cap score at 25 |

---

## Setup

### One-command install

**Windows (PowerShell — run once from the project root):**
```powershell
.\setup.ps1
```

**macOS / Linux:**
```bash
bash install.sh
```

Both scripts install Go, Node.js, and Ollama via the system package manager
(winget / Homebrew / apt), pull `llama3.1:8b`, install npm dependencies, build
the frontend, and copy `.env.example` → `.env`.

### Manual prerequisites
- Go 1.22+
- Node.js 20+
- [Ollama](https://ollama.com) running locally:
  ```bash
  ollama pull llama3.1:8b
  ```

### Configuration

```bash
cp .env.example .env
```

```env
PORT=8080
DB_PATH=jobs.db

OLLAMA_HOST=http://localhost:11434
OLLAMA_MODEL=llama3.1:8b
OLLAMA_NUM_PARALLEL=2        # concurrent LLM scoring calls

# Adzuna Belgium — free at developer.adzuna.com (25k calls/month)
ADZUNA_APP_ID=your_app_id_here
ADZUNA_APP_KEY=your_app_key_here

# Comma-separated skills shown as filter tags in the UI
SKILLS=React,TypeScript,JavaScript,Node.js,CSS,HTML,REST APIs,Git,Vite,Tailwind
```

### Edit your profile

`profile.md` drives everything:

| Section | Purpose |
|---------|---------|
| `## About me` | Free-text bio; tech terms picked up automatically |
| `## Experience` | Work history; best-matching entry used in email drafts |
| `## Projects` | Personal projects; best-matching entry used in email drafts |
| `## Tech stack` | Primary source of canonical tech tokens |
| `## What I'm looking for` | Role keywords, seeded into the LLM prompt |
| `## Search` | Scraping config: `location:`, `city:`, `queries:` list |

---

## Run

**Windows:**
```powershell
.\start.ps1
```

**macOS / Linux:**
```bash
bash start.sh
```

Both scripts start Ollama (if not running), verify the frontend build, and
launch the Go server. Open **http://localhost:8080**.

### Development (hot reload)
```bash
# Terminal 1 — backend
cd server && go run ./cmd/server

# Terminal 2 — frontend (Vite dev server at :5173)
cd client && npm run dev
```

---

## Project layout

```
job-seeker/
├── profile.md                    # candidate profile — edit this
├── .env                          # secrets and config (gitignored)
├── .env.example                  # template
├── setup.ps1                     # Windows one-command installer
├── start.ps1                     # Windows launcher
├── install.sh                    # macOS/Linux one-command installer
├── start.sh                      # macOS/Linux launcher
├── server/
│   ├── cmd/server/main.go        # entry point
│   └── internal/
│       ├── api/
│       │   ├── handlers.go       # SSE refresh, REST endpoints, title filters
│       │   ├── profile.go        # ## Search section parser
│       │   └── router.go         # chi routes, CORS, static file serving
│       ├── matcher/
│       │   ├── profile.go        # candidate index, tech definitions, deal-breakers
│       │   ├── score.go          # deterministic scorer
│       │   ├── llm.go            # Ollama referee + email drafting
│       │   └── cache.go          # in-memory score cache keyed on (job, profile)
│       ├── scrapers/
│       │   ├── linkedin.go       # Guest API + HTML fallback + 429 circuit breaker
│       │   └── adzuna.go         # JSON REST API + radius calculation
│       ├── store/sqlite.go       # jobs table + scrape_audit table
│       ├── models/job.go         # shared types incl. AuditRecord
│       └── debuglog/log.go       # tee stdout → debug.log
└── client/
    ├── src/
    │   ├── App.tsx               # main component, SSE handling, filters
    │   ├── components/           # JobCard, FilterBar, EmailModal, etc.
    │   └── types/job.ts
    └── package.json
```

---

## Tweaking the matcher

**Tech definitions** — `server/internal/matcher/profile.go`, `techDefinitions` slice.
Each entry: `{canonical, aliases, category, weight}`.
- Weight 3 = primary stack (7 pt per match)
- Weight 2 = secondary (4 pt)
- Weight 1 = tooling (1 pt)

**Broad tech vocabulary** — `server/internal/matcher/score.go`, `broadTechSpecs`.
Techs the candidate doesn't have, used only to detect *missing* required skills
in job descriptions. Add entries for techs you keep seeing in postings.

**Deal-breakers** — `server/internal/matcher/profile.go`, `dealBreakerSpecs`.
Each entry is a regex + label; matches in the combined title + description cap
the score at 25.

**LLM band** — `score.go`, `NeedsLLM` field. Currently `[35, 78]`. Widen to
send more jobs to the LLM; narrow to save time.

---

## Why local-first

- **Privacy** — your profile, which jobs you ignored, cover letters: all stay
  on your machine.
- **Cost** — no API bills. Ollama runs locally; LinkedIn and Adzuna are free.
- **Speed** — `llama3.1:8b` with GPU acceleration (RTX 4070 Ti tested) scores
  a full batch of 25 jobs in under two minutes.

---

## License
MIT
