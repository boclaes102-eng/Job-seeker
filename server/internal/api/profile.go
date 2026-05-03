package api

import (
	"bufio"
	"strings"
)

type SearchConfig struct {
	Location string // country/region (e.g. "België"), passed to LinkedIn's location field
	City     string // city for radius search via Adzuna (e.g. "Leuven")
	Queries  []string
}

// ParseSearchConfig reads the ## Search section from profile.md.
//
//	## Search
//	location: Belgium
//	city: Aarschot
//	queries:
//	- full-stack developer
//	- cybersecurity analyst
func ParseSearchConfig(profile string) SearchConfig {
	cfg := SearchConfig{Location: "België", City: ""}

	scanner := bufio.NewScanner(strings.NewReader(profile))
	inSearch := false
	inQueries := false

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if strings.HasPrefix(line, "## ") {
			inSearch = strings.EqualFold(strings.TrimPrefix(line, "## "), "search")
			inQueries = false
			continue
		}
		if !inSearch {
			continue
		}

		lower := strings.ToLower(line)
		if strings.HasPrefix(lower, "location:") {
			cfg.Location = strings.TrimSpace(strings.SplitN(line, ":", 2)[1])
			continue
		}
		if strings.HasPrefix(lower, "city:") {
			cfg.City = strings.TrimSpace(strings.SplitN(line, ":", 2)[1])
			continue
		}
		if lower == "queries:" {
			inQueries = true
			continue
		}
		if inQueries && strings.HasPrefix(line, "- ") {
			q := strings.TrimSpace(strings.TrimPrefix(line, "- "))
			if q != "" {
				cfg.Queries = append(cfg.Queries, q)
			}
		}
	}

	if len(cfg.Queries) == 0 {
		cfg.Queries = []string{"developer", "software engineer"}
	}
	return cfg
}
