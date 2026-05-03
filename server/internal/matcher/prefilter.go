package matcher

import (
	"fmt"
	"strings"

	"job-seeker/server/internal/models"
)

// ScoreBreakdown is the output of the deterministic scorer.
// Total is in [0, 100]. NeedsLLM signals whether the score lands in the
// uncertainty band where an LLM referee would add value.
type ScoreBreakdown struct {
	Total    int
	NeedsLLM bool

	// Reason fields, populated for downstream display and for the LLM prompt.
	MatchedTech  []string // canonical names found in the job description
	MissingTech  []string // role-relevant techs the candidate lacks (best-effort)
	RoleMatch    string   // matched role keyword, e.g. "full-stack developer"; "" if none
	LocationOK   bool
	LocationNote string // "Brussels" / "remote" / "outside accepted area"
	DealBreaker  string // first deal-breaker label that fired; "" if none
	CategoryHits map[string]int

	// Reason is a single-paragraph human-readable explanation, suitable for
	// display when no LLM is consulted.
	Reason string
}

// uncertainty band: scores in [llmLowBand, llmHighBand] benefit from LLM review.
// Below low: clear miss. Above high: clear hit.
const (
	llmLowBand  = 35
	llmHighBand = 78
)

// DeterministicScore computes a fast, deterministic 0-100 score for one job.
// The scoring rubric is intentionally simple and explicable — the LLM is only
// consulted afterwards for jobs in the uncertainty band.
//
// Components (additive, capped at 100):
//   - Tech overlap:    up to 60 points (count + category coverage)
//   - Role title:      up to 20 points (exact title match in job title, partial in body)
//   - Location:        up to 10 points (in accepted set OR remote)
//   - Profile section: up to 10 points (signal that role type matches "Looking for")
//
// Deal-breakers cap the score at 25.
func DeterministicScore(job *models.Job, idx *CandidateIndex) ScoreBreakdown {
	out := ScoreBreakdown{
		CategoryHits: map[string]int{},
		LocationOK:   true, // assume OK unless we positively detect otherwise
	}

	titleLC := strings.ToLower(job.Title)
	bodyLC := strings.ToLower(job.Description)
	combined := titleLC + "\n" + bodyLC

	// --- 1. Tech overlap (max 60) ---
	matchedSet := map[string]bool{}
	for _, t := range idx.TechTokens {
		for _, re := range t.Aliases {
			if re.MatchString(combined) {
				if !matchedSet[t.Canonical] {
					matchedSet[t.Canonical] = true
					out.MatchedTech = append(out.MatchedTech, t.Canonical)
					out.CategoryHits[t.Category]++
				}
				break
			}
		}
	}

	// Up to 40 points for raw tech-match count: 5 each, max 8 matches.
	techCountPoints := len(out.MatchedTech) * 5
	if techCountPoints > 40 {
		techCountPoints = 40
	}

	// Up to 20 points for category breadth — having tech in multiple
	// relevant categories signals a real fit, not just one accidental keyword.
	relevantCategories := []string{"language", "frontend", "backend", "data", "security", "iot", "infra", "ai"}
	categoryCount := 0
	for _, c := range relevantCategories {
		if out.CategoryHits[c] > 0 {
			categoryCount++
		}
	}
	categoryPoints := categoryCount * 5
	if categoryPoints > 20 {
		categoryPoints = 20
	}

	techScore := techCountPoints + categoryPoints

	// --- 2. Role title match (max 20) ---
	// Title match is much stronger evidence than body match.
	roleTitlePoints := 0
	for needle, canon := range roleKeywordCanon {
		if strings.Contains(titleLC, needle) {
			out.RoleMatch = canon
			roleTitlePoints = 20
			break
		}
	}
	if roleTitlePoints == 0 {
		// Partial credit for body mention only.
		for needle, canon := range roleKeywordCanon {
			if strings.Contains(bodyLC, needle) {
				out.RoleMatch = canon
				roleTitlePoints = 8
				break
			}
		}
	}

	// --- 3. Location match (max 10) ---
	locationPoints, locNote := scoreLocation(job, idx)
	out.LocationNote = locNote
	out.LocationOK = locationPoints > 0 || locNote == "" || locNote == "unknown"

	// --- 4. "Looking for" alignment (max 10) ---
	// If the job title references one of the candidate's target roles, that's
	// already counted above. Here we award a small bonus when the body of the
	// description mentions multiple of the candidate's target roles, which
	// signals the role's scope aligns with what the candidate wants.
	alignmentPoints := 0
	roleHitsInBody := 0
	for needle := range roleKeywordCanon {
		if strings.Contains(bodyLC, needle) {
			roleHitsInBody++
			if roleHitsInBody >= 2 {
				alignmentPoints = 10
				break
			}
		}
	}

	total := techScore + roleTitlePoints + locationPoints + alignmentPoints
	if total > 100 {
		total = 100
	}

	// --- 5. Deal-breakers (cap at 25) ---
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
	out.NeedsLLM = total >= llmLowBand && total <= llmHighBand
	out.Reason = buildDeterministicReason(out)
	return out
}

// scoreLocation returns (points, note). Points are 0 or 10; the note describes
// the decision for display.
func scoreLocation(job *models.Job, idx *CandidateIndex) (int, string) {
	// Empty location data: treat as unknown. LinkedIn occasionally omits it.
	loc := strings.ToLower(strings.TrimSpace(job.Location))
	if loc == "" {
		return 5, "unknown" // small benefit of doubt
	}
	if strings.Contains(loc, "remote") || strings.Contains(strings.ToLower(job.Description), "fully remote") {
		return 10, "remote"
	}
	for _, accepted := range idx.AcceptedLocations {
		if strings.Contains(loc, accepted) {
			return 10, accepted
		}
	}
	// Not in accepted list — but if it mentions Belgium, give partial credit;
	// scrapers already filter out non-Belgian jobs, so this is a safety net.
	if strings.Contains(loc, "belgi") {
		return 5, loc
	}
	return 0, "outside accepted area: " + loc
}

func buildDeterministicReason(b ScoreBreakdown) string {
	parts := []string{}

	if len(b.MatchedTech) > 0 {
		shown := b.MatchedTech
		if len(shown) > 6 {
			shown = shown[:6]
		}
		extra := ""
		if len(b.MatchedTech) > 6 {
			extra = fmt.Sprintf(" (+%d more)", len(b.MatchedTech)-6)
		}
		parts = append(parts, fmt.Sprintf("matched %d candidate technologies — %s%s",
			len(b.MatchedTech), strings.Join(shown, ", "), extra))
	} else {
		parts = append(parts, "no candidate technologies detected in the job description")
	}

	if b.RoleMatch != "" {
		parts = append(parts, fmt.Sprintf("title aligns with '%s'", b.RoleMatch))
	}

	if b.LocationNote != "" && b.LocationNote != "unknown" {
		if b.LocationOK {
			parts = append(parts, "location: "+b.LocationNote)
		} else {
			parts = append(parts, b.LocationNote)
		}
	}

	if b.DealBreaker != "" {
		parts = append(parts, "DEAL-BREAKER: "+b.DealBreaker)
	}

	r := strings.Join(parts, "; ")
	if r == "" {
		return "no signal"
	}
	return strings.ToUpper(r[:1]) + r[1:] + "."
}
