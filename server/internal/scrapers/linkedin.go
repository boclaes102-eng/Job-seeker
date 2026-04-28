package scrapers

import (
	"context"
	"crypto/md5"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"job-seeker/server/internal/models"
)

// Belgian city LinkedIn geoIds
var belgianCityGeoIDs = map[string]string{
	"aarschot":  "90009706",
	"leuven":    "90009706", // Leuven/Flemish Brabant area
	"brussel":   "103023037",
	"brussels":  "103023037",
	"antwerp":   "90010001",
	"antwerpen": "90010001",
	"gent":      "90009999",
	"ghent":     "90009999",
	"hasselt":   "90009783",
	"mechelen":  "90009900",
	"brugge":    "90009764",
	"bruges":    "90009764",
	"liège":     "90010118",
	"namur":     "90010190",
}

const belgiumGeoID = "101165590"

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
	params.Set("location", "Belgium")
	params.Set("geoId", s.GeoID)
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
	if s.Radius > 0 {
		// LinkedIn distance is in miles; 1 km ≈ 0.621 miles
		miles := int(float64(s.Radius) * 0.621)
		params.Set("distance", fmt.Sprintf("%d", miles))
	}

	rawURL := "https://www.linkedin.com/jobs/search/?" + params.Encode()

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
		return nil, fmt.Errorf("linkedin fetch: %w", err)
	}
	defer resp.Body.Close()

	doc, err := goquery.NewDocumentFromReader(resp.Body)
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
	return jobs, nil
}
