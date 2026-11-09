package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const jsearchEndpoint = "https://jsearch.p.rapidapi.com/search"

// JSearchProvider fetches postings from the JSearch API on RapidAPI.
type JSearchProvider struct {
	apiKey string
	host   string
	client *http.Client
}

func NewJSearchProvider(apiKey, host string) *JSearchProvider {
	return &JSearchProvider{
		apiKey: apiKey,
		host:   host,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

type jsearchResponse struct {
	Status string       `json:"status"`
	Data   []jsearchJob `json:"data"`
	Error  *struct {
		Message string `json:"message"`
	} `json:"error"`
}

type jsearchJob struct {
	JobTitle       string `json:"job_title"`
	EmployerName   string `json:"employer_name"`
	JobLocation    string `json:"job_location"`
	JobCity        string `json:"job_city"`
	JobState       string `json:"job_state"`
	JobCountry     string `json:"job_country"`
	JobDescription string `json:"job_description"`
	JobApplyLink   string `json:"job_apply_link"`
}

// FetchJobs orchestrates validating credentials, building the request,
// executing the HTTP call, and converting the payload.
func (p *JSearchProvider) FetchJobs(ctx context.Context, jobTitle, location string) ([]RawJob, error) {
	if p.apiKey == "" || p.host == "" {
		return nil, errors.New("jsearch: API key and host must be set")
	}

	searchQuery, err := buildSearchQuery(jobTitle, location)
	if err != nil {
		return nil, err
	}

	req, err := p.newSearchRequest(ctx, searchQuery)
	if err != nil {
		return nil, fmt.Errorf("jsearch: build request: %w", err)
	}

	payload, err := p.executeAndParseResponse(req)
	if err != nil {
		return nil, err
	}

	return toRawJobs(payload.Data), nil
}

// buildSearchQuery combines jobTitle and location into JSearch search syntax.
func buildSearchQuery(jobTitle, location string) (string, error) {
	title := strings.TrimSpace(jobTitle)
	if title == "" {
		return "", errors.New("jsearch: job title is empty")
	}

	if loc := strings.TrimSpace(location); loc != "" {
		return title + " in " + loc, nil
	}
	return title, nil
}

// newSearchRequest constructs the HTTP GET request with parameters and required RapidAPI headers.
func (p *JSearchProvider) newSearchRequest(ctx context.Context, searchQuery string) (*http.Request, error) {
	params := url.Values{}
	params.Set("query", searchQuery)
	params.Set("page", "1")
	params.Set("num_pages", "1")

	reqURL := jsearchEndpoint + "?" + params.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("X-RapidAPI-Key", p.apiKey)
	req.Header.Set("X-RapidAPI-Host", p.host)
	req.Header.Set("Accept", "application/json")

	return req, nil
}

// executeAndParseResponse executes the HTTP request and handles error/JSON decoding.
func (p *JSearchProvider) executeAndParseResponse(req *http.Request) (*jsearchResponse, error) {
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("jsearch: request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("jsearch: unexpected status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var payload jsearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("jsearch: decode response: %w", err)
	}

	if payload.Error != nil {
		return nil, fmt.Errorf("jsearch: api error: %s", payload.Error.Message)
	}

	return &payload, nil
}

// toRawJobs maps a slice of API models to domain RawJob slice.
func toRawJobs(items []jsearchJob) []RawJob {
	jobs := make([]RawJob, len(items))
	for i, j := range items {
		jobs[i] = j.toRawJob()
	}
	return jobs
}

func (j jsearchJob) toRawJob() RawJob {
	return RawJob{
		Title:       strings.TrimSpace(j.JobTitle),
		Company:     strings.TrimSpace(j.EmployerName),
		Location:    j.location(),
		Description: strings.TrimSpace(j.JobDescription),
		SourceURL:   strings.TrimSpace(j.JobApplyLink),
	}
}

// location prefers JSearch's pre-formatted job_location and falls back to
// joining the city, state and country fields.
func (j jsearchJob) location() string {
	if loc := strings.TrimSpace(j.JobLocation); loc != "" {
		return loc
	}
	var parts []string
	for _, s := range []string{j.JobCity, j.JobState, j.JobCountry} {
		if s = strings.TrimSpace(s); s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, ", ")
}
