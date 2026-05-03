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
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"job-seeker/server/internal/models"
)

// LinkedIn's public job search has two surfaces:
//
//   1. The full search-results page at /jobs/search/?... — heavyweight HTML,
//      occasionally returns a login wall for unauthenticated requests.
//
//   2. A "guest API" used by the embedded job widget on company pages, at
//      /jobs-guest/jobs/api/seeMoreJobPostings/search?... — returns clean,
//      lightweight HTML fragments with one <li> per job. Much more reliable.
//
// We try the guest API first and fall back to the search page on failure.
// Detail pages have a similar split: /jobs/view/{id}/ vs.
// /jobs-guest/jobs/api/jobPosting/{id}. We try guest first there too.

const (
	belgiumGeoID         = "101165590"
	guestSearchURL       = "https://www.linkedin.com/jobs-guest/jobs/api/seeMoreJobPostings/search"
	guestJobPostingURL   = "https://www.linkedin.com/jobs-guest/jobs/api/jobPosting/"
	maxJobsPerQuery      = 12
	httpRequestTimeout   = 15 * time.Second
	detailRequestTimeout = 10 * time.Second
)

// City-level geoIds give geographically scoped results. Country-level is the
// fallback when the requested city isn't recognised.
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

// belgianIndicators is a whitelist used to filter scraped jobs to Belgium-only.
// More robust than a blocklist: keep the job if location matches any indicator,
// or if location is empty/remote (which could still be Belgian).
var belgianIndicators = []string{
	"belgium", "belgique", "belgië",
	"vlaanderen", "flanders", "wallonie", "wallonia", "flemish", "walloon",
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
		return true
	}
	loc := strings.ToLower(location)
	if strings.Contains(loc, "remote") {
		return true
	}
	for _, ind := range belgianIndicators {
		if strings.Contains(loc, ind) {
			return true
		}
	}
	return false
}

// linkedInUserAgent is a current desktop Chrome UA string. Rotating this isn't
// effective — LinkedIn's bot detection is fingerprint-based, not UA-based.
const linkedInUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"

type LinkedInScraper struct {
	Query      string
	GeoID      string
	Radius     int
	MaxDaysOld int
}

func NewLinkedIn(query, _, city string, radiusKm, maxDaysOld int) *LinkedInScraper {
	geoID := belgiumGeoID
	if city != "" {
		if id, ok := belgianCityGeoIDs[strings.ToLower(city)]; ok {
			geoID = id
		}
	}
	return &LinkedInScraper{Query: query, GeoID: geoID, Radius: radiusKm, MaxDaysOld: maxDaysOld}
}

// Fetch returns up to maxJobsPerQuery LinkedIn job postings. It tries the
// guest API first, then falls back to the search-results page.
func (s *LinkedInScraper) Fetch() ([]*models.Job, error) {
	ctx, cancel := context.WithTimeout(context.Background(), httpRequestTimeout)
	defer cancel()

	if jobs, err := s.fetchGuest(ctx); err == nil && len(jobs) > 0 {
		return s.filterAndCap(jobs), nil
	} else if err != nil {
		slog.Debug("linkedin.guest_failed", "query", s.Query, "err", err)
	}
	return s.fetchSearchPage(ctx)
}

// fetchGuest queries the guest job-postings API with paging. Each "page" is
// a 25-item HTML fragment of <li> elements, much smaller than the full page.
func (s *LinkedInScraper) fetchGuest(ctx context.Context) ([]*models.Job, error) {
	params := url.Values{}
	params.Set("keywords", s.Query)
	params.Set("location", "Belgium")
	params.Set("geoId", s.GeoID)
	params.Set("sortBy", "DD") // descending date — newest first

	if tpr := timeFilter(s.MaxDaysOld); tpr != "" {
		params.Set("f_TPR", tpr)
	}
	if s.Radius > 0 {
		params.Set("distance", strconv.Itoa(s.Radius))
	}
	params.Set("start", "0")

	target := guestSearchURL + "?" + params.Encode()
	body, err := getWithRetry(ctx, target, 2)
	if err != nil {
		return nil, fmt.Errorf("guest search: %w", err)
	}
	return parseGuestSearchHTML(body)
}

