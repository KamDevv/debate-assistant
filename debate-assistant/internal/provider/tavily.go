package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/yourname/british-debate-assistant/internal/evidence"
)

const tavilySearchEndpoint = "https://api.tavily.com/search"

type Tavily struct {
	apiKey string
	client *http.Client
}

func NewTavily() *Tavily {
	return &Tavily{apiKey: os.Getenv("TAVILY_API_KEY"), client: &http.Client{Timeout: 30 * time.Second}}
}

func (t *Tavily) Available() bool { return t != nil && strings.TrimSpace(t.apiKey) != "" }

type tavilyRequest struct {
	Query          string `json:"query"`
	Topic          string `json:"topic"`
	SearchDepth    string `json:"search_depth"`
	MaxResults     int    `json:"max_results"`
	IncludeAnswer  bool   `json:"include_answer"`
	IncludeRawText bool   `json:"include_raw_content"`
}

type tavilyResponse struct {
	Results []struct {
		Title       string  `json:"title"`
		URL         string  `json:"url"`
		Content     string  `json:"content"`
		Score       float64 `json:"score"`
		PublishedAt string  `json:"published_date"`
	} `json:"results"`
}

func (t *Tavily) Search(ctx context.Context, query string, maxResults int) ([]evidence.Source, error) {
	if !t.Available() {
		return nil, errors.New("TAVILY_API_KEY is not configured")
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, errors.New("search query is empty")
	}
	if maxResults < 1 || maxResults > 10 {
		maxResults = 6
	}
	payload, err := json.Marshal(tavilyRequest{
		Query: query, Topic: "general", SearchDepth: "basic", MaxResults: maxResults,
		IncludeAnswer: false, IncludeRawText: false,
	})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tavilySearchEndpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+t.apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := t.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("web search request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("Tavily search returned HTTP %d", resp.StatusCode)
	}
	var decoded tavilyResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return nil, fmt.Errorf("decode Tavily response: %w", err)
	}
	out := make([]evidence.Source, 0, len(decoded.Results))
	for _, item := range decoded.Results {
		parsed, err := url.Parse(strings.TrimSpace(item.URL))
		if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" {
			continue
		}
		title := strings.TrimSpace(item.Title)
		if title == "" {
			title = parsed.Host
		}
		out = append(out, evidence.Source{
			Title: title, URL: parsed.String(), Domain: parsed.Host,
			Snippet: strings.TrimSpace(item.Content), PublishedAt: strings.TrimSpace(item.PublishedAt), Score: item.Score,
		})
	}
	for i := range out {
		out[i].ID = fmt.Sprintf("S%d", i+1)
	}
	return out, nil
}

// SearchMany runs a small, fixed number of independent searches concurrently,
// then deduplicates results by URL and assigns stable source IDs.
func (t *Tavily) SearchMany(ctx context.Context, queries []string, perQuery int, limit int) ([]evidence.Source, error) {
	if !t.Available() {
		return nil, errors.New("TAVILY_API_KEY is not configured")
	}
	if len(queries) == 0 {
		return nil, errors.New("at least one search query is required")
	}
	type result struct {
		sources []evidence.Source
		err     error
	}
	results := make([]result, len(queries))
	var wg sync.WaitGroup
	for i, query := range queries {
		wg.Add(1)
		go func(i int, query string) {
			defer wg.Done()
			results[i].sources, results[i].err = t.Search(ctx, query, perQuery)
		}(i, query)
	}
	wg.Wait()
	seen := map[string]bool{}
	var merged []evidence.Source
	var firstErr error
	for _, result := range results {
		if result.err != nil {
			if firstErr == nil {
				firstErr = result.err
			}
			continue
		}
		for _, source := range result.sources {
			key := strings.ToLower(strings.TrimRight(source.URL, "/"))
			if seen[key] {
				continue
			}
			seen[key] = true
			merged = append(merged, source)
		}
	}
	if len(merged) == 0 && firstErr != nil {
		return nil, firstErr
	}
	if limit > 0 && len(merged) > limit {
		merged = merged[:limit]
	}
	for i := range merged {
		merged[i].ID = fmt.Sprintf("S%d", i+1)
	}
	return merged, nil
}
