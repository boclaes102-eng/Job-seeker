package matcher

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"
)

// CandidateIndex is the structured, pre-compiled view of profile.md.
// Built once per refresh and shared (read-only) across every scoring goroutine.
type CandidateIndex struct {
	Tech              []TechToken
	RoleKeywords      []string // canonical role names from "Looking for" / queries
	AcceptedLocations []string // lowercased, e.g. ["leuven","brussels","remote"]
	DealBreakers      []dealBreaker
	Summary           string // compact text fed to the LLM (~400 tokens)
	Hash              string // SHA-256 fingerprint of the full profile
}

// TechToken is one canonical technology with its match patterns.
type TechToken struct {
	Canonical string           // display form, e.g. "TypeScript"
	Aliases   []*regexp.Regexp // case-insensitive whole-word patterns
	Category  string           // language|frontend|backend|data|security|iot|infra|ai|tooling
	Weight    int              // base weight for scoring (1-3)
}

type dealBreaker struct {
	Label   string
	Pattern *regexp.Regexp
}

// ─── Curated alias table ─────────────────────────────────────────────────────
//
// Every entry maps a canonical name → the lowercase strings that should match
// in a job description. Keep aliases minimal and unambiguous. The whole-word
// regex is generated automatically (handles ".", "#", "/" boundaries correctly).
//
// Bare "go" is intentionally NOT a Go-language alias — too many false positives
// ("go above and beyond", "we go"). The Go language only matches "golang".

type techDef struct {
	canonical string
	aliases   []string
	category  string
	weight    int // 1=tooling, 2=secondary, 3=primary stack
}

