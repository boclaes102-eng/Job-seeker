package matcher

import (
	"strings"

	"job-seeker/server/internal/models"
)

// ExtractKeywords parses the ## Tech stack section of profile.md and returns
// a deduplicated lowercase list of technology names used for fast pre-filtering.
func ExtractKeywords(profile string) []string {
	seen := map[string]bool{}
	var keywords []string

	add := func(raw string) {
		// Strip parentheticals: "JavaScript (ES2022)" → "JavaScript"
		if idx := strings.Index(raw, "("); idx != -1 {
			raw = strings.TrimSpace(raw[:idx])
		}
		kw := strings.ToLower(strings.TrimSpace(raw))
		if len(kw) < 2 || seen[kw] {
			return
		}
		seen[kw] = true
		keywords = append(keywords, kw)

		// Also add version-stripped variant: "next.js 15" → "next.js"
		parts := strings.Fields(kw)
		if len(parts) > 1 {
			last := parts[len(parts)-1]
			isVersion := true
			for _, c := range last {
				if c != '.' && (c < '0' || c > '9') {
					isVersion = false
					break
				}
			}
			if isVersion {
				base := strings.Join(parts[:len(parts)-1], " ")
				if !seen[base] {
					seen[base] = true
					keywords = append(keywords, base)
				}
			}
		}
	}

	inStack := false
	for _, line := range strings.Split(profile, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.EqualFold(trimmed, "## Tech stack") || strings.HasPrefix(trimmed, "## Tech stack") {
			inStack = true
			continue
		}
		if inStack && strings.HasPrefix(trimmed, "## ") {
			break
		}
		if !inStack || trimmed == "" {
			continue
		}
		// Strip leading "- " and bold markers
		trimmed = strings.TrimPrefix(trimmed, "- ")
		trimmed = strings.ReplaceAll(trimmed, "**", "")
		// Strip category label: "Backend: ..." → "..."
		if idx := strings.Index(trimmed, ": "); idx != -1 {
			trimmed = trimmed[idx+2:]
		}
		// Split on · and ,
		for _, part := range strings.FieldsFunc(trimmed, func(r rune) bool {
			return r == '·' || r == ','
		}) {
			add(part)
		}
	}
	return keywords
}

// HasTechOverlap returns true if any of the candidate's known technologies
// appear in the job title or the first 1000 characters of the description.
// Jobs with zero overlap are almost certainly in the wrong domain and can skip Ollama.
func HasTechOverlap(job *models.Job, keywords []string) bool {
	limit := 1000
	if len(job.Description) < limit {
		limit = len(job.Description)
	}
	haystack := strings.ToLower(job.Title + " " + job.Description[:limit])
	for _, kw := range keywords {
		if strings.Contains(haystack, kw) {
			return true
		}
	}
	return false
}
