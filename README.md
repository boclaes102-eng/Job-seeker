# Job Scout

A local-first job scouting tool. Scrapes LinkedIn + Adzuna concurrently, scores
postings against your profile with a fast deterministic algorithm + a local
Ollama LLM referee on borderline cases, and surfaces the best matches in a
React UI. SQLite for persistence; nothing leaves your machine except the
LinkedIn / Adzuna fetches and Ollama (local).

Built for **Bo Claes**, but the matcher is profile-driven so anyone can adapt
it by editing `profile.md`.

## Features

- **Two-stage scoring pipeline** — fast deterministic prefilter on every job;
  Ollama LLM consulted only on borderline scores (band 35–78), not every job.
  Cuts LLM calls by ~60% on a typical refresh.
- **Pipelined hydration + scoring** — LinkedIn descriptions are fetched
  concurrently with scoring; jobs flow through a channel so scoring kicks off
  the moment a description is ready.
- **Resilient LinkedIn scraping** — tries the lightweight Guest API first,
  falls back to the public search HTML; retries with backoff on 429/503.
- **Curated tech vocabulary** — 67 candidate techs + 50 broad-vocabulary techs,
  whole-word matching that handles `.`, `#`, `/`, `&` boundaries correctly
  (no more "go above and beyond" matching the Go language).
- **Score breakdown** — every score has a transparent rubric: Tech (max 60),
  Required-tech fit (max 20), Role title (max 20). Deal-breakers cap at 25.
  Location is **not** scored — you decide for yourself if a city is too far.
- **At-a-glance UI** — color-coded tech badges per category on every card,
  stale indicator for jobs >7 days old, real-time SSE progress while refreshing.
- **AI email drafts** — one click generates a tailored cover-email in the
  language of the posting, opens in Gmail compose.

## Architecture

```
┌────────────┐  ┌──────────┐  ┌─────────────┐  ┌────────┐  ┌────────┐
│ LinkedIn   │  │ Adzuna   │  │ Hydration   │  │ Score  │  │ SQLite │
│ scraper    │→ │ scraper  │→ │ pool (4×)   │→ │ pool   │→ │        │
│ (Guest API)│  │ (REST)   │  │             │  │ (4×)   │  │        │
└────────────┘  └──────────┘  └─────────────┘  └────────┘  └────────┘
                                       ↓                        ↑
                              fetches missing             ↑
                              descriptions              Ollama
                              concurrent w/scoring     LLM referee
                                                       (borderline only)
                                                       ↑
                                                   profile.md
```

The handler streams SSE events as each phase progresses (`scrape_start`,
`source_done`, `dedup`, `hydrate_start`, `hydrate_progress`, `scoring`, `done`).
The React client renders a multi-stage progress bar from these.

### Scoring rubric

| Component | Max | How it's calculated |
|-----------|-----|---------------------|
| Tech coverage | 60 | weighted sum of matched candidate techs (primary stack 7pt, secondary 4pt, tooling 1pt), capped at 60 |
| Required-tech fit | 20 | fraction of techs in the job's "Profile & Skills" section that the candidate has, raised to power 0.7 |
| Role title match | 20 | 20pt for title match, 8pt for body-only mention |
| **Total** | **100** | |
| Deal-breakers | cap | "10+ years required", "native French speaker required", "security clearance required" → cap total at 25 |

The uncertainty band `[35, 78]` triggers an LLM call. Below: clear no.
Above: clear yes. The LLM can adjust by ±25 points.

## Setup

### Prerequisites
- Go 1.22+ (tested on 1.22 and 1.26)
- Node.js 20+
- [Ollama](https://ollama.com) running locally with a small model:
  ```bash
  ollama pull llama3.2:3b
  ```

### Configuration
Copy `.env.example` to `.env`:
```bash
cp .env.example .env
```

Edit `.env`:
```env
# Adzuna (free trial — register at https://developer.adzuna.com)
ADZUNA_APP_ID=your_id
ADZUNA_APP_KEY=your_key

# Ollama (defaults shown — change if needed)
OLLAMA_HOST=http://localhost:11434
OLLAMA_MODEL=llama3.2:3b
OLLAMA_NUM_PARALLEL=4    # concurrent LLM calls during scoring
HYDRATE_CONCURRENCY=4    # concurrent LinkedIn detail fetches

# Optional
PORT=8080
DB_PATH=jobs.db
LOG_PATH=debug.log
```

### Edit your profile
`profile.md` drives the matcher. Edit it directly or use the in-app Profile
editor. The relevant sections:

- `## About me` — free-text bio. Tech terms here are picked up automatically.
- `## Tech stack` — primary source of canonical tech tokens. Use bullet lists
  with `·`, `,`, or `/` separators.
- `## What I'm looking for` — used for role keywords and to seed the LLM prompt.
- `## Search` — controls scraping. `location:`, `city:`, and a list of `queries:`.

The matcher rebuilds the candidate index automatically when you save the
profile (and clears the score cache so the next refresh re-evaluates everything).

## Run

### Server
```bash
cd server
go run ./cmd/server
```

### Client
```bash
cd client
npm install
npm run dev
```

Open http://localhost:5173.

## Project layout

```
job-seeker/
├── profile.md                              # candidate profile (you edit this)
├── server/
│   ├── cmd/server/main.go                  # entry point
│   └── internal/
│       ├── api/
│       │   ├── handlers.go                 # SSE refresh, REST endpoints
│       │   ├── profile.go                  # ## Search section parser
│       │   └── router.go                   # chi routes + CORS
│       ├── matcher/
│       │   ├── profile.go                  # candidate index from profile.md
│       │   ├── score.go                    # deterministic scorer
│       │   ├── llm.go                      # Ollama referee + email drafts
│       │   └── cache.go                    # in-memory (job, profile) cache
│       ├── scrapers/
│       │   ├── linkedin.go                 # Guest API + fallback + retry
│       │   └── adzuna.go                   # JSON REST API
│       ├── store/sqlite.go                 # persistence
│       ├── models/job.go                   # shared types
│       └── debuglog/log.go                 # tee stdout to file
└── client/
    ├── src/
    │   ├── App.tsx                         # main component, SSE handling
    │   ├── api/client.ts                   # typed REST wrappers
    │   ├── components/                     # JobCard, FilterBar, etc.
    │   └── types/job.ts
    └── package.json
```

## Tweaking the matcher

The single biggest lever is `techDefinitions` in `server/internal/matcher/profile.go`.
Each entry is `{canonical, aliases, category, weight}`. Weight 3 = primary
stack (worth 7 points if matched), 2 = secondary (4 points), 1 = tooling
(1 point). Add or remove entries as your stack evolves.

The `broadTechSpecs` table in `score.go` holds techs the candidate doesn't
have — used purely for detecting *missing* required techs in job descriptions
(e.g. "this job requires Vue.js, candidate doesn't have it"). Add entries
when you keep seeing a tech you don't know in job postings.

## Why local-first

- **Privacy** — your profile, the jobs you've ignored, the cover-letters
  you've drafted: all stay on your machine.
- **Cost** — no API bills. Ollama runs on your laptop; LinkedIn/Adzuna are free.
- **Speed** — local LLM has no round-trip; small 3B model + 8GB RAM is enough.

## License
MIT