var techDefinitions = []techDef{
	// Languages — primary
	{"Python", []string{"python", "python3", "py3"}, "language", 3},
	{"TypeScript", []string{"typescript"}, "language", 3},
	{"JavaScript", []string{"javascript", "ecmascript"}, "language", 3},
	{"Go", []string{"golang", "go-lang"}, "language", 3}, // bare "go" excluded
	// Languages — secondary
	{"PHP", []string{"php"}, "language", 2},
	{"C#", []string{"c#", "csharp", "c-sharp", ".net", "dotnet"}, "language", 2},
	{"SQL", []string{"sql"}, "language", 2},
	{"Bash", []string{"bash", "shell scripting"}, "language", 1},

	// Frontend
	{"React", []string{"react", "react.js", "reactjs"}, "frontend", 3},
	{"Next.js", []string{"next.js", "nextjs"}, "frontend", 3},
	{"Three.js", []string{"three.js", "threejs"}, "frontend", 3},
	{"Vue", []string{"vue", "vue.js", "vuejs", "vuex", "nuxt", "nuxt.js"}, "frontend", 2},
	{"Angular", []string{"angular", "angular.js", "angularjs"}, "frontend", 1},
	{"Tailwind CSS", []string{"tailwind", "tailwindcss", "tailwind css"}, "frontend", 2},
	{"Vite", []string{"vite"}, "frontend", 1},
	{"Chart.js", []string{"chart.js", "chartjs"}, "frontend", 1},
	{"HTML", []string{"html", "html5"}, "frontend", 1},
	{"CSS", []string{"css", "css3"}, "frontend", 1},
	{"GLSL", []string{"glsl"}, "frontend", 1},

	// Backend / runtimes
	{"Node.js", []string{"node.js", "nodejs", "node js"}, "backend", 3}, // bare "node" excluded
	{"Fastify", []string{"fastify"}, "backend", 3},
	{"FastAPI", []string{"fastapi", "fast api"}, "backend", 3},
	{"asyncio", []string{"asyncio"}, "backend", 2},
	{"WebSockets", []string{"websockets", "websocket", "web socket"}, "backend", 2},
	{"REST", []string{"rest api", "restful", "rest apis"}, "backend", 2},
	{"GraphQL", []string{"graphql"}, "backend", 2},

	// Data / messaging
	{"PostgreSQL", []string{"postgresql", "postgres", "pg", "psql"}, "data", 3},
	{"Redis", []string{"redis"}, "data", 3},
	{"SQLite", []string{"sqlite"}, "data", 2},
	{"MongoDB", []string{"mongodb", "mongo"}, "data", 2},
	{"Kafka", []string{"kafka"}, "data", 2},
	{"BullMQ", []string{"bullmq", "bull mq"}, "data", 2},
	{"Drizzle ORM", []string{"drizzle", "drizzle orm"}, "data", 1},

	// ML / data science
	{"XGBoost", []string{"xgboost"}, "data", 3},
	{"scikit-learn", []string{"scikit-learn", "sklearn", "scikit learn"}, "data", 3},
	{"OpenCV", []string{"opencv"}, "data", 3},
	{"pandas", []string{"pandas"}, "data", 2},
	{"Streamlit", []string{"streamlit"}, "data", 2},

	// Security
	{"Scapy", []string{"scapy"}, "security", 3},
	{"Nmap", []string{"nmap"}, "security", 3},
	{"mitmproxy", []string{"mitmproxy"}, "security", 3},
	{"ldap3", []string{"ldap3"}, "security", 2},
	{"YARA", []string{"yara"}, "security", 3},
	{"MITRE ATT&CK", []string{"mitre att&ck", "mitre attack", "att&ck framework"}, "security", 2},
	{"OpenVAS", []string{"openvas"}, "security", 3},
	{"Nessus", []string{"nessus"}, "security", 3},
	{"Burp Suite", []string{"burp suite", "burpsuite", "burp"}, "security", 3},
	{"Metasploit", []string{"metasploit"}, "security", 3},
	{"Wireshark", []string{"wireshark"}, "security", 3},
	{"Active Directory", []string{"active directory", "ad audit", "domain controller"}, "security", 2},

	// IoT
	{"Raspberry Pi", []string{"raspberry pi", "rpi"}, "iot", 3},
	{"Arduino", []string{"arduino"}, "iot", 3},
	{"PCB Design", []string{"pcb design", "pcb layout"}, "iot", 3},
	{"Firmware", []string{"firmware"}, "iot", 2},
	{"Node-RED", []string{"node-red", "nodered"}, "iot", 2},

	// Infra / DevOps / cloud
	{"Docker", []string{"docker", "dockerfile"}, "infra", 3},
	{"Kubernetes", []string{"kubernetes", "k8s"}, "infra", 2},
	{"GitHub Actions", []string{"github actions", "github ci", "ci/cd", "ci / cd", "continuous integration", "continuous delivery", "gitlab ci", "gitlab-ci", "jenkins", "circleci", "teamcity", "azure devops pipelines"}, "infra", 2},
	{"CI/CD", []string{"ci/cd", "ci / cd", "continuous integration", "continuous delivery", "github actions", "github ci", "gitlab ci", "jenkins", "circleci"}, "infra", 2},
	{"Vercel", []string{"vercel"}, "infra", 1},
	{"Railway", []string{"railway.app", "railway hosting"}, "infra", 1}, // avoid plain "railway"
	{"Prometheus", []string{"prometheus"}, "infra", 2},
	{"Grafana", []string{"grafana"}, "infra", 2},
	{"AWS", []string{"aws", "amazon web services"}, "infra", 2},
	{"S3", []string{"s3 bucket", "amazon s3"}, "infra", 1},
	{"SQS", []string{"amazon sqs", "sqs queue"}, "infra", 1},

	// AI
	{"Anthropic Claude", []string{"anthropic claude", "claude api", "anthropic api"}, "ai", 3},
	{"Groq", []string{"groq"}, "ai", 2},
	{"Supabase", []string{"supabase"}, "ai", 2},
	{"LLM", []string{"llm", "large language model", "large language models"}, "ai", 2},
	{"Ollama", []string{"ollama"}, "ai", 2},

	// Tooling
	{"Git", []string{"git", "github"}, "tooling", 1},
	{"Linux", []string{"linux"}, "tooling", 1},
	{"pytest", []string{"pytest"}, "tooling", 1},
	{"Vitest", []string{"vitest"}, "tooling", 1},
}

// roleKeywordCanon maps lowercase needles to the canonical role name.
// Used to detect role-title alignment in job titles and bodies.
var roleKeywordCanon = map[string]string{
	"full-stack":           "full-stack developer",
	"fullstack":            "full-stack developer",
	"full stack":           "full-stack developer",
	"front-end":            "frontend developer",
	"frontend":             "frontend developer",
	"back-end":             "backend developer",
	"backend":              "backend developer",
	"web developer":        "web developer",
	"software engineer":    "software engineer",
	"software developer":   "software developer",
	"data engineer":        "data engineer",
	"data pipeline":        "data engineer",
	"machine learning":     "ML engineer",
	"ml engineer":          "ML engineer",
	"ai engineer":          "AI engineer",
	"llm engineer":         "LLM engineer",
	"devops":               "DevOps engineer",
	"devsecops":            "DevSecOps engineer",
	"cloud engineer":       "cloud engineer",
	"site reliability":     "SRE",
	"sre":                  "SRE",
	"iot":                  "IoT engineer",
	"embedded":             "embedded engineer",
	"firmware engineer":    "embedded engineer",
	"cybersecurity":        "cybersecurity analyst",
	"security analyst":     "security analyst",
	"security engineer":    "security engineer",
	"penetration tester":   "penetration tester",
	"pentester":            "penetration tester",
	"soc analyst":          "SOC analyst",
	"security consultant":  "security consultant",
	"api developer":        "API developer",
	"go developer":         "Go developer",
	"react developer":      "React developer",
	"node.js developer":    "Node.js developer",
	"typescript developer": "TypeScript developer",
	"python developer":     "Python developer",
	// Teaching
	"leerkracht":           "IT teacher",
	"docent":               "IT teacher",
	"lecturer":             "IT teacher",
	"it teacher":           "IT teacher",
	"ict teacher":          "IT teacher",
	"lector":               "IT teacher",
	"lesgever":             "IT teacher",
	"instructeur":          "IT teacher",
}