// fetchSearchPage uses the full /jobs/search/ page. Heavier and more brittle
// than the guest API but sometimes returns results when the guest API doesn't.
func (s *LinkedInScraper) fetchSearchPage(ctx context.Context) ([]*models.Job, error) {
	params := url.Values{}
	params.Set("keywords", s.Query)
	params.Set("location", "Belgium")
	if tpr := timeFilter(s.MaxDaysOld); tpr != "" {
		params.Set("f_TPR", tpr)
	}
	params.Set("sortBy", "DD")

	target := "https://www.linkedin.com/jobs/search/?" + params.Encode()
	body, err := getWithRetry(ctx, target, 1)
	if err != nil {
		slog.Error("linkedin.search_page_failed", "query", s.Query, "err", err)
		return nil, fmt.Errorf("linkedin fetch: %w", err)
	}
	jobs, err := parseSearchPageHTML(body)
	if err != nil {
		return nil, err
	}
	return s.filterAndCap(jobs), nil
}

// timeFilter maps days-old to LinkedIn's f_TPR seconds-suffixed parameter.
func timeFilter(maxDaysOld int) string {
	switch maxDaysOld {
	case 1:
		return "r86400"
	case 3:
		return "r259200"
	case 7:
		return "r604800"
	case 30:
		return "r2592000"
	case 0:
		return ""
	default:
		return "r2592000"
	}
}

// getWithRetry does a GET with up to retries+1 attempts. Backs off on 429/503.
// Returns the response body on the first success, or the last error.
func getWithRetry(ctx context.Context, target string, retries int) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt <= retries; attempt++ {
		body, err := doGet(ctx, target)
		if err == nil {
			return body, nil
		}
		lastErr = err
		// Retry on transient/rate-limit errors only.
		if !shouldRetry(err) {
			break
		}
		// Exponential backoff capped at 4s. Respect ctx cancellation.
		wait := time.Duration(500*(1<<attempt)) * time.Millisecond
		if wait > 4*time.Second {
			wait = 4 * time.Second
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(wait):
		}
	}
	return nil, lastErr
}

// shouldRetry returns true for 429/503 — anything else is a permanent failure.
func shouldRetry(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "429") || strings.Contains(msg, "503") || strings.Contains(msg, "rate")
}

func doGet(ctx context.Context, target string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", linkedInUserAgent)
	req.Header.Set("Accept-Language", "nl-BE,nl;q=0.9,en-US;q=0.8,en;q=0.7")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, fmt.Errorf("status 429 (rate-limited)")
	}
	if resp.StatusCode == http.StatusServiceUnavailable {
		return nil, fmt.Errorf("status 503 (rate-limited)")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}

	// Cap reads at 1 MB (job listing HTML is at most ~200 KB even for full pages).
	return io.ReadAll(io.LimitReader(resp.Body, 1<<20))
}

