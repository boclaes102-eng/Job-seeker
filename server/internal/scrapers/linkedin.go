package scrapers

import (
	"bytes"
	"context"
	"crypto/md5"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"job-seeker/server/internal/models"
)

const belgiumGeoID = "101165590"

// City-level geoIds produce geographically scoped results on LinkedIn's public search.
// The country-level geoId (101165590) returns globally popular results for
// unauthenticated requests, so we prefer the city/region-level ID when available.
var belgianCityGeoIDs = map[string]string{
	"leuven":    "90009706",
	"aarschot":  "90009706",
	"mechelen":  "90009900",
	"brussel":   "103023037",
	"brussels":  "103023037",
	"antwerp":   "90010001",
	"antwerpen": "90010001",
	"gent":      "90009999",
	"ghent":     "90009999",
	"hasselt":   "90009783",
	"brugge":    "90009764",
	"bruges":    "90009764",
	"liège":     "90010118",
	"namur":     "90010190",
}

// Belgian location indicators — country names in EN/FR/NL, regions, and major cities.
// A whitelist is more robust than a blocklist: keep the job only if the location
// matches one of these, or if the location is empty/remote (could still be Belgian).
var belgianIndicators = []string{
	// Country
	"belgium", "belgique", "belgië",
	// Regions
	"vlaanderen", "flanders", "wallonie", "wallonia", "flemish", "walloon",
	// Cities
	"brussel", "brussels", "bruxelles",
	"antwerp", "antwerpen",
	"gent", "ghent",
	"brugge", "bruges",
	"leuven", "louvain",
	"mechelen",
	"hasselt",
	"liège", "liege",
	"namur",
	"charleroi",
	"mons",
	"kortrijk",
	"aalst",
	"genk",
	"roeselare",
	"turnhout",
	"sint-niklaas",
	"dendermonde",
	"ieper",
	"tongeren",
	"arlon",
	"verviers",
	"oostende", "ostend",
}

func isBelgian(location string) bool {
	if location == "" {
		return true // no location data — don't discard
	}
	loc := strings.ToLower(location)
	if strings.Contains(loc, "remote") {
		return true // remote roles can target Belgian workers
	}
	for _, indicator := range belgianIndicators {
		if strings.Contains(loc, indicator) {
			return true
		}
	}
	return false
}

type LinkedInScraper struct {
	Query      string
	GeoID      string
	Radius     int
	MaxDaysOld int
}

func NewLinkedIn(query, _ string, city string, radiusKm, maxDaysOld int) *LinkedInScraper {
	geoID := belgiumGeoID
	if city != "" {
		if id, ok := belgianCityGeoIDs[strings.ToLower(city)]; ok {
			geoID = id
		}
	}
	return &LinkedInScraper{Query: query, GeoID: geoID, Radius: radiusKm, MaxDaysOld: maxDaysOld}
}

func (s *LinkedInScraper) Fetch() ([]*models.Job, error) {
	params := url.Values{}
	params.Set("keywords", s.Query)
	// No geoId — LinkedIn's own public search form doesn't include one.
	// It resolves "Belgium" to the correct geoId internally. Passing a geoId
	// explicitly (90009706=Netherlands area, 101165590=UK-biased global) broke things.
	params.Set("location", "Belgium")
	// f_TPR = time range in seconds; fall back to 30 days if no filter set
	tpr := "r2592000"
	switch s.MaxDaysOld {
	case 1:
		tpr = "r86400"
	case 3:
		tpr = "r259200"
	case 7:
		tpr = "r604800"
	case 30:
		tpr = "r2592000"
	}
	params.Set("f_TPR", tpr)
	params.Set("sortBy", "DD")

	rawURL := "https://www.linkedin.com/jobs/search/?" + params.Encode()
	slog.Debug("linkedin.request", "query", s.Query, "url", rawURL)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36")
	req.Header.Set("Accept-Language", "nl-BE,nl;q=0.9,en-US;q=0.8,en;q=0.7")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		slog.Error("linkedin.http_error", "query", s.Query, "err", err)
		return nil, fmt.Errorf("linkedin fetch: %w", err)
	}
	defer resp.Body.Close()
	slog.Debug("linkedin.response", "query", s.Query, "status", resp.StatusCode)

	// Capture first 400 bytes so we can log an HTML preview when no jobs are found
	// (helps detect login walls, CAPTCHAs, or empty pages).
	preview, _ := io.ReadAll(io.LimitReader(resp.Body, 400))
	fullBody := io.MultiReader(bytes.NewReader(preview), resp.Body)

	doc, err := goquery.NewDocumentFromReader(fullBody)
	if err != nil {
		return nil, fmt.Errorf("linkedin parse: %w", err)
	}

	var jobs []*models.Job
	doc.Find(".job-search-card, .base-card, li.jobs-search-results__list-item").EachWithBreak(func(_ int, sel *goquery.Selection) bool {
		if len(jobs) >= 10 {
			return false // stop after 10 per query
		}
		title := strings.TrimSpace(sel.Find(".base-search-card__title, h3").First().Text())
		company := strings.TrimSpace(sel.Find(".base-search-card__subtitle, h4").First().Text())
		location := strings.TrimSpace(sel.Find(".job-search-card__location, .base-search-card__metadata").First().Text())
		href, _ := sel.Find("a.base-card__full-link, a").First().Attr("href")

		if title == "" || href == "" {
			return true
		}
		if idx := strings.Index(href, "?"); idx != -1 {
			href = href[:idx]
		}

		id := fmt.Sprintf("%x", md5.Sum([]byte(href)))
		jobs = append(jobs, &models.Job{
			ID:        id,
			Title:     title,
			Company:   company,
			Location:  location,
			URL:       href,
			Source:    "linkedin",
			PostedAt:  time.Now(),
			FetchedAt: time.Now(),
			Status:    models.StatusNew,
		})
		return true
	})

	if len(jobs) == 0 {
		slog.Warn("linkedin.no_jobs_scraped", "query", s.Query, "html_preview", string(preview))
	} else {
		slog.Debug("linkedin.raw_jobs", "query", s.Query, "count", len(jobs))
	}

	// Keep only Belgian jobs; log the filter decision for every job
	var filtered []*models.Job
	for _, j := range jobs {
		passed := isBelgian(j.Location)
		slog.Debug("linkedin.job", "query", s.Query, "title", j.Title, "company", j.Company, "location", j.Location, "belgian", passed)
		if passed {
			filtered = append(filtered, j)
		}
	}
	slog.Debug("linkedin.filter_result", "query", s.Query, "raw", len(jobs), "passed", len(filtered))

	return filtered, nil
}