// dealBreakerSpecs are conservative patterns that signal the role is a clear
// mismatch regardless of tech overlap. A match caps the deterministic score
// at 25 (so the job still appears in the list, but at the bottom).
type dealBreakerSpec struct {
	pattern string
	label   string
}

var dealBreakerSpecs = []dealBreakerSpec{
	// Seniority — title-based (checked against title+description in scorer)
	{`\b(senior|sr\.?)\s+(developer|engineer|architect|devops|devsecops|analyst|consultant|fullstack|full.stack|backend|frontend|python|java|node|react|ml|ai|data|cloud|security|software|manager|devops)\b`, "Senior role"},
	{`\bprincipal\s+(engineer|developer|architect|consultant|software)\b`, "Principal/Staff role"},
	{`\bstaff\s+engineer\b`, "Principal/Staff role"},
	// Years of experience
	{`\b([5-9]|1[0-9]|20)\s*\+\s*(years?|jaar|jaren)\b.{0,60}\b(experience|ervaring|expertise|required|vereist)\b`, "5+ years experience required"},
	{`\b(10|11|12|13|14|15|20)\s*\+?\s*(years?|jaar|jaren)\b.{0,40}\b(experience|ervaring|expertise)\b`, "10+ years experience required"},
	// Language / clearance / other hard blocks
	{`\bnative\s+(?:french|français|frans)\b|\bmoedertaal\s+frans\b|\blangue\s+maternelle\s+français`, "Native French speaker required"},
	{`\b(?:security clearance|veiligheidsmachtiging|nato secret|cosmic top secret)\b`, "Security clearance required"},
	{`\b(?:stageopdracht|stageplaats|internship|stagiair|stage\s+student|student\s+stage)\b`, "Internship/stage position"},
	{`\b(?:thesis\s+student|student\s+thesis|thesis\s+worker|bachelorproef|masterproef|eindwerk)\b`, "Thesis/student project"},
}

// ─── Public API ──────────────────────────────────────────────────────────────

// BuildCandidateIndex parses profile.md once and returns a ready-to-use index.
func BuildCandidateIndex(profile string) *CandidateIndex {
	idx := &CandidateIndex{
		Hash: shortHash(profile),
	}

	// Step 1: parse what tech the candidate actually mentions.
	stackTokens := extractStackTokens(profile)

	// Step 2: build the active TechToken set — only entries from the
	// curated table whose aliases overlap with what we found in the profile.
	for _, def := range techDefinitions {
		hit := false
		for _, alias := range def.aliases {
			if stackTokens[alias] {
				hit = true
				break
			}
		}
		// Also match by canonical name (in case the profile uses the canonical form directly).
		if !hit && stackTokens[strings.ToLower(def.canonical)] {
			hit = true
		}
		if !hit {
			continue
		}
		idx.Tech = append(idx.Tech, TechToken{
			Canonical: def.canonical,
			Aliases:   compileAliases(def.aliases),
			Category:  def.category,
			Weight:    def.weight,
		})
	}

	// Step 3: roles + locations from "What I'm looking for" and "Search".
	lookingFor := sectionContent(profile, "What I'm looking for")
	queries := sectionContent(profile, "Search")
	combined := strings.ToLower(lookingFor + "\n" + queries)
	roleSet := map[string]bool{}
	for needle, canon := range roleKeywordCanon {
		if strings.Contains(combined, needle) {
			roleSet[canon] = true
		}
	}
	for r := range roleSet {
		idx.RoleKeywords = append(idx.RoleKeywords, r)
	}
	idx.AcceptedLocations = extractLocations(profile)

	// Step 4: deal-breakers (compiled once).
	for _, s := range dealBreakerSpecs {
		if re, err := regexp.Compile(`(?i)` + s.pattern); err == nil {
			idx.DealBreakers = append(idx.DealBreakers, dealBreaker{Label: s.label, Pattern: re})
		}
	}

	// Step 5: compact summary used by the LLM referee.
	idx.Summary = buildSummary(profile, idx)

	return idx
}

