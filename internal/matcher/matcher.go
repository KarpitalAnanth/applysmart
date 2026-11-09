package matcher

import (
	"context"

	"applysmart/internal/provider"
)

// Job is a provider.RawJob enriched with an AI assessment of how well a candidate's resume fits it.
type Job struct {
	provider.RawJob

	// MatchScore is the fit between the resume and the job, from 0 to 100.
	MatchScore int
	// MatchReason explains the score: strengths, gaps and missing skills.
	MatchReason string
}

// Analyzer scores a raw job posting against an unstructured resume.
type Analyzer interface {
	AnalyzeJob(ctx context.Context, resumeText string, rawJob provider.RawJob) (*Job, error)
}

// matchResult is the JSON object the LLM is instructed to return.
type matchResult struct {
	MatchScore  int    `json:"match_score"`
	MatchReason string `json:"match_reason"`
}
