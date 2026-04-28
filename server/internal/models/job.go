package models

import "time"

type JobStatus string

const (
	StatusNew       JobStatus = "new"
	StatusPipeline  JobStatus = "pipeline"
	StatusApplied   JobStatus = "applied"
	StatusDismissed JobStatus = "dismissed"
)

type Job struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Company     string    `json:"company"`
	Location    string    `json:"location"`
	Description string    `json:"description"`
	URL         string    `json:"url"`
	Source      string    `json:"source"` // "indeed" | "vdab" | "linkedin"
	PostedAt    time.Time `json:"postedAt"`
	FetchedAt   time.Time `json:"fetchedAt"`
	MatchScore  int       `json:"matchScore"`
	MatchReason string    `json:"matchReason"`
	Status      JobStatus `json:"status"`
}
