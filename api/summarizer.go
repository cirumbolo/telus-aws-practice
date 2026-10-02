package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

// Summarizer turns a note's text into a short summary. It is the seam for the
// LLM call, so handlers can be tested with a fake and the provider swapped
// without touching them.
type Summarizer interface {
	Summarize(ctx context.Context, text string) (string, error)
}

// FuelIXSummarizer calls FuelIX's OpenAI-compatible chat completions endpoint.
type FuelIXSummarizer struct {
	baseURL string
	apiKey  string
	model   string
	client  *http.Client
}

func NewFuelIXSummarizer(baseURL, apiKey, model string) *FuelIXSummarizer {
	return &FuelIXSummarizer{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		model:   model,
		client:  &http.Client{Timeout: 30 * time.Second},
	}
}

const summarizePrompt = "Summarize the whole document in 2-3 sentences of plain text, " +
	"describing what the overall context of the page is about. Reply with the summary only."

func (s *FuelIXSummarizer) Summarize(ctx context.Context, text string) (string, error) {
	body, err := json.Marshal(map[string]any{
		"model": s.model,
		"messages": []map[string]string{
			{"role": "system", "content": summarizePrompt},
			{"role": "user", "content": text},
		},
	})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.apiKey)

	res, err := s.client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()

	if res.StatusCode/100 != 2 {
		// Logged for operators; never forwarded to the browser.
		b, _ := io.ReadAll(io.LimitReader(res.Body, 1<<10))
		log.Printf("fuelix: status %d: %s", res.StatusCode, b)
		return "", fmt.Errorf("fuelix: unexpected status %d", res.StatusCode)
	}

	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("fuelix: decode response: %w", err)
	}
	if len(out.Choices) == 0 || strings.TrimSpace(out.Choices[0].Message.Content) == "" {
		return "", errors.New("fuelix: empty response")
	}
	return strings.TrimSpace(out.Choices[0].Message.Content), nil
}
