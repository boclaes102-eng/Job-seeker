package matcher

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
	"sync"

	"job-seeker/server/internal/models"
)

// ScoreBreakdown is the output of the deterministic scorer.
//
// Total is in [0, 100]. NeedsLLM signals whether the score is in the
// uncertainty band where an LLM referee would add useful judgement.
type ScoreBreakdown struct {
	Total          int
	NeedsLLM       bool
	MatchedTech    []string // canonical names found in the job description
	MissingTech    []string // techs the job seems to require but the candidate lacks
	RoleMatch      string   // matched role keyword, e.g. "full-stack developer"
	LocationOK     bool
	LocationNote   string
	DealBreaker    string // first deal-breaker label that fired; "" if none
	TechPoints     int
	RequiredPoints int
	RolePoints     int
	LocationPoints int
	Reason         string // human-readable single-paragraph explanation
}

// Uncertainty band: scores in [llmLowBand, llmHighBand] benefit from LLM review.
// Below low: clear miss. Above high: clear hit.
const (
	llmLowBand  = 35
	llmHighBand = 78
)

// DeterministicScore computes a fast 0-100 score for one job. Microseconds
// per call — safe to run on every scraped job.
//
// Components (location is intentionally NOT scored — the user wants to judge
// distance themselves; we still surface the location string for display):
//   - Tech coverage     (max 60) — weighted by category importance
//   - Required-tech fit (max 20) — fraction of job-required techs the candidate has
//   - Role title match  (max 20) — strongest in the job title
//   - Deal-breakers     (cap at 25 if matched)
func DeterministicScore(job *models.Job, idx *CandidateIndex) ScoreBreakdown {
	out := ScoreBreakdown{LocationOK: true}

	titleLC := strings.ToLower(job.Title)
	bodyLC := strings.ToLower(job.Description)
	combined := titleLC + "\n" + bodyLC

	// ── 1. Tech coverage ────────────────────────────────────────────────
	matchedTokens := []TechToken{}
	for _, t := range idx.Tech {
		for _, re := range t.Aliases {
			if re.MatchString(combined) {
				matchedTokens = append(matchedTokens, t)
				out.MatchedTech = append(out.MatchedTech, t.Canonical)
				break
			}
		}
	}

	// Sort for stable display: highest-weight first, then alphabetical.
	sort.SliceStable(matchedTokens, func(i, j int) bool {
		if matchedTokens[i].Weight != matchedTokens[j].Weight {
			return matchedTokens[i].Weight > matchedTokens[j].Weight
		}
		return matchedTokens[i].Canonical < matchedTokens[j].Canonical
	})
	out.MatchedTech = make([]string, len(matchedTokens))
	for i, t := range matchedTokens {
		out.MatchedTech[i] = t.Canonical
	}

	techScore := 0
	for _, t := range matchedTokens {
		switch t.Weight {
		case 3:
			techScore += 7 // primary stack — strongest signal
		case 2:
			techScore += 4 // secondary
		case 1:
			techScore += 1 // tooling — tiny boost
		}
	}
	if techScore > 60 {
		techScore = 60
	}
	out.TechPoints = techScore

	// ── 2. Required-tech fit ────────────────────────────────────────────
	// Find the "requirements" portion of the description, then check, against
	// a broad vocabulary, what techs the JOB asks for and how many the
	// candidate has. This catches missing-but-required techs (e.g. job wants
	// Vue.js, candidate doesn't have it) which the candidate-only vocab misses.
	requirements := findRequirementSection(job.Description)
	requirementsLC := strings.ToLower(requirements)
	if requirementsLC != "" {
		jobTechs := detectTechsInText(requirementsLC, ensureJobVocabulary())
		candidateHasSet := map[string]bool{}
		for _, t := range matchedTokens {
			candidateHasSet[t.Canonical] = true
		}
		var have, lack []string
		for _, t := range jobTechs {
			if candidateHasSet[t] {
				have = append(have, t)
			} else {
				lack = append(lack, t)
			}
		}
		out.MissingTech = lack
		if total := len(jobTechs); total > 0 {
			fraction := float64(len(have)) / float64(total)
			// Curve: pow(fraction, 0.7) — rewards partial matches more generously
			// while still penalising "candidate has 1/5 of required techs".
			weighted := pow(fraction, 0.7)
			out.RequiredPoints = int(20 * weighted)
		}
	}

	// ── 3. Role title match ─────────────────────────────────────────────
	rolePoints := 0
	roleMatch := ""
	for needle, canon := range roleKeywordCanon {
		if strings.Contains(titleLC, needle) {
			rolePoints = 20
			roleMatch = canon
			break
		}
	}
	if rolePoints == 0 {
		for needle, canon := range roleKeywordCanon {
			if strings.Contains(bodyLC, needle) {
				rolePoints = 8
				roleMatch = canon
				break
			}
		}
	}
	out.RolePoints = rolePoints
	out.RoleMatch = roleMatch

	// ── 4. Location (display only, NOT scored) ──────────────────────────
	out.LocationNote = strings.TrimSpace(job.Location)
	out.LocationOK = true // not scoring on it; treat all as acceptable
	out.LocationPoints = 0

	// ── 5. Combine + deal-breakers ──────────────────────────────────────
	total := out.TechPoints + out.RequiredPoints + out.RolePoints
	if total > 100 {
		total = 100
	}
	for _, db := range idx.DealBreakers {
		if db.Pattern.MatchString(job.Description) {
			out.DealBreaker = db.Label
			if total > 25 {
				total = 25
			}
			break
		}
	}

	out.Total = total
	out.NeedsLLM = total >= llmLowBand && total <= llmHighBand && out.DealBreaker == ""
	out.Reason = buildDeterministicReason(out)
	return out
}

