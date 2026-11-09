package matcher

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"applysmart/internal/provider"
)

const (
	geminiBaseURL      = "https://generativelanguage.googleapis.com/v1beta/models/"
	DefaultGeminiModel = "gemini-2.5-flash"
)

// GeminiAnalyzer implements Analyzer using the Gemini generateContent API.
type GeminiAnalyzer struct {
	apiKey string
	model  string
	client *http.Client
}

// NewGeminiAnalyzer returns an analyzer for the given model. An empty model selects DefaultGeminiModel.
func NewGeminiAnalyzer(apiKey, model string) *GeminiAnalyzer {
	if model == "" {
		model = DefaultGeminiModel
	}
	return &GeminiAnalyzer{
		apiKey: apiKey,
		model:  model,
		client: &http.Client{Timeout: 60 * time.Second},
	}
}

type geminiPart struct {
	Text string `json:"text"`
}

type geminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []geminiPart `json:"parts"`
}

type geminiGenerationConfig struct {
	Temperature      float64        `json:"temperature"`
	ResponseMIMEType string         `json:"responseMimeType"`
	ResponseSchema   map[string]any `json:"responseSchema"`
}

type geminiRequest struct {
	SystemInstruction geminiContent          `json:"systemInstruction"`
	Contents          []geminiContent        `json:"contents"`
	GenerationConfig  geminiGenerationConfig `json:"generationConfig"`
}

type geminiResponse struct {
	Candidates []struct {
		Content      geminiContent `json:"content"`
		FinishReason string        `json:"finishReason"`
	} `json:"candidates"`
	PromptFeedback *struct {
		BlockReason string `json:"blockReason"`
	} `json:"promptFeedback"`
	Error *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Status  string `json:"status"`
	} `json:"error"`
}

// matchResultSchema mirrors matchResult so Gemini's structured output mode enforces the shape server-side, in addition to the prompt instructions.
var matchResultSchema = map[string]any{
	"type": "OBJECT",
	"properties": map[string]any{
		"match_score":  map[string]any{"type": "INTEGER"},
		"match_reason": map[string]any{"type": "STRING"},
	},
	"required":         []string{"match_score", "match_reason"},
	"propertyOrdering": []string{"match_score", "match_reason"},
}

// AnalyzeJob builds the prompts, calls Gemini, and maps the JSON verdict onto an enriched Job.
func (g *GeminiAnalyzer) AnalyzeJob(ctx context.Context, resumeText string, rawJob provider.RawJob) (*Job, error) {
	if g.apiKey == "" {
		return nil, errors.New("matcher: gemini API key must be set")
	}
	if strings.TrimSpace(resumeText) == "" {
		return nil, errors.New("matcher: resume text is empty")
	}
	if strings.TrimSpace(rawJob.Description) == "" && strings.TrimSpace(rawJob.Title) == "" {
		return nil, errors.New("matcher: job has no title or description")
	}

	prompt, err := renderUserPrompt(resumeText, rawJob)
	if err != nil {
		return nil, fmt.Errorf("matcher: render prompt: %w", err)
	}

	text, err := g.generate(ctx, prompt)
	if err != nil {
		return nil, err
	}

	result, err := parseMatchResult(text)
	if err != nil {
		return nil, err
	}

	return &Job{
		RawJob:      rawJob,
		MatchScore:  result.MatchScore,
		MatchReason: result.MatchReason,
	}, nil
}

// generate sends a single-turn request and returns the model's raw text output.
func (g *GeminiAnalyzer) generate(ctx context.Context, userPrompt string) (string, error) {
	body, err := json.Marshal(geminiRequest{
		SystemInstruction: geminiContent{Parts: []geminiPart{{Text: systemPrompt}}},
		Contents:          []geminiContent{{Role: "user", Parts: []geminiPart{{Text: userPrompt}}}},
		GenerationConfig: geminiGenerationConfig{
			Temperature:      0.0,
			ResponseMIMEType: "application/json",
			ResponseSchema:   matchResultSchema,
		},
	})
	if err != nil {
		return "", fmt.Errorf("matcher: encode request: %w", err)
	}

	url := geminiBaseURL + g.model + ":generateContent"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("matcher: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-goog-api-key", g.apiKey)

	resp, err := g.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("matcher: gemini request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return "", fmt.Errorf("matcher: gemini unexpected status %d: %s", resp.StatusCode, strings.TrimSpace(string(msg)))
	}

	var payload geminiResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", fmt.Errorf("matcher: decode gemini response: %w", err)
	}

	return payload.text()
}

func (r *geminiResponse) text() (string, error) {
	if r.Error != nil {
		return "", fmt.Errorf("matcher: gemini api error %d (%s): %s", r.Error.Code, r.Error.Status, r.Error.Message)
	}
	if r.PromptFeedback != nil && r.PromptFeedback.BlockReason != "" {
		return "", fmt.Errorf("matcher: gemini blocked prompt: %s", r.PromptFeedback.BlockReason)
	}
	if len(r.Candidates) == 0 {
		return "", errors.New("matcher: gemini returned no candidates")
	}

	c := r.Candidates[0]
	var b strings.Builder
	for _, p := range c.Content.Parts {
		b.WriteString(p.Text)
	}
	if b.Len() == 0 {
		return "", fmt.Errorf("matcher: gemini returned empty content (finish reason %q)", c.FinishReason)
	}
	return b.String(), nil
}

// parseMatchResult decodes and validates the model output. It tolerates a stray markdown fence, which models occasionally emit despite instructions.
func parseMatchResult(text string) (*matchResult, error) {
	cleaned := strings.TrimSpace(text)
	cleaned = strings.TrimPrefix(cleaned, "```json")
	cleaned = strings.TrimPrefix(cleaned, "```")
	cleaned = strings.TrimSuffix(cleaned, "```")
	cleaned = strings.TrimSpace(cleaned)

	var result matchResult
	dec := json.NewDecoder(strings.NewReader(cleaned))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&result); err != nil {
		return nil, fmt.Errorf("matcher: unmarshal match result: %w", err)
	}

	if result.MatchScore < 0 || result.MatchScore > 100 {
		return nil, fmt.Errorf("matcher: match score %d out of range 0-100", result.MatchScore)
	}
	result.MatchReason = strings.TrimSpace(result.MatchReason)
	if result.MatchReason == "" {
		return nil, errors.New("matcher: match reason is empty")
	}
	return &result, nil
}