// parseGuestSearchHTML extracts jobs from the lightweight guest API output.
func parseGuestSearchHTML(body []byte) ([]*models.Job, error) {
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("guest parse: %w", err)
	}
	var jobs []*models.Job
	doc.Find("li, div.job-search-card, div.base-card").EachWithBreak(func(_ int, sel *goquery.Selection) bool {
		if len(jobs) >= maxJobsPerQuery {
			return false
		}
		title := strings.TrimSpace(sel.Find(".base-search-card__title, h3").First().Text())
		company := strings.TrimSpace(sel.Find(".base-search-card__subtitle, h4").First().Text())
		location := cleanLinkedInText(sel.Find(".job-search-card__location, .base-search-card__metadata").First().Text())
		href, _ := sel.Find("a.base-card__full-link, a").First().Attr("href")
		if title == "" || href == "" {
			return true
		}
		if i := strings.Index(href, "?"); i != -1 {
			href = href[:i]
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
	return jobs, nil
}

// parseSearchPageHTML extracts jobs from the full /jobs/search/ HTML page.
func parseSearchPageHTML(body []byte) ([]*models.Job, error) {
	preview := body
	if len(preview) > 400 {
		preview = preview[:400]
	}
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("linkedin parse: %w", err)
	}
	var jobs []*models.Job
	doc.Find(".job-search-card, .base-card, li.jobs-search-results__list-item").EachWithBreak(func(_ int, sel *goquery.Selection) bool {
		if len(jobs) >= maxJobsPerQuery {
			return false
		}
		title := strings.TrimSpace(sel.Find(".base-search-card__title, h3").First().Text())
		company := strings.TrimSpace(sel.Find(".base-search-card__subtitle, h4").First().Text())
		location := cleanLinkedInText(sel.Find(".job-search-card__location, .base-search-card__metadata").First().Text())
		href, _ := sel.Find("a.base-card__full-link, a").First().Attr("href")
		if title == "" || href == "" {
			return true
		}
		if i := strings.Index(href, "?"); i != -1 {
			href = href[:i]
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
		slog.Warn("linkedin.no_jobs_scraped", "html_preview", string(preview))
	}
	return jobs, nil
}

// filterAndCap drops non-Belgian jobs and caps at maxJobsPerQuery.
func (s *LinkedInScraper) filterAndCap(jobs []*models.Job) []*models.Job {
	var out []*models.Job
	for _, j := range jobs {
		if isBelgian(j.Location) {
			out = append(out, j)
			if len(out) >= maxJobsPerQuery {
				break
			}
		}
	}
	return out
}

// FetchLinkedInDescription does a follow-up GET for one job's full description.
// LinkedIn's search-results page only carries titles + URLs, so without this
// step the matcher has nothing to score against. Tries the guest job-posting
// API first (more reliable), falls back to the public detail page.
func FetchLinkedInDescription(ctx context.Context, jobURL string) (string, error) {
	if jobURL == "" {
		return "", fmt.Errorf("empty url")
	}

	// Extract the numeric job ID from URLs like
	// https://www.linkedin.com/jobs/view/4097287456/
	if jobID := extractLinkedInJobID(jobURL); jobID != "" {
		guestURL := guestJobPostingURL + jobID
		if text, err := fetchAndExtractDescription(ctx, guestURL); err == nil && text != "" {
			return text, nil
		}
	}

	// Fallback: hit the public detail page directly.
	return fetchAndExtractDescription(ctx, jobURL)
}

func extractLinkedInJobID(jobURL string) string {
	const marker = "/jobs/view/"
	i := strings.Index(jobURL, marker)
	if i == -1 {
		return ""
	}
	rest := jobURL[i+len(marker):]
	end := len(rest)
	for j, r := range rest {
		if r < '0' || r > '9' {
			end = j
			break
		}
	}
	return rest[:end]
}

func fetchAndExtractDescription(ctx context.Context, target string) (string, error) {
	dctx, cancel := context.WithTimeout(ctx, detailRequestTimeout)
	defer cancel()

	body, err := getWithRetry(dctx, target, 1)
	if err != nil {
		return "", fmt.Errorf("linkedin detail: %w", err)
	}

	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("linkedin detail parse: %w", err)
	}

	// LinkedIn detail markup has a few variants; try them in priority order.
	selectors := []string{
		".show-more-less-html__markup",
		".description__text",
		"section.show-more-less-html",
		"div.description",
	}
	for _, sel := range selectors {
		if text := strings.TrimSpace(doc.Find(sel).First().Text()); text != "" {
			return cleanLinkedInText(text), nil
		}
	}
	if text := strings.TrimSpace(doc.Find("article").First().Text()); text != "" {
		t := cleanLinkedInText(text)
		if len(t) > 8000 {
			t = t[:8000]
		}
		return t, nil
	}
	return "", nil
}

// cleanLinkedInText collapses LinkedIn's heavily-padded text fragments into
// a single readable line. The search results HTML wraps every metadata block
// in 30+ lines of indented blanks plus a trailing relative date marker
// ("11 uur geleden"); without cleanup this noise ends up in our DB.
func cleanLinkedInText(s string) string {
	s = strings.ReplaceAll(s, "\u00a0", " ")
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return ""
	}
	joined := strings.Join(fields, " ")

	for _, marker := range []string{
		" Wees een van de eerste sollicitanten",
		" Actief aan het werven",
		" Be among the first applicants",
		" Promoted",
	} {
		if i := strings.Index(joined, marker); i != -1 {
			joined = joined[:i]
		}
	}
	joined = trimTimeAgoSuffix(joined)
	return strings.TrimSpace(joined)
}

// trimTimeAgoSuffix removes a trailing relative-date phrase like "11 uur geleden",
// "2 hours ago", or "il y a 3 jours" — NL/EN/FR.
func trimTimeAgoSuffix(s string) string {
	timeUnits := []string{
		"seconde", "seconden", "minuut", "minuten", "uur", "uren", "dag", "dagen",
		"week", "weken", "maand", "maanden",
		"second", "seconds", "minute", "minutes", "hour", "hours", "day", "days",
		"month", "months",
		"heure", "heures", "jour", "jours", "semaine", "semaines", "mois",
	}
	suffixes := []string{"geleden", "ago", "il y a"}
	low := strings.ToLower(s)
	for _, suf := range suffixes {
		if !strings.HasSuffix(low, suf) {
			continue
		}
		head := strings.TrimSpace(low[:len(low)-len(suf)])
		toks := strings.Fields(head)
		if len(toks) >= 2 {
			for _, u := range timeUnits {
				if toks[len(toks)-1] == u {
					cut := strings.Join(toks[:len(toks)-2], " ")
					if idx := strings.LastIndex(strings.ToLower(s), cut); idx != -1 && cut != "" {
						return strings.TrimSpace(s[:idx+len(cut)])
					}
					return ""
				}
			}
		}
	}
	return s
}