// ─── Profile parsing helpers ─────────────────────────────────────────────────

// extractStackTokens reads "## Tech stack", "Stack:" lines under "## Projects",
// AND scans free-text in "## About me" / "## Experience" / "## Projects" prose
// for tech terms from the curated alias map. Casts a wide net so a tech the
// candidate clearly has experience with (mentioned in their bio) is recognised
// even if it doesn't appear as a comma-separated stack list item.
//
// Splitters: "·", ",", and "/" (so "HTML/CSS" yields both "html" and "css").
// Parentheticals like "JavaScript (ES2022)" are stripped.
func extractStackTokens(profile string) map[string]bool {
	out := map[string]bool{}

	addLine := func(line string) {
		s := strings.TrimSpace(line)
		s = strings.TrimPrefix(s, "- ")
		s = strings.ReplaceAll(s, "**", "")
		// Strip "Foo:" or "Foo / Bar:" style category labels.
		// We want to drop the prefix UP TO the first ": " IF the prefix is
		// short and looks like a label (only letters/spaces/slashes/ampersands).
		if i := strings.Index(s, ": "); i != -1 && i < 30 {
			label := s[:i]
			isLabel := true
			for _, r := range label {
				if !(r == ' ' || r == '/' || r == '&' || r == '-' ||
					(r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')) {
					isLabel = false
					break
				}
			}
			if isLabel {
				s = s[i+2:]
			}
		}
		// Split on the three separators.
		for _, part := range strings.FieldsFunc(s, func(r rune) bool {
			return r == '·' || r == ',' || r == '/'
		}) {
			t := strings.ToLower(strings.TrimSpace(part))
			if i := strings.Index(t, "("); i != -1 {
				t = strings.TrimSpace(t[:i])
			}
			if t == "" {
				continue
			}
			out[t] = true
			// Also record version-stripped: "next.js 15" → "next.js"
			if fields := strings.Fields(t); len(fields) > 1 && isVersionLike(fields[len(fields)-1]) {
				out[strings.Join(fields[:len(fields)-1], " ")] = true
			}
		}
	}

	inStack := false
	inProjects := false
	for _, line := range strings.Split(profile, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## ") {
			head := strings.ToLower(strings.TrimPrefix(trimmed, "## "))
			inStack = head == "tech stack"
			inProjects = head == "projects"
			continue
		}
		if inStack && trimmed != "" {
			addLine(trimmed)
		}
		if inProjects && strings.HasPrefix(strings.ToLower(trimmed), "stack:") {
			addLine(strings.TrimSpace(trimmed[len("stack:"):]))
		}
	}

	// Second pass: scan About / Experience / Projects free text for any alias
	// from the curated table. This catches tech the candidate clearly has but
	// didn't list under "Tech stack" (e.g. "Active Directory" mentioned in About).
	freeText := strings.ToLower(
		sectionContent(profile, "About me") + "\n" +
			sectionContent(profile, "Experience") + "\n" +
			sectionContent(profile, "Projects"),
	)
	for _, def := range techDefinitions {
		for _, alias := range def.aliases {
			if containsWholeWord(freeText, alias) {
				out[alias] = true
			}
		}
	}

	return out
}

// containsWholeWord checks if `needle` appears in `haystack` as a whole word
// (using the same boundary semantics as compileAliases).
func containsWholeWord(haystack, needle string) bool {
	if !strings.Contains(haystack, needle) {
		return false
	}
	// Walk all positions and verify boundaries.
	start := 0
	for {
		idx := strings.Index(haystack[start:], needle)
		if idx == -1 {
			return false
		}
		actual := start + idx
		end := actual + len(needle)
		left := actual == 0 || !isWordChar(rune(haystack[actual-1]))
		right := end == len(haystack) || !isWordChar(rune(haystack[end]))
		if left && right {
			return true
		}
		start = actual + 1
	}
}

func isWordChar(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_'
}

