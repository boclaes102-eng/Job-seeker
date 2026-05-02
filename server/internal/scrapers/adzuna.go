package scrapers

import (
	"context"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"job-seeker/server/internal/models"
)

type AdzunaScraper struct {
	Query      string
	AppID      string
	AppKey     string
	City       string
	Radius     int // km
	MaxDaysOld int // 0 = no limit
}

func NewAdzuna(query, appID, appKey, city string, radius, maxDaysOld int) *AdzunaScraper {
	if city == "" {
		city = "Belgium"
	}
	if radius <= 0 {
		radius = 50
	}
	return &AdzunaScraper{Query: query, AppID: appID, AppKey: appKey, City: city, Radius: radius, MaxDaysOld: maxDaysOld}
}

type adzunaResponse struct {
	Results []adzunaJob `json:"results"`
}

type adzunaJob struct {
	Title       string         `json:"title"`
	Description string         `json:"description"`
	RedirectURL string         `json:"redirect_url"`
	Created     string         `json:"created"`
	Company     adzunaCompany  `json:"company"`
	Location    adzunaLocation `json:"location"`
}

type adzunaCompany struct {
	DisplayName string `json:"display_name"`
}

type adzunaLocation struct {
	DisplayName string `json:"display_name"`
}

func (s *AdzunaScraper) Fetch() ([]*models.Job, error) {
	if s.AppID == "" || s.AppKey == "" {
		return nil, fmt.Errorf("adzuna: ADZUNA_APP_ID and ADZUNA_APP_KEY not set in .env")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	params := url.Values{}
	params.Set("app_id", s.AppID)
	params.Set("app_key", s.AppKey)
	params.Set("what", s.Query)
	params.Set("where", s.City)
	params.Set("distance", fmt.Sprintf("%d", s.Radius))
	params.Set("results_per_page", "20")
	params.Set("sort_by", "date")
	params.Set("content-type", "application/json")
	if s.MaxDaysOld > 0 {
		params.Set("max_days_old", fmt.Sprintf("%d", s.MaxDaysOld))
	}

	apiURL := "https://api.adzuna.com/v1/api/jobs/be/search/1?" + params.Encode()
	slog.Debug("adzuna.request", "query", s.Query, "city", s.City, "radius_km", s.Radius, "url", apiURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		slog.Error("adzuna.http_error", "query", s.Query, "err", err)
		return nil, fmt.Errorf("adzuna fetch: %w", err)
	}
	defer resp.Body.Close()
	slog.Debug("adzuna.response", "query", s.Query, "status", resp.StatusCode)

	if resp.StatusCode != http.StatusOK {
		slog.Error("adzuna.bad_status", "query", s.Query, "status", resp.Status)
		return nil, fmt.Errorf("adzuna fetch: %s", resp.Status)
	}

	var result adzunaResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("adzuna decode: %w", err)
	}
	slog.Debug("adzuna.result", "query", s.Query, "count", len(result.Results))

	var jobs []*models.Job
	for _, item := range result.Results {
		posted := time.Now()
		if t, err := time.Parse(time.RFC3339, item.Created); err == nil {
			posted = t
		}
		id := fmt.Sprintf("%x", md5.Sum([]byte(item.RedirectURL)))
		jobs = append(jobs, &models.Job{
			ID:          id,
			Title:       item.Title,
			Company:     item.Company.DisplayName,
			Location:    item.Location.DisplayName,
			Description: item.Description,
			URL:         item.RedirectURL,
			Source:      "adzuna",
			PostedAt:    posted,
			FetchedAt:   time.Now(),
			Status:      models.StatusNew,
		})
	}
	return jobs, nil
}
