package services

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// WikipediaSummary represents the short extract from a query
type WikipediaSummary struct {
	Title   string `json:"title"`
	Extract string `json:"extract"`
	URL     string `json:"content_urls"`
}

// SearchWikipedia searches the Wikipedia REST API for the given query and returns a summary excerpt.
func SearchWikipedia(query string) (*WikipediaSummary, error) {
	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	// We'll use the search endpoint to find the exact article name first,
	// then fetch its summary.
	searchURL := fmt.Sprintf("https://en.wikipedia.org/w/api.php?action=query&list=search&srsearch=%s&utf8=&format=json", url.QueryEscape(query))

	req, err := http.NewRequest("GET", searchURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create search req: %w", err)
	}
	req.Header.Set("User-Agent", "AGNews/1.0 (https://github.com/kdub/ag_news)")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("search req failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bad status code on search: %d", resp.StatusCode)
	}

	var searchRes struct {
		Query struct {
			Search []struct {
				Title string `json:"title"`
			} `json:"search"`
		} `json:"query"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&searchRes); err != nil {
		return nil, fmt.Errorf("failed to decode search json: %w", err)
	}

	if len(searchRes.Query.Search) == 0 {
		return nil, fmt.Errorf("no wikipedia articles found for query: %s", query)
	}

	bestMatchTitle := searchRes.Query.Search[0].Title

	// Now fetch the summary for the best match title
	return fetchSummary(bestMatchTitle)
}

func fetchSummary(title string) (*WikipediaSummary, error) {
	client := &http.Client{Timeout: 5 * time.Second}

	// Use the REST API for clean extracts
	summaryURL := fmt.Sprintf("https://en.wikipedia.org/api/rest_v1/page/summary/%s", url.PathEscape(title))

	req, err := http.NewRequest("GET", summaryURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "AGNews/1.0 (https://github.com/kdub/ag_news)")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bad status code on summary: %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var parsed struct {
		Title       string `json:"title"`
		Extract     string `json:"extract"`
		ContentURLs struct {
			Desktop struct {
				Page string `json:"page"`
			} `json:"desktop"`
		} `json:"content_urls"`
	}

	if err := json.Unmarshal(bodyBytes, &parsed); err != nil {
		return nil, err
	}

	return &WikipediaSummary{
		Title:   parsed.Title,
		Extract: parsed.Extract,
		URL:     parsed.ContentURLs.Desktop.Page,
	}, nil
}
