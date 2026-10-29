# Job Seeker

A local job-hunting assistant for Belgium. It searches Belgian job sites, compares every job with your CV, and shows the best matches first, each with a link to apply.

It runs entirely on your own computer. There is no AI service, no subscription and no cost. Your CV and job database stay in the project folder and are never committed to git.

## What it does

1. **Search.** It queries every enabled job source for each of your search keywords. Jobs older than the age limit are skipped, and the same posting found on two sites is only kept once.
2. **Filter.** Only English and Dutch jobs are kept. French postings and jobs that require French are filtered out.
3. **Score.** Every job gets a score from 0 to 100 for how well it fits **your CV**. The app reads your skills directly from `CV.pdf`.
4. **Show.** Jobs are listed from best to worst match. Each one shows:
   - which of your skills it asks for;
   - which tools it asks for that are related to yours;
   - which skills you don't have;
   - how far it is from home;
   - an **Apply** link to the original posting.
5. **Track.** Mark jobs as saved, applied, interview, rejected or hidden.

### How the score works

| Part | Weight | What it measures |
|---|---|---|
| Role | 40% | Is the job title one of the roles your CV points to? Security, pentesting and SOC rank highest, then full stack, IoT/embedded, development and infrastructure. |
| Skills | 30% | How many of your CV skills the posting asks for, weighted by importance. A related tool counts half, for example Splunk when your CV says SIEM. |
| Coverage | 15% | Of all the skills the posting names, how many you have. |
| Location | 15% | Distance from the nearest of your home towns (Langdorp and Antwerp by default). Remote or hybrid work is a bonus. |

Some things change the score after that:
- **Lower:** senior, lead and manager titles; internships; jobs that ask many more years of experience than you have.
- **Higher:** junior and medior titles.

Score labels: **80+** strong match, **65+** good match, **45+** worth a look.

Job sites only share a short snippet of each posting. Open a job and paste the full posting text, or click **Try to fetch full description**. The job is then re-scored on the complete text.

## Job sources

| Source | What it covers | Key needed |
|---|---|---|
| [ictjob.be](https://www.ictjob.be) RSS | Belgium's largest IT-specific job board | No |
| [Le Forem open data](https://www.odwb.be/explore/dataset/offres-d-emploi-forem/) | All Forem job offers. Mostly French, so few remain after the language filter | No |
| [Jooble](https://jooble.org/api/about) | Aggregator with wide Belgian coverage | Free key, for Belgium |
| [Adzuna Belgium](https://developer.adzuna.com/) | Aggregates many Belgian job boards | Paid: 25 test calls, then £8/month |

Jooble keys are tied to one country's job index. Use a key for Belgium with `JOOBLE_HOST=be.jooble.org`, which is the default.

These sources are left out on purpose:
- **LinkedIn, Indeed and StepStone** forbid scraping in their terms of use. Jooble picks up many of their postings legally.
- **VDAB** only opens its vacancy API to contracted partners.

### English and Dutch only

French jobs are filtered out in two ways:

1. **Posting language.** Each posting's language is detected. Many Brussels jobs have an English title but a French text, and those are caught too.
2. **French requirement.** English or Dutch postings that ask for French are hidden, for example "fluent in Dutch and French" or "tweetalig". Wording where Dutch alone is enough is kept, such as "Dutch or French" or "French is a plus".

You can change this under **Settings → Job languages**.

## Setup (Windows)

You need Python 3.11 or newer.

```powershell
cd Job-seeker
python -m venv .venv
.venv\Scripts\activate
pip install -r requirements.txt

copy .env.example .env      # optional: add your Jooble key
python run.py
```

Put your CV in the project folder as `CV.pdf`. Then open http://127.0.0.1:8000 and click **Search now**.

On macOS or Linux, use `source .venv/bin/activate`, `cp` instead of `copy`, and forward slashes.

### Tests

```powershell
pip install -r requirements-dev.txt
python -m pytest
ruff check .
```

The tests use a temporary data folder and a generated test CV, so they never touch your `CV.pdf` or job database, and they make no network calls. GitHub Actions runs the same checks on every push and pull request: see `.github/workflows/ci.yml`.

### Updating your CV

Replace `CV.pdf` with your newest version. The app re-reads it automatically and re-scores all jobs. **Settings → Your CV** shows which skills it found.

If a skill on your CV is not recognised, add it to the vocabulary in `jobseeker/skills.py`.

## Settings

The **Settings** tab holds:
- search keywords;
- your home towns, comma separated, and the maximum travel distance. The distance is measured from whichever home is nearest to the job;
- your years of experience;
- the maximum job age;
- job languages;
- which sources are enabled.

Saving re-scores all jobs.

The **`.env`** file holds API keys and technical options. See `.env.example`.

| Variable | Default | Meaning |
|---|---|---|
| `JOOBLE_API_KEY` | | Enables Jooble |
| `JOOBLE_HOST` | `be.jooble.org` | Must match your Jooble key's country |
| `ADZUNA_APP_ID`, `ADZUNA_APP_KEY` | | Enables Adzuna |
| `CV_PDF` | `CV.pdf` | Path to your CV |
| `AUTO_SEARCH_HOURS` | `0` | Search automatically every N hours while the app runs |

## Project structure

```
run.py                     start the web app
jobseeker/
  api.py                   FastAPI routes (JSON API + serves the frontend)
  pipeline.py              a search run: fetch -> store -> score
  sources/                 one module per job site (ictjob, forem, jooble, adzuna)
  cv.py                    reads CV.pdf and extracts your skills
  skills.py                skill vocabulary: aliases (EN/NL), weights, related tools
  matching.py              the scoring engine
  geo.py                   Belgian towns and postcodes -> distance from home
  language.py              English / Dutch / French detection + "French required" check
  enrich.py                best-effort full description download
  db.py                    SQLite schema and queries
frontend/                  single-page UI (plain HTML/CSS/JS, no build step)
data/                      jobs.db (git-ignored)
```

**Stack:**
- Python with FastAPI for the backend;
- SQLite for the database, a single file with no server;
- plain HTML, CSS and JavaScript for the frontend.

## Privacy

`.gitignore` excludes `data/`, `CV.pdf`, every PDF and `.env`. Before you push this repository anywhere, run `git status` and check that no personal files are listed.

The only data that leaves your machine is your search keywords, which are sent to the job sites. Your CV never leaves your computer.

## Limitations

- Scores are based on keywords and rules, not on a real reading of the text. They are good for ranking and filtering, but read the posting before you apply.
- Feed descriptions are short. Paste the full posting for a more accurate score.
- Distances are approximate: they are based on the town or postcode region, with a road-distance factor.