// broadTechVocabulary is used for detecting what techs a job description
// REQUIRES — independent of what the candidate has. This lets us catch
// missing-but-required techs (e.g. job requires Vue.js, candidate doesn't have it).
//
// Each entry: canonical name → match patterns (whole-word, case-insensitive).
// Keep this conservative — only common, named technologies. Don't include
// generic terms like "API" or "database".
var broadTechSpecs = []struct {
	canonical string
	aliases   []string
}{
	// Languages
	{"Java", []string{"java"}}, // matches "java" but NOT "javascript" via word boundary
	{"Kotlin", []string{"kotlin"}},
	{"Scala", []string{"scala"}},
	{"Ruby", []string{"ruby"}},
	{"Rust", []string{"rust"}},
	{"Swift", []string{"swift"}},
	{"Elixir", []string{"elixir"}},
	{"Clojure", []string{"clojure"}},
	{"Perl", []string{"perl"}},
	// Frameworks
	{"Spring Boot", []string{"spring boot", "spring framework"}},
	{"Django", []string{"django"}},
	{"Flask", []string{"flask"}},
	{"Rails", []string{"ruby on rails", "rails developer"}},
	{"Laravel", []string{"laravel"}},
	{"Symfony", []string{"symfony"}},
	{"ASP.NET", []string{"asp.net", "asp .net", "aspnet"}},
	{"Blazor", []string{"blazor"}},
	{"Xamarin", []string{"xamarin"}},
	{"Svelte", []string{"svelte", "sveltekit"}},
	// Frontend frameworks (broad — these may overlap with Vue/Angular in the
	// curated list. The dedup logic in ensureJobVocabulary skips dupes).
	{"Vuex", []string{"vuex"}},
	{"Nuxt", []string{"nuxt", "nuxt.js"}},
	// Mobile
	{"Android", []string{"android development", "android sdk", "android developer"}},
	{"iOS", []string{"ios development", "ios sdk", "ios developer"}},
	{"Flutter", []string{"flutter"}},
	{"React Native", []string{"react native"}},
	// Databases
	{"MongoDB", []string{"mongodb", "mongo db"}},
	{"MySQL", []string{"mysql"}},
	{"Oracle", []string{"oracle database", "oracle db"}},
	{"MS SQL Server", []string{"sql server", "ms sql", "microsoft sql"}},
	{"DynamoDB", []string{"dynamodb"}},
	{"Cassandra", []string{"cassandra"}},
	{"Neo4j", []string{"neo4j"}},
	{"Elasticsearch", []string{"elasticsearch", "elastic search"}},
	{"Snowflake", []string{"snowflake"}},
	{"BigQuery", []string{"bigquery"}},
	// Enterprise / specialised
	{"Salesforce", []string{"salesforce", "apex code", "soql"}},
	{"SAP", []string{"sap abap", "abap", "sap erp"}},
	{"ServiceNow", []string{"servicenow"}},
	{"Magento", []string{"magento"}},
	{"Shopify", []string{"shopify"}},
	{"WordPress", []string{"wordpress"}},
	{"Drupal", []string{"drupal"}},
	// Cloud
	{"Azure", []string{"azure", "microsoft azure"}},
	{"GCP", []string{"gcp", "google cloud platform", "google cloud"}},
	{"Terraform", []string{"terraform"}},
	{"Ansible", []string{"ansible"}},
	// Big data
	{"Spark", []string{"apache spark", "pyspark"}},
	{"Hadoop", []string{"hadoop"}},
	{"Airflow", []string{"airflow"}},
	// ML
	{"TensorFlow", []string{"tensorflow"}},
	{"PyTorch", []string{"pytorch"}},
	// Testing / build
	{"JUnit", []string{"junit"}},
	{"Cypress", []string{"cypress"}},
	{"Selenium", []string{"selenium"}},
	{"Maven", []string{"maven"}},
	{"Gradle", []string{"gradle"}},
}

// jobRequirementVocabulary is the merged vocabulary used to detect what techs
// a job mentions: every tech the candidate has (from the curated table) PLUS
// the broad outside-the-candidate vocabulary above.
//
// Built lazily on first use and cached.
var jobVocabulary []TechToken
var jobVocabularyMu = newOnce()

