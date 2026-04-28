package scrapers

import (
	"context"
	"crypto/md5"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/mmcdole/gofeed"
	"job-seeker/server/internal/models"
)

type JobatScraper struct {
	Query    string
	Location string
}

func NewJobat(query, location string) *JobatScraper {
	return &JobatScraper{Query: query, Location: location}
}

func (s *JobatScraper) Fetch() ([]*models.Job, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	params := url.Values{}
	params.Set("keywords", s.Query)

	feedURL := "https://www.jobat.be/nl/vacatures/rss?" + params.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, feedURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	req.Header.Set("Accept", "application/rss+xml, application/xml, text/xml, */*")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("jobat fetch: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("jobat fetch: %s", resp.Status)
	}

	fp := gofeed.NewParser()
	feed, err := fp.Parse(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("jobat parse: %w", err)
	}

	var jobs []*models.Job
	for _, item := range feed.Items {
		posted := time.Now()
		if item.PublishedParsed != nil {
			posted = *item.PublishedParsed
		}

		company := ""
		if item.Author != nil {
			company = item.Author.Name
		}

		// Extract location from categories or description
		location := s.Location
		for _, cat := range item.Categories {
			if strings.Contains(strings.ToLower(cat), "antwerp") ||
				strings.Contains(strings.ToLower(cat), "brussel") ||
				strings.Contains(strings.ToLower(cat), "leuven") ||
				strings.Contains(strings.ToLower(cat), "gent") ||
				strings.Contains(strings.ToLower(cat), "hasselt") {
				location = cat
				break
			}
		}

		id := fmt.Sprintf("%x", md5.Sum([]byte(item.Link)))
		jobs = append(jobs, &models.Job{
			ID:          id,
			Title:       item.Title,
			Company:     company,
			Location:    location,
			Description: html.UnescapeString(item.Description),
			URL:         item.Link,
			Source:      "jobat",
			PostedAt:    posted,
			FetchedAt:   time.Now(),
			Status:      models.StatusNew,
		})
	}
	return jobs, nil
}
