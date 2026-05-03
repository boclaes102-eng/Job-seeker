# Job Scout

A local-first job scouting tool for the Belgian market. It scrapes LinkedIn and Adzuna against the search queries in your `profile.md`, runs every posting through a two-stage matcher (deterministic scorer + local LLM referee), and tracks your application pipeline. Everything runs on your machine — no subscriptions, no third-party AI APIs, your CV never leaves the box.

## Stack

| Layer | Tech |
|---|---|
| Backend | Go · chi · goquery · SQLite (modernc/sqlite, pure Go) |
| AI scoring | Ollama (default `llama3.2:3b`) — runs locally |
| Job sources | LinkedIn (HTML scraping) · Adzuna Belgium (JSON API) |
| Frontend | React · TypeScript · Vite · Tailwind CSS |

## Why this exists

Generic job boards rank by recency and SEO, not fit. Off-the-shelf AI tools want your CV in their cloud. Job Scout flips that: your profile sits in a Markdown file, scoring runs on a local LLM, and the only network calls go to the job boards themselves.

## How scoring works

The previous version sent every job through `llama3.1:8b` with the entire profile in the prompt — slow, and the LLM would frequently hallucinate gaps the candidate didn't actually have. The current version is a two-stage pipeline:

**Stage 1 — Deterministic scorer (runs on every job, microseconds).** Parses `profile.md` once per refresh into a structured `CandidateIndex`: tech tokens with aliases (`postgres`/`postgresql`/`pg`), role keywords, accepted locations, and deal-breakers. Each job gets a 0–100 score from:

- Tech overlap (max 60) — how many of the candidate's technologies appear in the description, plus a category-breadth bonus
- Role title match (max 20) — strongest signal when the job title contains a target role
- Location (max 10)
- Looking-for alignment (max 10)
- Deal-breakers (cap at 25) — patterns like "10+ years required", "native French speaker", "security clearance"

Crucially, matching uses **whole-word regex**, so "we go above and beyond" doesn't get counted as Go-language experience. (That single bug was responsible for most false positives in the old version.)

**Stage 2 — LLM referee (runs only on borderline jobs).** Scores in the uncertainty band (35–78) get a second look from Ollama. The prompt is small and surgical: the candidate summary (~400 tokens, not the whole profile), the deterministic findings, and the job description. The LLM can adjust the score by at most ±25 — it's a referee, not a re-scorer. On any LLM error, the deterministic score wins; the pipeline never blocks.

A small in-memory cache keyed on `(jobURL, descriptionHash, profileHash)` means re-running a refresh on the same data is instant.

In practice this scores ~70% of jobs without touching the LLM at all, and the remaining ~30% use a smaller default model (`llama3.2:3b`, ~2GB) instead of `llama3.1:8b`. End-to-end refreshes go from minutes to tens of seconds.

## Features

- Multi-source scraping — LinkedIn and Adzuna in parallel goroutines, sequential within each source to respect rate limits
- Search queries pulled directly from `profile.md` — edit them in the UI
- Round-robin balancing across queries so a popular keyword (`developer`) doesn't crowd out narrower ones (`LLM engineer`)
- Filters: source, posting date (today / 3 days / week / month), radius (10–100 km), max jobs
- Pipeline tab: add jobs, mark applied, generate a tailored application email that opens in Gmail compose — written in the job's language (Dutch or English)
- Dismissed jobs are remembered and never resurface
- Profile editor — every section of `profile.md` editable in the browser, including the search query list

## Setup

### Prerequisites

- Go 1.22 or newer
- Node.js 18 or newer
- [Ollama](https://ollama.com/)

```bash
ollama pull llama3.2:3b   # default — ~2GB, fast
# or pull llama3.1:8b if you prefer the larger model
```

### Adzuna API key

Free, instant, no credit card. Sign up at [developer.adzuna.com](https://developer.adzuna.com/), create an app, copy the App ID and App Key.

### Configure

Copy `.env.example` to `.env` in the project root and fill in your keys:

```env
PORT=8080
DB_PATH=jobs.db

OLLAMA_HOST=http://localhost:11434
OLLAMA_MODEL=llama3.2:3b
OLLAMA_NUM_PARALLEL=4

ADZUNA_APP_ID=your_app_id
ADZUNA_APP_KEY=your_app_key
```

### Fill in your profile

Edit `profile.md` (or use the **My Profile** tab once running). Sections that matter:

- `## Tech stack` — primary input for the deterministic matcher
- `## Projects` — `Stack:` lines are also indexed
- `## What I'm looking for` — drives role-keyword and location matching
- `## Search` — `location`, `city`, and the list of search queries

### Run

```bash
# Terminal 1 — Go API
cd server
go run ./cmd/server      # → http://localhost:8080

# Terminal 2 — React frontend
cd client
npm install              # first time only
npm run dev              # → http://localhost:5173
```

For best Ollama throughput on multiple parallel scoring calls:

```bash
OLLAMA_NUM_PARALLEL=4 ollama serve
```

## Architecture

```
profile.md  →  CandidateIndex  (parsed once per refresh)
                  ↓
               Search queries
                  ↓
       ┌──────────┴──────────┐
       │                     │
   LinkedIn               Adzuna
   (goroutine)          (goroutine)
       │                     │
       └──────────┬──────────┘
                  ↓
            Dedup by URL
                  ↓
       Round-robin cap across queries
                  ↓
       ┌──────────┴──────────┐
       │  DeterministicScore │   ← runs on every job
       │  (microseconds)     │
       └──────────┬──────────┘
                  │
            score ∉ [35, 78]?  →  done
                  │
                  ↓ (uncertainty band)
       ┌─────────────────────┐
       │  Ollama refineScore │   ← max ±25 adjustment
       │  (3b model, ~3s)    │
       └──────────┬──────────┘
                  ↓
                SQLite
                  ↓
            React dashboard
```

Dismissed jobs stay in SQLite forever. Future refreshes upsert by URL, so a dismissed posting can never reappear in `new`.

## Project structure

```
job-seeker/
├── server/
│   ├── cmd/server/main.go             # entry point
│   └── internal/
│       ├── api/                       # chi router, SSE stream
│       ├── matcher/
│       │   ├── profile.go             # CandidateIndex parser
│       │   ├── prefilter.go           # deterministic scorer
│       │   ├── ollama.go              # LLM referee + email drafting
│       │   └── cache.go               # in-memory score cache
│       ├── models/
│       ├── scrapers/                  # linkedin.go, adzuna.go
│       └── store/                     # SQLite CRUD
├── client/
│   └── src/
│       ├── components/                # JobCard, FilterBar, PipelineView, ProfileEditor…
│       ├── api/client.ts
│       └── App.tsx
├── profile.md                         # your CV — edit this
├── .env                               # config (gitignored)
└── jobs.db                            # SQLite database (auto-created)
```

## Why Go for the backend

Two reasons. First, the workload is naturally concurrent — scraping multiple sources, each with its own rate limit, and then running an unknown number of LLM calls in parallel. Goroutines + channels + `context.Context` express that more cleanly than `Promise.all` + `AbortController`. Second, range — the frontend is React/TypeScript, so the backend in Go shows the project owns the full polyglot picture rather than reaching for the same hammer twice.