func ensureJobVocabulary() []TechToken {
	jobVocabularyMu.Do(func() {
		seen := map[string]bool{}
		all := make([]TechToken, 0, len(techDefinitions)+len(broadTechSpecs))
		for _, def := range techDefinitions {
			all = append(all, TechToken{
				Canonical: def.canonical,
				Aliases:   compileAliases(def.aliases),
				Category:  def.category,
				Weight:    def.weight,
			})
			seen[def.canonical] = true
		}
		for _, spec := range broadTechSpecs {
			if seen[spec.canonical] || len(spec.aliases) == 0 {
				continue
			}
			all = append(all, TechToken{
				Canonical: spec.canonical,
				Aliases:   compileAliases(spec.aliases),
				Category:  "outside",
				Weight:    1,
			})
		}
		jobVocabulary = all
	})
	return jobVocabulary
}

// findRequirementSection extracts the requirements portion of a job description.
//
// Strategy:
//  1. If a structured "Profile & Skills:" / "Vereisten:" / "Requirements:" header
//     is present, return the 800 chars after it.
//  2. Otherwise, return the whole description (capped at 1500 chars). This
//     catches inline phrasing like "Strong X experience required" where the
//     tech is mentioned BEFORE the "required" keyword.
//
// We deliberately don't use the inline "required" form as a header, because
// the relevant tech is typically to its left, not its right.
func findRequirementSection(desc string) string {
	if desc == "" {
		return ""
	}
	for _, re := range requirementHeaders {
		if loc := re.FindStringIndex(desc); loc != nil {
			start := loc[1]
			// Skip past the header — but make sure there's a colon or newline
			// nearby (signal that this is a section header, not inline text).
			lookahead := start + 30
			if lookahead > len(desc) {
				lookahead = len(desc)
			}
			windowAfter := desc[start:lookahead]
			if !strings.ContainsAny(windowAfter, ":\n*-•") {
				// Inline mention like "X required." — fall through to whole-desc scan.
				continue
			}
			end := start + 800
			if end > len(desc) {
				end = len(desc)
			}
			return desc[start:end]
		}
	}
	if len(desc) > 1500 {
		return desc[:1500]
	}
	return desc
}

// requirementHeaders are anchored on words/phrases that signal a structured
// requirements list. Inline phrasing like "X required" is intentionally NOT
// in this list — see findRequirementSection's comment.
var requirementHeaders = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\b(?:profile\s*&?\s*skills?|requirements?|qualifications?|what\s+you\s+bring|must\s+haves?|nice\s+to\s+haves?|key\s+skills?|technical\s+requirements?)\b`),
	regexp.MustCompile(`(?i)\b(?:profiel|vereisten?|kwalificaties|jouw\s+profiel|wat\s+je\s+meebrengt|sterke\s+kennis\s+van)\b`),
}

// buildDeterministicReason produces a single sentence explaining the score.
// detectTechsInText returns canonical names for every tech in `vocab` that
// the (already-lowercased) text mentions.
func detectTechsInText(textLC string, vocab []TechToken) []string {
	seen := map[string]bool{}
	var out []string
	for _, t := range vocab {
		for _, re := range t.Aliases {
			if re.MatchString(textLC) {
				if !seen[t.Canonical] {
					seen[t.Canonical] = true
					out = append(out, t.Canonical)
				}
				break
			}
		}
	}
	return out
}

// pow wraps math.Pow with a guard against base <= 0.
func pow(base, exp float64) float64 {
	if base <= 0 {
		return 0
	}
	return math.Pow(base, exp)
}

// once is a tiny sync.Once equivalent for lazy vocab init.
// We need this because the jobVocabulary slice is built once and read concurrently.
type once struct {
	mu   sync.Mutex
	done bool
}

func newOnce() *once { return &once{} }

func (o *once) Do(fn func()) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if !o.done {
		fn()
		o.done = true
	}
}

func buildDeterministicReason(b ScoreBreakdown) string {
	var parts []string

	if len(b.MatchedTech) > 0 {
		shown := b.MatchedTech
		if len(shown) > 5 {
			shown = shown[:5]
		}
		extra := ""
		if len(b.MatchedTech) > 5 {
			extra = fmt.Sprintf(" (+%d more)", len(b.MatchedTech)-5)
		}
		parts = append(parts, fmt.Sprintf("matched %d candidate technologies — %s%s",
			len(b.MatchedTech), strings.Join(shown, ", "), extra))
	} else {
		parts = append(parts, "no candidate technologies detected")
	}

	if b.RoleMatch != "" {
		if b.RolePoints == 20 {
			parts = append(parts, fmt.Sprintf("title is a %s role", b.RoleMatch))
		} else {
			parts = append(parts, fmt.Sprintf("description mentions %s", b.RoleMatch))
		}
	}

	if len(b.MissingTech) > 0 && len(b.MissingTech) <= 3 {
		parts = append(parts, fmt.Sprintf("missing: %s", strings.Join(b.MissingTech, ", ")))
	} else if len(b.MissingTech) > 3 {
		parts = append(parts, fmt.Sprintf("missing %d required techs", len(b.MissingTech)))
	}

	if b.DealBreaker != "" {
		parts = append(parts, "DEAL-BREAKER: "+b.DealBreaker)
	}

	r := strings.Join(parts, "; ")
	if r == "" {
		return "no signal."
	}
	return strings.ToUpper(r[:1]) + r[1:] + "."
}
