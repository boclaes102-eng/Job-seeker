# Job Scout

A full-stack job scouting tool that scrapes Belgian job boards, scores every posting against your profile using a local LLM, and manages your application pipeline — all running locally with no subscriptions.

---

## Stack

| Layer | Technology |
|---|---|
| Backend | Go · chi · goquery · gofeed · SQLite |
| AI scoring | Ollama (llama3.1:8b) — runs locally, free |
| Job sources | LinkedIn (geoId Belgium) · Adzuna Belgium API |
| Frontend | React · TypeScript · Vite · Tailwind CSS |

---

## Features

- **Multi-source scraping** — LinkedIn and Adzuna Belgium searched in parallel using Go goroutines
- **30 search queries** pulled from `profile.md` — covers full-stack, backend, security, data engineering, DevOps, IoT, AI/ML
- **Balanced job cap** — max N jobs distributed proportionally across all query categories so no single domain dominates
- **AI scoring** — Ollama reads the job description + your tech stack and experience, scores 0–100, writes a 2–3 sentence explanation in the job's language (Dutch or English)
- **Date + radius + source filters** — scrape from the last 3 days / this week / this month within 10–100 km
- **Pipeline tab** — add interesting jobs, mark applied, and generate a personalised application email via Ollama that opens directly in Gmail compose
- **Ignored jobs are blacklisted** — dismissed postings are stored and never returned by future searches
- **Profile editor** — edit your CV, projects, tech stack, and search queries directly in the app

---

## Getting started

### 1. Prerequisites

- [Go 1.22+](https://go.dev/dl/)
- [Node.js 18+](https://nodejs.org/)
- [Ollama](https://ollama.com/) with `llama3.1:8b` pulled

```bash
ollama pull llama3.1:8b
```

### 2. Adzuna API key (free)

Sign up at [developer.adzuna.com](https://developer.adzuna.com/) — instant, no credit card. Create an app and copy your App ID and App Key.

### 3. Configure `.env`

```env
PORT=8080
DB_PATH=jobs.db

OLLAMA_HOST=http://localhost:11434
OLLAMA_MODEL=llama3.1:8b
OLLAMA_NUM_PARALLEL=4

ADZUNA_APP_ID=your_app_id
ADZUNA_APP_KEY=your_app_key
```

### 4. Fill in your profile

Edit `profile.md` — or use the **My Profile** tab in the app. The key sections are:

- **Tech stack** — the primary input for AI scoring
- **Experience** and **Projects** — give the model full context
- **What I'm looking for** — used to calibrate fit scores
- **Search** — `city`, `location`, and the list of `queries` to scrape

### 5. Run

**Terminal 1 — Go API:**
```bash
cd server
go run ./cmd/server
# → http://localhost:8080
```

**Terminal 2 — React frontend:**
```bash
cd client
npm install   # first time only
npm run dev
# → http://localhost:5173
```

For faster Ollama scoring, start Ollama with parallel mode enabled:
```powershell
$env:OLLAMA_NUM_PARALLEL=4; ollama serve
```

---

## How it works

```
profile.md
  └── Search queries (30 by default)
        │
        ├── LinkedIn goroutines (all queries in parallel, 10 results/query)
        └── Adzuna API calls (sequential, 700ms delay — rate limit)
              │
              ▼
        Dedup by URL
              │
              ▼
        Balanced cap (N jobs, proportional per query)
              │
              ▼
        Ollama scores each job (concurrently, up to OLLAMA_NUM_PARALLEL)
              │
              ▼
        SQLite (status: new / pipeline / applied / dismissed)
              │
              ▼
        React dashboard
```

**Dismissed jobs** are permanently stored in SQLite. Every future scrape checks the database and skips any URL already marked dismissed — they never reappear.

---

## Project structure

```
job-seeker/
├── server/
│   ├── cmd/server/main.go          # entry point
│   └── internal/
│       ├── api/                    # chi router, SSE stream, handlers
│       ├── matcher/                # Ollama scoring + email drafting
│       ├── models/                 # Job struct
│       ├── scrapers/               # LinkedIn, Adzuna
│       └── store/                  # SQLite CRUD
├── client/
│   └── src/
│       ├── components/             # JobCard, FilterBar, PipelineView, ProfileEditor...
│       ├── api/client.ts           # typed fetch wrappers
│       └── App.tsx
├── profile.md                      # your CV — edit this
├── .env                            # API keys and config
└── jobs.db                         # SQLite database (auto-created)
```

---

## Why Go for the backend?

The backend is intentionally written in Go rather than Node.js/TypeScript (the frontend stack) to demonstrate range. Go's goroutines make the concurrent scraping clean and efficient — 30 queries × 2 sources = 60 goroutines firing simultaneously with zero callback hell.
