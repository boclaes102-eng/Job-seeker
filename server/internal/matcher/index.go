package matcher

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"
)

// TechToken represents one technology from the candidate's stack.
// Aliases are compiled regexes that match the technology in job descriptions.
type TechToken struct {
	Canonical string
	Category  string
	Aliases   []*regexp.Regexp
}

// DealBreaker is a technology pattern that, if present in a job as a primary
// requirement, caps the deterministic score at 25 (clear domain mismatch).
type DealBreaker struct {
	Pattern *regexp.Regexp
	Label   string
}

// CandidateIndex is built once per scrape from profile.md and shared across
// all scoring goroutines. It is immutable after construction.
type CandidateIndex struct {
	TechTokens        []TechToken
	DealBreakers      []DealBreaker
	AcceptedLocations []string
	RoleKeywords      []string // for logging / display
	Hash              string   // SHA-256 of profile, used for cache invalidation
	Summary           string   // compact profile text sent to the LLM
}

// roleKeywordCanon maps lowercase needles to canonical role names.
// Used by DeterministicScore to detect role-title alignment.
var roleKeywordCanon = map[string]string{
	"full-stack":          "full-stack developer",
	"fullstack":           "full-stack developer",
	"full stack":          "full-stack developer",
	"frontend developer":  "frontend developer",
	"front-end developer": "frontend developer",
	"backend developer":   "backend developer",
	"back-end developer":  "backend developer",
	"software engineer":   "software engineer",
	"software developer":  "software developer",
	"web developer":       "web developer",
	"data engineer":       "data engineer",
	"data scientist":      "data scientist",
	"machine learning":    "ml/ai engineer",
	"ml engineer":         "ml/ai engineer",
	"ai engineer":         "ai engineer",
	"llm engineer":        "llm engineer",
	"devops engineer":     "devops engineer",
	"cloud engineer":      "cloud engineer",
	"security engineer":   "security engineer",
	"cybersecurity":       "cybersecurity",
	"penetration tester":  "penetration tester",
	"pentest":             "penetration tester",
	"iot engineer":        "iot engineer",
	"embedded":            "embedded engineer",
	"api developer":       "api developer",
	"react developer":     "react developer",
	"python developer":    "python developer",
	"go developer":        "go developer",
	"golang":              "go developer",
	"node.js developer":   "node.js developer",
	"typescript developer": "typescript developer",
}

// BuildCandidateIndex parses profile.md and returns an immutable index
// ready for concurrent use by the scoring pipeline.
func BuildCandidateIndex(profile string) *CandidateIndex {
	h := sha256.Sum256([]byte(profile))

	idx := &CandidateIndex{
		Hash:              hex.EncodeToString(h[:]),
		TechTokens:        parseTechTokens(profile),
		DealBreakers:      staticDealBreakers(),
		AcceptedLocations: parseAcceptedLocations(profile),
		Summary:           buildSummary(profile),
	}

	for needle := range roleKeywordCanon {
		idx.RoleKeywords = append(idx.RoleKeywords, needle)
	}
	return idx
}

// parseTechTokens extracts every technology from ## Tech stack and compiles
// regex aliases for each one.
func parseTechTokens(profile string) []TechToken {
	categoryMap := map[string]string{
		"languages":      "language",
		"backend":        "backend",
		"frontend":       "frontend",
		"ml":             "data",
		"data":           "data",
		"security":       "security",
		"iot":            "iot",
		"hardware":       "iot",
		"infra":          "infra",
		"devops":         "infra",
		"ai":             "ai",
		"apis":           "ai",
		"tooling":        "tooling",
	}

	var tokens []TechToken
	seen := map[string]bool{}
	currentCategory := "other"
	inStack := false

	for _, line := range strings.Split(profile, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## Tech stack") {
			inStack = true
			continue
		}
		if inStack && strings.HasPrefix(trimmed, "## ") {
			break
		}
		if !inStack || trimmed == "" {
			continue
		}

		trimmed = strings.TrimPrefix(trimmed, "- ")
		trimmed = strings.ReplaceAll(trimmed, "**", "")

		if idx := strings.Index(trimmed, ": "); idx != -1 {
			label := strings.ToLower(trimmed[:idx])
			for k, v := range categoryMap {
				if strings.Contains(label, k) {
					currentCategory = v
					break
				}
			}
			trimmed = trimmed[idx+2:]
		}

		for _, part := range strings.FieldsFunc(trimmed, func(r rune) bool {
			return r == '·' || r == ','
		}) {
			part = strings.TrimSpace(part)
			if pi := strings.Index(part, "("); pi != -1 {
				part = strings.TrimSpace(part[:pi])
			}
			if part == "" || len(part) < 2 {
				continue
			}
			lc := strings.ToLower(part)
			if seen[lc] {
				continue
			}
			seen[lc] = true

			aliases := compileAliases(part)
			if len(aliases) > 0 {
				tokens = append(tokens, TechToken{
					Canonical: part,
					Category:  currentCategory,
					Aliases:   aliases,
				})
			}
		}
	}
	return tokens
}

