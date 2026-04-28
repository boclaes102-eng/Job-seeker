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

type VDABScraper struct {
	Query    string
	Location string // city or region, e.g. "Aarschot"
}

func NewVDAB(query string) *VDABScraper {
	return &VDABScraper{Query: query}
}

func NewVDABWithLocation(query, location string) *VDABScraper {
	return &VDABScraper{Query: query, Location: location}
}

func (s *VDABScraper) Fetch() ([]*models.Job, error) {
	params := url.Values{}
	params.Set("q", s.Query)
	if s.Location != "" {
		params.Set("l", s.Location)
		params.Set("radius", "30")
	}

	rawURL := "https://www.vdab.be/jobs?" + params.Encode()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	req.Header.Set("Accept-Language", "nl-BE,nl;q=0.9,en;q=0.8")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("vdab fetch: %w", err)
	}
	defer resp.Body.Close()

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("vdab parse: %w", err)
	}

	var jobs []*models.Job
	doc.Find("article.job-result, li.job-result, .vacancy-result").Each(func(_ int, sel *goquery.Selection) {
		title := strings.TrimSpace(sel.Find("h2, h3, .job-title, .vacancy-title").First().Text())
		company := strings.TrimSpace(sel.Find(".company-name, .employer, .bedrijf").First().Text())
		location := strings.TrimSpace(sel.Find(".location, .locatie, .gemeente").First().Text())
		href, _ := sel.Find("a").First().Attr("href")

		if title == "" || href == "" {
			return
		}
		if !strings.HasPrefix(href, "http") {
			href = "https://www.vdab.be" + href
		}

		id := fmt.Sprintf("%x", md5.Sum([]byte(href)))
		jobs = append(jobs, &models.Job{
			ID:        id,
			Title:     title,
			Company:   company,
			Location:  location,
			URL:       href,
			Source:    "vdab",
			PostedAt:  time.Now(),
			FetchedAt: time.Now(),
			Status:    models.StatusNew,
		})
	})
	return jobs, nil
}
