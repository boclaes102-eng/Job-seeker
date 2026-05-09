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
Three interconnected tools built as one end-to-end workflow: CyberOps Dashboard (Next.js · Vercel) feeds recon data through a shared PostgreSQL backend (Fastify · Redis · Railway) and into CyberSuite Pro desktop (Python · Windows) — a single click moves results from browser to pentesting toolkit.

### CyberOps Dashboard
56 integrated security tools across OSINT, recon, threat intel, web analysis, forensics, and SIEM — built on Next.js 15 with zero third-party UI libraries; all core algorithms hand-rolled from scratch: Wagner-Fischer edit distance, MurmurHash3, CVSS v3.1 base score, and a sliding-window rate limiter running at Vercel Edge. All API credentials isolated server-side; zero any-escapes in strict TypeScript.
Stack: Next.js 15 · TypeScript (strict) · Tailwind CSS · Vercel · PostgreSQL

### Threat Intel Platform
Production backend API powering the three-platform ecosystem — multi-source IOC enrichment (AbuseIPDB + VirusTotal + AlienVault OTX in parallel with Redis caching), NVD CVE feed sync with exponential backoff, CPE→CVE asset correlation, SIEM event ingestion with 7 detection rules, JWT auth, and a Prometheus metrics endpoint. Three BullMQ workers run continuously on Railway; full CI pipeline tests against real Postgres and Redis containers.
Stack: Fastify · TypeScript · PostgreSQL 16 · Redis 7 · BullMQ · Drizzle ORM · Docker · Railway · Vitest

### CyberSuite Pro
15-module desktop penetration testing toolkit — modules include Network Map (ARP/nmap/SNMP), NIDS with 6 detection algorithms, MITM/ARP Spoofing, SSL Interceptor (mitmproxy), AD Enumeration via pure-Python LDAP, Metasploit Bridge, Static Malware Analyzer (Shannon entropy + YARA + 18 MITRE ATT&CK API mappings), and a full Report Generator. Thread-safe I/O with threading.local(), UAC auto-elevation on startup, 147 pytest tests; ships as a single-file Windows .exe.
Stack: Python · CustomTkinter · Scapy · Nmap · mitmproxy · ldap3 · YARA · MITRE ATT&CK · PyInstaller

### The Deep Space Project
Zero-dependency interactive 3D portfolio at thedeepspaceproject.be — five fully built apps inside a Three.js space scene: a retro PC with 10 security tools, a live TV (Hacker News · crypto · weather), an arcade cabinet with Pong/Galaga/Breakout plus multiplayer Pong (Supabase Realtime, authoritative physics + lerp interpolation), a procedurally synthesised jukebox, and a phone booth. A browser-side SIEM monitors for XSS, SQLi, prototype pollution, path probes, and credential attacks in real time. Zero npm dependencies shipped to browser.
Stack: Three.js · Vanilla JS · Supabase · Groq AI · GSAP · GLSL · Web Audio API

### Real-Time Data Pipeline
Three-tier streaming pipeline: three concurrent async producers (stocks via yfinance, crypto via CoinGecko, Reddit sentiment via PRAW) push into an asyncio.Queue acting as a Kafka-compatible in-process broker, which a consumer drains to aiosqlite and broadcasts via WebSocket fan-out to a live Chart.js dashboard. REST endpoints expose historical snapshots and throughput metrics; migrating to Confluent Kafka requires changing one file.
Stack: Python · FastAPI · asyncio · WebSockets · aiosqlite · Chart.js

### Network Intrusion Detection System
Production-grade Python NIDS with six statistical detection modules: Port Scan (T1046, sliding 60-second windows), SYN Flood (Welford online baselines), DNS Tunneling (Shannon entropy + label length + query rates), ARP Poisoning (MAC binding conflict detection), ICMP Amplification (Smurf + reflection), and 4σ Statistical Baseline Anomaly. Async Scapy packet capture, Rich terminal dashboard, ECS-aligned NDJSON output for SIEM ingestion.
Stack: Python · Scapy · asyncio · Rich · NDJSON (ECS)

### Telecom Churn Predictor
XGBoost classifier trained on the IBM Telco dataset — ROC-AUC 0.84, 79.7% accuracy across 7,043 customers and 20 features. Live Streamlit UI outputs a churn probability gauge, risk verdict, retention recommendations, key churn driver bullets, and full feature importance rankings.
Stack: XGBoost · Streamlit · scikit-learn · pandas

### PyMind — AI Python Assistant
Agentic CLI powered by Claude Sonnet with tool use and prompt caching — reads entire codebases, searches code with grep/glob, executes Python snippets, and chains multiple tool calls in an autonomous loop until the task is complete. Built on the Anthropic API with persistent prompt caching to keep costs low on long sessions.
Stack: Python · Anthropic Claude API · Tool use · Prompt caching

### Job Scout
Full-stack job scouting tool built to automate my own job search — Go backend scrapes LinkedIn and Adzuna concurrently with goroutines, runs a two-stage scoring pipeline (deterministic keyword match + Ollama LLM referee), and surfaces ranked results in a React + TypeScript dashboard. SQLite audit trail logs every scoring decision.
Stack: Go · chi · SQLite · Ollama · React · TypeScript · Vite · Tailwind CSS

### Sub-Checker
Scans 2 years of Gmail to surface recurring subscriptions — fully local, nothing leaves the machine; reads email headers via the Gmail API with OAuth2, groups by sender pattern, and outputs a sorted cost summary.
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
