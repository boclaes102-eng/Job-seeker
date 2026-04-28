package scrapers

import (
	"compress/gzip"
	"crypto/md5"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/mmcdole/gofeed"
	"job-seeker/server/internal/models"
)

type IndeedScraper struct {
	Query    string
	Location string
}

func NewIndeed(query, location string) *IndeedScraper {
	if location == "" {
		location = "Belgium"
	}
	return &IndeedScraper{Query: query, Location: location}
}

// browserTransport injects realistic browser headers so Indeed doesn't 403.
type browserTransport struct{ base http.RoundTripper }

func (t *browserTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8")
	req.Header.Set("Accept-Language", "nl-BE,nl;q=0.9,en-US;q=0.8,en;q=0.7")
	req.Header.Set("Accept-Encoding", "gzip, deflate, br")
	req.Header.Set("Referer", "https://be.indeed.com/")
	req.Header.Set("Cache-Control", "max-age=0")
	req.Header.Set("Sec-Fetch-Dest", "document")
	req.Header.Set("Sec-Fetch-Mode", "navigate")
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	req.Header.Set("Upgrade-Insecure-Requests", "1")
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(req)
}

func (s *IndeedScraper) Fetch() ([]*models.Job, error) {
	params := url.Values{}
	params.Set("q", s.Query)
	params.Set("l", s.Location)
	params.Set("sort", "date")
	params.Set("fromage", "30")
	params.Set("radius", "50")
	params.Set("format", "rss")

	feedURL := "https://be.indeed.com/jobs?" + params.Encode()

	client := &http.Client{
		Transport: &browserTransport{},
		Timeout:   20 * time.Second,
	}

	req, err := http.NewRequest("GET", feedURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("indeed fetch: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("indeed fetch: http error: %s", resp.Status)
	}

	var reader io.Reader = resp.Body
	if resp.Header.Get("Content-Encoding") == "gzip" {
		gz, err := gzip.NewReader(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("indeed gzip: %w", err)
		}
		defer gz.Close()
		reader = gz
	}

	fp := gofeed.NewParser()
	feed, err := fp.Parse(reader)
	if err != nil {
		return nil, fmt.Errorf("indeed parse: %w", err)
	}

	var jobs []*models.Job
	for _, item := range feed.Items {
		posted := time.Now()
		if item.PublishedParsed != nil {
			posted = *item.PublishedParsed
		}
		id := fmt.Sprintf("%x", md5.Sum([]byte(item.Link)))
		jobs = append(jobs, &models.Job{
			ID:          id,
			Title:       item.Title,
			Company:     extractIndeedCompany(item),
			Location:    s.Location,
			Description: html.UnescapeString(item.Description),
			URL:         item.Link,
			Source:      "indeed",
			PostedAt:    posted,
			FetchedAt:   time.Now(),
			Status:      models.StatusNew,
		})
	}
	return jobs, nil
}

func extractIndeedCompany(item *gofeed.Item) string {
	if item.Author != nil && item.Author.Name != "" {
		return item.Author.Name
	}
	for i := len(item.Title) - 1; i >= 0; i-- {
		if item.Title[i] == '-' && i > 0 && item.Title[i-1] == ' ' {
			return strings.TrimSpace(item.Title[i+2:])
		}
	}
	return "Unknown"
}