// compileAliases turns each lowercase alias into a regex with manual word
// boundaries. Plain `\b` doesn't anchor at ".", "#", "/" — so we use
// (^|[^a-z0-9_]) ... ([^a-z0-9_+#.]|$) for tokens with non-word chars.
//
// Trade-off: slightly slower than `\b` but correct for "next.js", "c#", ".net".
func compileAliases(aliases []string) []*regexp.Regexp {
	out := make([]*regexp.Regexp, 0, len(aliases))
	for _, a := range aliases {
		quoted := regexp.QuoteMeta(a)
		// (?i) case-insensitive; (^|\W) prefix; (\W|$) suffix.
		// \W matches anything that is not a letter/digit/underscore — wide enough
		// for our tokens, narrow enough that "node-red" matches inside "node-red,git"
		// and "tailwindcss" matches inside "(tailwindcss)".
		pat := `(?i)(?:^|\W)` + quoted + `(?:\W|$)`
		if re, err := regexp.Compile(pat); err == nil {
			out = append(out, re)
		}
	}
	return out
}

func isVersionLike(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r != '.' && (r < '0' || r > '9') {
			return false
		}
	}
	return true
}

// sectionContent returns the body of a `## Heading` section, trimmed.
func sectionContent(profile, heading string) string {
	target := strings.ToLower(heading)
	var b strings.Builder
	in := false
	for _, line := range strings.Split(profile, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## ") {
			head := strings.ToLower(strings.TrimPrefix(trimmed, "## "))
			in = head == target
			continue
		}
		if in {
			b.WriteString(line)
			b.WriteByte('\n')
		}
	}
	return strings.TrimSpace(b.String())
}

// extractLocations finds Belgian cities mentioned in "What I'm looking for"
// and "Search". Falls back to the major Flemish/Brussels tech hubs.
func extractLocations(profile string) []string {
	hay := strings.ToLower(sectionContent(profile, "What I'm looking for") + "\n" + sectionContent(profile, "Search"))
	known := []string{
		"leuven", "brussels", "brussel", "bruxelles", "mechelen", "hasselt",
		"antwerp", "antwerpen", "gent", "ghent", "aarschot", "langdorp",
		"kortrijk", "brugge", "bruges",
	}
	var hits []string
	for _, c := range known {
		if strings.Contains(hay, c) {
			hits = append(hits, c)
		}
	}
	// Always-accepted defaults: remote and any Belgian region.
	hits = append(hits, "remote", "belgi") // "belgi" matches België/Belgium/Belgique
	return uniqueLowered(hits)
}

func uniqueLowered(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(strings.ToLower(s))
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

// buildSummary produces a compact (~400 token) candidate description for LLM
// prompts. We strip the heavy projects + experience details and instead surface
// the canonical tech list so the LLM has a clean ground-truth signal.
func buildSummary(profile string, idx *CandidateIndex) string {
	var b strings.Builder

	// Extract "Based in:" from the About me section for an accurate location header.
	location := "Belgium"
	if about := sectionContent(profile, "About me"); about != "" {
		for _, line := range strings.Split(about, "\n") {
			lc := strings.ToLower(line)
			if strings.Contains(lc, "based in:") {
				if parts := strings.SplitN(line, ":", 2); len(parts) == 2 {
					location = strings.TrimSpace(parts[1])
				}
				break
			}
		}
	}
	b.WriteString("Candidate profile — location: " + location + "\n\n")

	if about := sectionContent(profile, "About me"); about != "" {
		first := firstParagraph(about)
		if len(first) > 600 {
			first = first[:600] + "..."
		}
		b.WriteString("About: ")
		b.WriteString(first)
		b.WriteString("\n\n")
	}

	if len(idx.Tech) > 0 {
		names := make([]string, 0, len(idx.Tech))
		for _, t := range idx.Tech {
			if t.Weight >= 2 { // skip pure tooling like Git/Linux from summary
				names = append(names, t.Canonical)
			}
		}
		if len(names) > 0 {
			if len(names) > 30 {
				names = names[:30]
			}
			b.WriteString("Tech (verbatim ground truth — never claim the candidate lacks any of these): ")
			b.WriteString(strings.Join(names, ", "))
			b.WriteString("\n\n")
		}
	}

	if len(idx.RoleKeywords) > 0 {
		b.WriteString("Target roles: ")
		b.WriteString(strings.Join(idx.RoleKeywords, ", "))
		b.WriteString("\n\n")
	}

	if lf := sectionContent(profile, "What I'm looking for"); lf != "" {
		if len(lf) > 600 {
			lf = lf[:600] + "..."
		}
		b.WriteString("Wants: ")
		b.WriteString(lf)
		b.WriteString("\n")
	}

	return b.String()
}

func firstParagraph(s string) string {
	if i := strings.Index(s, "\n\n"); i != -1 {
		return strings.TrimSpace(s[:i])
	}
	return strings.TrimSpace(s)
}

func shortHash(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:8])
}
