package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const geminiEndpoint = "https://generativelanguage.googleapis.com/v1beta/models/"

type Gemini struct {
	apiKey     string
	model      string
	client     *http.Client
	maxRetries int
}

func NewGemini() *Gemini {
	return &Gemini{
		apiKey:     os.Getenv("GEMINI_API_KEY"),
		model:      env("GEMINI_MODEL", "gemini-3.8-flash"),
		client:     &http.Client{Timeout: durationEnv("GEMINI_TIMEOUT", 240*time.Second)},
		maxRetries: intEnv("GEMINI_MAX_RETRIES", 2, 0, 5),
	}
}

func (g *Gemini) Available() bool { return g != nil && strings.TrimSpace(g.apiKey) != "" }
func (g *Gemini) Model() string {
	if g == nil {
		return ""
	}
	return g.model
}

// GenerateJSON asks Gemini for a JSON response and decodes it into out. The
// caller remains responsible for validating IDs and other untrusted fields.
func (g *Gemini) GenerateJSON(ctx context.Context, system, prompt string, out any) error {
	if !g.Available() {
		return errors.New("GEMINI_API_KEY is not configured; create an API key in Google AI Studio")
	}
	payload := map[string]any{
		"systemInstruction": map[string]any{
			"parts": []map[string]string{{"text": system}},
		},
		"contents": []map[string]any{{
			"role":  "user",
			"parts": []map[string]string{{"text": prompt}},
		}},
		"generationConfig": map[string]any{
			"responseMimeType": "application/json",
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode Gemini request: %w", err)
	}
	endpoint := geminiEndpoint + url.PathEscape(g.model) + ":generateContent"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create Gemini request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-goog-api-key", g.apiKey)
	var resp *http.Response
	for attempt := 0; attempt <= g.maxRetries; attempt++ {
		resp, err = g.client.Do(req)
		if err == nil && resp.StatusCode >= 200 && resp.StatusCode < 300 {
			break
		}
		retryable := err != nil || (resp != nil && (resp.StatusCode == 408 || resp.StatusCode == 429 || resp.StatusCode == 500 || resp.StatusCode == 502 || resp.StatusCode == 503 || resp.StatusCode == 504))
		if !retryable || attempt == g.maxRetries || ctx.Err() != nil {
			if err != nil {
				return fmt.Errorf("call Gemini API after %d attempt(s): %w", attempt+1, err)
			}
			break
		}
		if resp != nil && resp.Body != nil {
			resp.Body.Close()
		}
		delay := time.Duration(1<<attempt) * time.Second
		if delay > 15*time.Second {
			delay = 15 * time.Second
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return fmt.Errorf("Gemini request cancelled: %w", ctx.Err())
		case <-timer.C:
		}
		req, err = http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("create Gemini retry request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("x-goog-api-key", g.apiKey)
	}
	if resp == nil {
		return errors.New("Gemini API returned no response")
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Read only a small error body for useful provider diagnostics. Never
		// include the request URL, because it contains the API key.
		errorBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		var providerError struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal(errorBody, &providerError) == nil && providerError.Error.Message != "" {
			return fmt.Errorf("Gemini API returned HTTP %d: %s", resp.StatusCode, providerError.Error.Message)
		}
		return fmt.Errorf("Gemini API returned HTTP %d", resp.StatusCode)
	}
	var envelope struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
			FinishReason string `json:"finishReason"`
		} `json:"candidates"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(&envelope); err != nil {
		return fmt.Errorf("decode Gemini response: %w", err)
	}
	if len(envelope.Candidates) == 0 {
		return errors.New("Gemini returned no candidates")
	}
	var text string
	for _, part := range envelope.Candidates[0].Content.Parts {
		if part.Text != "" {
			text += part.Text
		}
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return fmt.Errorf("Gemini returned no text (finish reason: %s)", envelope.Candidates[0].FinishReason)
	}
	// JSON mode normally returns plain JSON. This small fallback also handles
	// accidental Markdown fences without weakening the JSON decoder.
	text = strings.TrimPrefix(text, "```json")
	text = strings.TrimPrefix(text, "```")
	text = strings.TrimSuffix(text, "```")
	text = strings.TrimSpace(text)
	if err := json.Unmarshal([]byte(text), out); err != nil {
		return fmt.Errorf("Gemini returned invalid JSON: %w", err)
	}
	return nil
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func durationEnv(key string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	d, err := time.ParseDuration(value)
	if err != nil || d <= 0 {
		return fallback
	}
	return d
}
func intEnv(key string, fallback, min, max int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	n, err := strconv.Atoi(value)
	if err != nil || n < min {
		return fallback
	}
	if n > max {
		return max
	}
	return n
}