// compileAliases returns regexes that match a technology name and its common
// variants inside a job description.
func compileAliases(tech string) []*regexp.Regexp {
	lc := strings.ToLower(tech)

	// Per-technology alias overrides
	overrides := map[string][]string{
		"python":       {`\bpython\b`, `\bpython3\b`},
		"javascript":   {`\bjavascript\b`, `\bjs\b`, `\becmascript\b`},
		"typescript":   {`\btypescript\b`, `\bts\b`},
		"go":           {`\bgolang\b`, `\bgo\b`},
		"c#":           {`\bc#\b`, `\bcsharp\b`, `\bc sharp\b`},
		"node.js":      {`node\.?js`, `\bnodejs\b`},
		"next.js":      {`next\.?js`, `\bnextjs\b`},
		"react":        {`\breact\b`, `react\.?js`},
		"three.js":     {`three\.?js`, `\bthreejs\b`},
		"fastapi":      {`fastapi`, `fast api`},
		"postgresql":   {`postgresql`, `\bpostgres\b`},
		"tailwind css": {`tailwind`, `tailwindcss`},
		"github actions": {`github actions`, `github ci`},
		"aws":          {`\baws\b`, `amazon web services`},
		"docker":       {`\bdocker\b`, `dockerfile`},
		"kubernetes":   {`\bkubernetes\b`, `\bk8s\b`},
		"burp suite":   {`burp suite`, `burpsuite`},
		"mitre att&ck": {`mitre att.?ck`, `att.?ck framework`},
	}

	if patterns, ok := overrides[lc]; ok {
		var result []*regexp.Regexp
		for _, p := range patterns {
			if re, err := regexp.Compile(`(?i)` + p); err == nil {
				result = append(result, re)
			}
		}
		return result
	}

	// Default: exact match (case-insensitive, whole-word for short names)
	escaped := regexp.QuoteMeta(lc)
	if len(lc) <= 4 {
		escaped = `\b` + escaped + `\b`
	}
	if re, err := regexp.Compile(`(?i)` + escaped); err == nil {
		return []*regexp.Regexp{re}
	}
	return nil
}

// staticDealBreakers returns patterns for technologies clearly outside the
// candidate's domain. A match caps the deterministic score at 25.
func staticDealBreakers() []DealBreaker {
	specs := []struct {
		pattern string
		label   string
	}{
		{`\bjava\b(?!\s*script)`, "Java (not JavaScript)"},
		{`\bspring\s+boot\b|\bspring\s+framework\b|\bmaven\b|\bgradle\b`, "Java/Spring ecosystem"},
		{`\basp\.net\b|\bblazor\b|\bxamarin\b`, ".NET framework"},
		{`\bsalesforce\b|\bapex\s+code\b|\bsoql\b`, "Salesforce"},
		{`\bsap\s+[a-z]+|\babap\b`, "SAP/ABAP"},
		{`\bruby\s+on\s+rails\b|\brails\s+developer\b`, "Ruby on Rails"},
		{`\bswift\b.{0,30}\bios\b|\bios\s+developer\b|\bxcode\b`, "iOS/Swift"},
		{`\bandroid\s+developer\b|\bkotlin\b.{0,30}\bandroid\b`, "Android/Kotlin"},
	}

	var result []DealBreaker
	for _, s := range specs {
		if re, err := regexp.Compile(`(?i)` + s.pattern); err == nil {
			result = append(result, DealBreaker{Pattern: re, Label: s.label})
		}
	}
	return result
}

// parseAcceptedLocations extracts the candidate's acceptable work locations.
func parseAcceptedLocations(profile string) []string {
	// Extract city from the Search section, then add known Belgian hubs.
	city := ""
	for _, line := range strings.Split(profile, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "city:") {
			city = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(line, "city:")))
			break
		}
	}

	accepted := []string{"remote", "belgi"} // "belgi" matches België/Belgium/Belgique
	if city != "" && city != "belgium" {
		accepted = append(accepted, city)
	}
	// Always include major Belgian tech hubs
	for _, hub := range []string{"brussel", "brussels", "bruxelles", "antwerp", "antwerpen", "gent", "ghent", "mechelen", "hasselt", "leuven"} {
		accepted = append(accepted, hub)
	}
	return accepted
}

// buildSummary returns the profile stripped of the ## Search section.
// This is what the LLM receives as candidate context.
func buildSummary(profile string) string {
	if idx := strings.Index(profile, "\n## Search"); idx != -1 {
		return strings.TrimSpace(profile[:idx])
	}
	return strings.TrimSpace(profile)
}
