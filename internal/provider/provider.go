package provider

import "context"

// RawJob is a job posting returned by an upstream provider before deduplication.
type RawJob struct {
	Title       string
	Company     string
	Location    string
	Description string
	SourceURL   string
	ContentHash string
}

// JobProvider fetches job postings matching a search query and location.
type JobProvider interface {
	FetchJobs(ctx context.Context, jobTitle string, location string) ([]RawJob, error)
}
