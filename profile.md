# Bo Claes — Developer Profile

## About me
Full-stack developer, IoT engineer, and cybersecurity analyst based in Langdorp, Belgium.
I build complete systems from the ground up — hardware to application layer.

At MyPitch I was the sole engineer for an AI-powered soccer analytics platform: installing cameras, routing footage through AWS S3/SQS, and writing Python/OpenCV pipelines that extracted per-player metrics by jersey number. At Agilica I built a Three.js drone fleet management tool, rebuilt the company website with a full analytics pipeline (~30% sales increase), and assembled custom embedded hardware including PCB soldering. In 2026 I independently arranged and executed a 3-month security engagement at a public safety facility — running OpenVAS/Nessus scans, auditing Active Directory, conducting an authorised penetration test, and delivering remediation reports.

Currently completing a Cybersecurity Analyst & Engineer programme at Syntra alongside active project work.

- Based in: Langdorp, Belgium
- Background: web development, IoT, embedded systems, offensive security, machine learning
- Open to roles in: full-stack development, data engineering, cybersecurity, AI/ML engineering, IT/ICT teaching
- Languages: Dutch (native), English (professional)

## Experience
- **Fire Station Leuven** (2026 · 3 months) — Cybersecurity Consultant (independent engagement)
  Network scan with OpenVAS/Nessus, full AD audit, authorised penetration test, remediation reports.
- **Agilica** (2024–2025) — Fullstack Developer & Software/Hardware Engineer
  Three.js drone fleet app, website rebuild, PCB soldering, embedded hardware for drone systems.
- **Karel de Grote Hogeschool** (2023–2024) — Lecturer, Programming & IoT
  Delivered full programming and IoT curriculum to bachelor students; mentored project teams.
- **MyPitch** (2022) — Software & Infrastructure Engineer
  Sole engineer for AI soccer analytics platform: AWS S3/SQS pipeline, Python/OpenCV player tracking.

## Education
- Cybersecurity Analyst & Engineer — Syntra (2025–2026, currently enrolled)
- Professional Bachelor Multimedia & Creative Technologies: Web Development — Karel de Grote (2022–2024)
- Graduate Internet of Things — Karel de Grote (2020–2022)

## Projects

### Three-platform cybersecurity ecosystem
CyberOps Dashboard (Next.js · Vercel) → Threat Intel Platform (Fastify · PostgreSQL · BullMQ · Redis · Railway) → CyberSuite Pro Desktop (Python · CustomTkinter · Windows). Recon results flow from the browser dashboard through a server-side proxy into a shared PostgreSQL backend and load into the desktop attack tool with one click.

### CyberOps Dashboard
56 integrated security tools — Next.js 15, TypeScript strict, Vercel. All algorithms from scratch: Wagner-Fischer edit distance, MurmurHash3, CVSS v3.1 base score, sliding-window rate limiter at Vercel Edge. Zero any-escapes in strict TypeScript; all API keys server-side.

### Threat Intel Platform
Production Fastify API — PostgreSQL 16, Redis 7, three BullMQ workers: CVE feed (NIST NVD), IOC enrichment (AbuseIPDB + VirusTotal + AlienVault OTX in parallel), asset-scan (CPE→CVE correlation). Docker multi-target builds, full CI pipeline with real Postgres/Redis containers.
Stack: Fastify · TypeScript · PostgreSQL · Redis · BullMQ · Drizzle ORM · Docker · Railway · Vitest

### CyberSuite Pro
15-module penetration testing toolkit — network discovery, MITM, SSL intercept, Metasploit bridge, AD enumeration via pure-Python LDAP, YARA + MITRE ATT&CK malware analysis, reporting. 147 pytest tests, ships as single-file .exe.
Stack: Python · CustomTkinter · Scapy · Nmap · mitmproxy · ldap3 · YARA · MITRE ATT&CK · PyInstaller

### The Deep Space Project
Fully interactive 3D portfolio — thedeepspaceproject.be. Three.js space scene with multiplayer Pong (Supabase Realtime, authoritative physics + lerp interpolation), procedural jukebox, browser-side SIEM. Zero npm dependencies shipped to browser.
Stack: Three.js · Vanilla JS · Supabase · Web Audio API · GSAP · Groq AI · GLSL

### Real-Time Data Pipeline
3 async producers (stocks, crypto, Reddit sentiment) → Kafka-compatible async broker → SQLite → WebSocket fan-out → live Chart.js dashboard. Swapping in Confluent Kafka = one file change.
Stack: Python · FastAPI · asyncio · WebSockets · SQLite · Chart.js

