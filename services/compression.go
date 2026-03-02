package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"

	"github.com/kdub/ag_news/ingest"
	"google.golang.org/genai"
)

// CompressedArticle holds the facts extracted from a raw HTML scrape
type CompressedArticle struct {
	Link       string `json:"link"`
	Title      string `json:"title"`
	SourceName string `json:"source_name"`
	Summary    string `json:"summary"` // Fact-extracted text from Gemini
}

func getArticleCacheDir() string {
	dir := filepath.Join(".", ".cache", "articles")
	os.MkdirAll(dir, 0755)
	return dir
}

func getArticleHash(link string) string {
	hash := sha256.Sum256([]byte(link))
	return hex.EncodeToString(hash[:])
}

// CompressArticles takes a slice of raw full articles and compresses them concurrently via Gemini.
// It checks the local disk cache first before calling the LLM.
func CompressArticles(ctx context.Context, articles []ingest.ArticleContent) ([]CompressedArticle, error) {
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("GEMINI_API_KEY environment variable is not set")
	}

	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey: apiKey,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create genai client: %w", err)
	}

	cacheDir := getArticleCacheDir()
	results := make([]CompressedArticle, len(articles))
	var wg sync.WaitGroup
	var errMu sync.Mutex
	var firstErr error

	// Concurrency limit semaphore to avoid 429 Too Many Requests
	sem := make(chan struct{}, 5)

	for i, art := range articles {
		wg.Add(1)
		go func(idx int, article ingest.ArticleContent) {
			defer wg.Done()

			// Check for early exit globally
			errMu.Lock()
			if firstErr != nil {
				errMu.Unlock()
				return
			}
			errMu.Unlock()

			hash := getArticleHash(article.Link)
			cachePath := filepath.Join(cacheDir, hash+".json")

			// 1. Check local cache
			data, err := os.ReadFile(cachePath)
			if err == nil {
				var cached CompressedArticle
				if json.Unmarshal(data, &cached) == nil {
					// log.Printf("Cache hit for article: %s", article.Title)
					results[idx] = cached
					return
				}
			}

			log.Printf("Cache miss. Starting compression for: %s", article.Title)

			// 2. Fetch via LLM API (acquire semaphore context)
			sem <- struct{}{}
			defer func() { <-sem }()

			prompt := "Extract the critical facts, figures, dates, and key information from the following news article text. Ignore boilerplate, ads, or unrelated links. Keep it concise (a few bullet points or a short paragraph) representing the raw facts to be used later in a synthesized digest.\n\n"
			prompt += fmt.Sprintf("Title: %s\nText:\n%s", article.Title, article.FullText)

			var summary string
			resp, err := client.Models.GenerateContent(ctx, "gemini-2.5-flash", genai.Text(prompt), nil)
			if err != nil {
				log.Printf("Failed to compress article '%s': %v", article.Title, err)
				errMu.Lock()
				if firstErr == nil {
					firstErr = err
				}
				errMu.Unlock()
				return
			}

			if len(resp.Candidates) > 0 && len(resp.Candidates[0].Content.Parts) > 0 {
				summary = resp.Candidates[0].Content.Parts[0].Text
			} else {
				summary = "Failed to extract summary."
			}

			comp := CompressedArticle{
				Link:       article.Link,
				Title:      article.Title,
				SourceName: article.SourceName,
				Summary:    summary,
			}

			results[idx] = comp

			// Save to cache
			if b, err := json.MarshalIndent(comp, "", "  "); err == nil {
				os.WriteFile(cachePath, b, 0644)
			}

		}(i, art)
	}

	wg.Wait()

	if firstErr != nil {
		return nil, firstErr
	}

	return results, nil
}
