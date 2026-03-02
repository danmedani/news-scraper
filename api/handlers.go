package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/kdub/ag_news/ingest"
	"github.com/kdub/ag_news/services"
)

// AppState holds the global state for our simple in-memory backend
type AppState struct {
	Clusters  []*services.TopicCluster
	Feeds     []ingest.FeedSource
	FeedStats map[string]ingest.FeedStatus
}

// TopicsResponse wraps the clustered topics and the raw feed health stats
type TopicsResponse struct {
	Clusters  []*services.TopicCluster     `json:"clusters"`
	FeedStats map[string]ingest.FeedStatus `json:"feed_stats"`
}

func NewAppState() *AppState {
	return &AppState{
		Feeds: ingest.GetFeeds(),
	}
}

// HandleGetTopics fetches the RSS feeds, clusters them with Gemini, and returns the JSON.
func (app *AppState) HandleGetTopics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx := context.Background()

	// Parse limit and refresh from query
	limit := 15
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 50 {
			limit = l
		}
	}
	refresh := r.URL.Query().Get("refresh") != "false"

	// 1. If not refreshing, try to load from cache immediately
	if !refresh {
		clusters, err := services.LoadClustersCache()
		if err == nil && len(clusters) > 0 {
			log.Printf("Loading topics from cache (refresh=false)...\n")
			// Reconstruct basic feed stats from cached data
			feedStats := make(map[string]ingest.FeedStatus)
			for _, f := range app.Feeds {
				feedStats[f.Name] = ingest.FeedStatus{Name: f.Name, URL: f.URL}
			}
			for _, c := range clusters {
				for _, a := range c.Articles {
					if s, ok := feedStats[a.SourceName]; ok {
						s.CachedCount++
						feedStats[a.SourceName] = s
					}
				}
			}

			app.Clusters = clusters
			app.FeedStats = feedStats

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(TopicsResponse{
				Clusters:  clusters,
				FeedStats: feedStats,
			})
			return
		}
		log.Println("No cache found or error loading cache, proceeding to fetch...")
	}

	// 1. Fetch RSS items
	log.Printf("Fetching feeds with limit %d...\n", limit)
	summaries, feedStats := ingest.FetchFeeds(ctx, app.Feeds, limit)

	if len(summaries) == 0 {
		http.Error(w, `{"error": "No articles found"}`, http.StatusInternalServerError)
		return
	}

	// Load Cache
	existingClusters, err := services.LoadClustersCache()
	if err != nil {
		log.Printf("Could not load cache: %v", err)
		existingClusters = []*services.TopicCluster{}
	}

	// Initialize feedStats with all known sources from app.Feeds
	// This ensures even sources that failed to fetch show up in the health panel.
	feedStatsFull := make(map[string]ingest.FeedStatus)
	for _, f := range app.Feeds {
		if s, ok := feedStats[f.Name]; ok {
			feedStatsFull[f.Name] = s
		} else {
			feedStatsFull[f.Name] = ingest.FeedStatus{
				Name: f.Name,
				URL:  f.URL,
			}
		}
	}

	// Identify new articles and populate cached counts from existing data
	existingLinks := make(map[string]bool)
	for _, c := range existingClusters {
		for _, a := range c.Articles {
			existingLinks[a.Link] = true

			// Match source name case-insensitively
			for name, stat := range feedStatsFull {
				if strings.EqualFold(name, a.SourceName) {
					stat.CachedCount++
					feedStatsFull[name] = stat
					break
				}
			}
		}
	}

	var newArticles []ingest.ArticleSummary
	for _, a := range summaries {
		// Identify which source this belongs to (case-insensitively)
		var matchedName string
		for name := range feedStatsFull {
			if strings.EqualFold(name, a.SourceName) {
				matchedName = name
				break
			}
		}

		if matchedName != "" {
			stat := feedStatsFull[matchedName]
			if !existingLinks[a.Link] {
				newArticles = append(newArticles, a)
				stat.NewCount++
			}
			feedStatsFull[matchedName] = stat
		}
	}

	// Update feedStats for the response
	feedStats = feedStatsFull

	// 2. Cluster
	log.Printf("Clustering %d new articles into %d existing...", len(newArticles), len(existingClusters))
	clusters, err := services.ClusterTopics(ctx, existingClusters, newArticles)
	if err != nil {
		log.Printf("Clustering failed: %v", err)
		if isLimit, wait := services.IsRateLimitError(err); isLimit {
			w.WriteHeader(http.StatusTooManyRequests)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"error":       "API Rate Limited",
				"retry_after": wait,
				"feed_stats":  feedStats,
			})
			return
		}
		http.Error(w, `{"error": "Failed to cluster topics"}`, http.StatusInternalServerError)
		return
	}

	// Save Cache
	log.Printf("Saving %d clusters to disk cache...", len(clusters))
	if err := services.SaveClustersCache(clusters); err != nil {
		log.Printf("Failed to save cluster cache: %v", err)
	}

	// Save to memory so the frontend can retrieve them later
	app.Clusters = clusters
	app.FeedStats = feedStats

	resp := TopicsResponse{
		Clusters:  clusters,
		FeedStats: feedStats,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// HandleGenerateDigest takes a topic ID (index), scrapes its articles, and runs the Gemini summarization.
func (app *AppState) HandleGenerateDigest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	topicIdxStr := r.URL.Query().Get("topicId")
	topicIdx, err := strconv.Atoi(topicIdxStr)
	if err != nil || topicIdx < 0 || topicIdx >= len(app.Clusters) {
		http.Error(w, `{"error": "Invalid topic ID"}`, http.StatusBadRequest)
		return
	}

	selectedCluster := app.Clusters[topicIdx]
	ctx := context.Background()

	// Set headers for SSE if the client supports it or we want to stream
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // Disable proxy buffering

	flusher, ok := w.(http.Flusher)
	if !ok {
		// Fallback to non-streaming if flusher is not available
		log.Println("Streaming not supported by response writer")
	}

	sendProgress := func(step int, message string) {
		event := map[string]interface{}{
			"type":    "progress",
			"step":    step,
			"message": message,
		}
		data, _ := json.Marshal(event)
		fmt.Fprintf(w, "data: %s\n\n", data)
		if flusher != nil {
			flusher.Flush()
		}
	}

	// 1. Scrape full content
	sendProgress(1, fmt.Sprintf("Scraping %d articles and decoding URLs...", len(selectedCluster.Articles)))

	var fullArticles []ingest.ArticleContent
	var skippedArticles []ingest.ArticleSummary

	for _, summary := range selectedCluster.Articles {
		if stat, ok := app.FeedStats[summary.SourceName]; ok {
			stat.SkippedCount = 0
			app.FeedStats[summary.SourceName] = stat
		}
	}

	for _, summary := range selectedCluster.Articles {
		articleData, err := ingest.ScrapeArticle(summary)
		if err != nil {
			log.Printf("Skipping [%s] '%s' (%s) due to error: %v", summary.SourceName, summary.Title, summary.Link, err)
			skippedArticles = append(skippedArticles, summary)
			if stat, ok := app.FeedStats[summary.SourceName]; ok {
				stat.SkippedCount++
				app.FeedStats[summary.SourceName] = stat
			}
			continue
		}

		if len(articleData.FullText) < 100 {
			log.Printf("Skipping [%s] '%s' (%s) - extracted text too short (%d chars)", summary.SourceName, summary.Title, summary.Link, len(articleData.FullText))
			skippedArticles = append(skippedArticles, summary)
			if stat, ok := app.FeedStats[summary.SourceName]; ok {
				stat.SkippedCount++
				app.FeedStats[summary.SourceName] = stat
			}
			continue
		}

		fullArticles = append(fullArticles, *articleData)
	}

	if len(fullArticles) == 0 && len(skippedArticles) == 0 {
		fmt.Fprintf(w, "event: error\ndata: %s\n\n", `{"message": "Could not scrape any articles for this topic."}`)
		return
	}

	// 2. Compress the articles concurrently
	sendProgress(2, fmt.Sprintf("Extracting facts from %d articles...", len(fullArticles)))
	compressedArticles, err := services.CompressArticles(ctx, fullArticles)
	if err != nil {
		log.Printf("Compression failed: %v", err)
		if isLimit, wait := services.IsRateLimitError(err); isLimit {
			event := map[string]interface{}{
				"type":        "error",
				"error":       "API Rate Limited",
				"retry_after": wait,
			}
			data, _ := json.Marshal(event)
			fmt.Fprintf(w, "event: error\ndata: %s\n\n", data)
			return
		}
		fmt.Fprintf(w, "event: error\ndata: %s\n\n", `{"message": "Failed to extract facts from articles"}`)
		return
	}

	// 3. Generate digest using compressed facts
	sendProgress(3, "Synthesizing final digest with Gemini...")
	digest, err := services.GenerateDigest(ctx, compressedArticles)
	if err != nil {
		log.Printf("Digest generation failed: %v", err)
		if isLimit, wait := services.IsRateLimitError(err); isLimit {
			event := map[string]interface{}{
				"type":        "error",
				"error":       "API Rate Limited",
				"retry_after": wait,
			}
			data, _ := json.Marshal(event)
			fmt.Fprintf(w, "event: error\ndata: %s\n\n", data)
			return
		}
		fmt.Fprintf(w, "event: error\ndata: %s\n\n", `{"message": "Failed to generate digest"}`)
		return
	}

	// Final result
	response := map[string]interface{}{
		"type":             "result",
		"title":            selectedCluster.Title,
		"digest":           digest,
		"articles":         selectedCluster.Articles,
		"skipped_articles": skippedArticles,
		"feed_stats":       app.FeedStats,
	}
	data, _ := json.Marshal(response)
	fmt.Fprintf(w, "event: result\ndata: %s\n\n", data)
}