### Telecom Churn Predictor
XGBoost classifier — ROC-AUC 0.84, 79.7% accuracy. IBM Telco dataset, 7,043 customers, 20 features. Live Streamlit UI with probability gauge, risk verdict, churn driver bullets, feature importances.
Stack: XGBoost · Streamlit · scikit-learn · pandas

### PyMind — AI Python Assistant
Agentic CLI powered by Claude Sonnet with tool use and prompt caching. Reads codebases, searches code, runs Python snippets; agentic loop chains multiple tool calls until complete.
Stack: Python · Anthropic Claude API · Tool use · Prompt caching

### Job Scout
Full-stack job scouting tool — Go backend scrapes job boards concurrently using goroutines, scores with local Ollama LLM, stores in SQLite; React + TypeScript frontend.
Stack: Go · chi · SQLite · Ollama · React · TypeScript · Vite · Tailwind

### Sub-Checker
Scans 2 years of Gmail to detect recurring subscriptions. Fully local — nothing leaves the machine.
Stack: Python · Gmail API · OAuth2

## Tech stack
- **Languages**: Python · TypeScript · JavaScript (ES2022) · PHP · C# · SQL · Bash · Go
- **Backend**: Fastify · Next.js 15 · FastAPI · asyncio · WebSockets · REST · PostgreSQL · Redis · AWS · Node.js
- **Frontend**: Three.js · React · Tailwind CSS · Vanilla JS · Chart.js · HTML/CSS · GLSL
- **ML / Data**: XGBoost · scikit-learn · OpenCV · pandas · Streamlit · Kafka · asyncio pipelines
- **Security**: Scapy · Nmap · mitmproxy · ldap3 · YARA · MITRE ATT&CK · OpenVAS · Nessus · Burp Suite · Metasploit · Wireshark
- **IoT / Hardware**: Raspberry Pi · Arduino · PCB Design · Soldering · Firmware · Node-RED
- **Infra / DevOps**: Docker · Railway · Vercel · BullMQ · GitHub Actions · Prometheus · Grafana
- **AI / APIs**: Anthropic Claude · Groq (Llama 3.1) · Supabase · VirusTotal · Shodan · AbuseIPDB
- **Tooling**: Git · Linux · Drizzle ORM · PyInstaller · pytest · Vitest

## What I'm looking for
Medior-level role where I can own real features end-to-end — not just tickets, but actual technical decisions. Sweet spot: full-stack product engineering meets the infrastructure/security layer.

Roles that fit well:
- **Full-stack developer** — React/Next.js/TypeScript frontend + Fastify/FastAPI/Node.js backend, ideally with a PostgreSQL or Redis backbone; comfortable taking a feature from Figma to production including CI and deployment config
- **Backend / data engineer** — async pipelines, message queues (BullMQ, Kafka), worker architectures, REST or WebSocket APIs; Python or TypeScript, doesn't matter
- **Security-focused developer / DevSecOps** — building security tooling, integrating SIEM pipelines, writing scanners, or hardening production systems; hands-on pentest experience and know how attacks work at the code level
- **IoT / embedded + software hybrid** — firmware AND cloud backend AND dashboard; exactly what I did at Agilica and MyPitch
- **AI / ML engineer** — agentic tools, LLM integration, data pipelines, predictive models; shipped a Claude-powered agentic CLI and an XGBoost churn predictor with live Streamlit UI
- **IT/ICT teacher or lecturer** — programming, web development, IoT, and cybersecurity for secondary schools, colleges, or training centres; taught a full bachelor-level curriculum at Karel de Grote Hogeschool, mentored project teams, genuinely enjoy explaining technical concepts

What I care about:
- Code review actually happens
- CI pipeline exists and is respected
- Engineers own their deployments
- Mentorship or growth path — I level up fast

Location: Leuven area, Belgium. On-site in Leuven/Brussels/Mechelen/Hasselt is fine, hybrid preferred, full remote also works. Not relocating outside Belgium.
Seniority: medior. Shipped complete production systems solo, led technical decisions, mentored students — but no 10-year career and I'm upfront about that.

## Search
location: België
city: Leuven
queries:
- full-stack developer
- frontend developer
- web developer
- backend developer
- React developer
- JavaScript developer
- TypeScript developer
- Next.js developer
- Node.js developer
- Python developer
- cybersecurity analyst
- security engineer
- penetration tester
- SOC analyst
- DevSecOps engineer
- security consultant
- software engineer
- data engineer
- data pipeline engineer
- ML engineer
- machine learning engineer
- AI engineer
- LLM engineer
- DevOps engineer
- cloud engineer
- IoT engineer
- embedded software engineer
- API developer
- Go developer
- Fastify developer
- IT teacher
- ICT teacher
- programming teacher
- lecturer IT
