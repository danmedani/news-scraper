package services

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/kdub/ag_news/ingest"
	"google.golang.org/genai"
)

// TopicCluster represents a group of related articles.
type TopicCluster struct {
	Title           string                  `json:"title"`
	Description     string                  `json:"description"`
	SourceCounts    map[string]int          `json:"source_counts"`
	Indices         []int                   `json:"article_indices"`
	HasCachedDigest bool                    `json:"has_cached_digest"`
	Articles        []ingest.ArticleSummary `json:"-"` // We map this later
}

// ClusterTopics uses Gemini to group the provided articles into 5-10 major topics.
// We ask Gemini to return JSON so we can easily parse it and display it to the user.
func ClusterTopics(ctx context.Context, existingClusters []*TopicCluster, newArticles []ingest.ArticleSummary) ([]*TopicCluster, error) {
	if len(newArticles) == 0 {
		return existingClusters, nil
	}

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

	promptText := buildClusteringPrompt(existingClusters, newArticles)

	// We use the JSON response schema capability to enforce structured output
	config := &genai.GenerateContentConfig{
		ResponseMIMEType: "application/json",
		Temperature:      genai.Ptr(float32(0.2)),
	}

	resp, err := client.Models.GenerateContent(ctx, "gemini-2.5-flash", genai.Text(promptText), config)
	if err != nil {
		return nil, fmt.Errorf("failed to cluster topics: %w", err)
	}

	if len(resp.Candidates) == 0 || len(resp.Candidates[0].Content.Parts) == 0 {
		return nil, fmt.Errorf("no response generated for clustering")
	}

	part := resp.Candidates[0].Content.Parts[0]
	partText := part.Text

	// The model returns a list of mapping predictions for the NEW articles
	type GeminiMapping struct {
		Title          string `json:"title"`
		Description    string `json:"description"`
		ArticleIndices []int  `json:"article_indices"`
	}

	var predictions []GeminiMapping
	if err := json.Unmarshal([]byte(partText), &predictions); err != nil {
		return nil, fmt.Errorf("failed to parse topic clustering JSON: %w\nData: %s", err, partText)
	}

	clusterMap := make(map[string]*TopicCluster)
	var finalClusters []*TopicCluster

	// Load existing ones into map
	for _, ec := range existingClusters {
		clusterMap[ec.Title] = ec
		finalClusters = append(finalClusters, ec)
	}

	// Process new predictions
	for _, pc := range predictions {
		tc, exists := clusterMap[pc.Title]
		if !exists {
			tc = &TopicCluster{
				Title:        pc.Title,
				Description:  pc.Description,
				SourceCounts: make(map[string]int),
			}
			finalClusters = append(finalClusters, tc)
			clusterMap[pc.Title] = tc
		}

		if tc.SourceCounts == nil {
			tc.SourceCounts = make(map[string]int)
		}

		// Append matching new articles to the cluster
		for _, idx := range pc.ArticleIndices {
			if idx >= 0 && idx < len(newArticles) {
				art := newArticles[idx]
				tc.Articles = append(tc.Articles, art)
				tc.SourceCounts[art.SourceName]++
			}
		}
	}

	return finalClusters, nil
}

func buildClusteringPrompt(existingClusters []*TopicCluster, newArticles []ingest.ArticleSummary) string {
	prompt := "You are an intelligent news aggregator. You will be provided with a list of NEW recent news articles, each with an index, source, and summary.\n"

	if len(existingClusters) > 0 {
		prompt += "You also have a list of EXISTING thematic event topics.\n"
		prompt += "Your job is to read the NEW articles and either match them to an EXISTING topic, or create a NEW topic if they represent a distinct event.\n\n"
		prompt += "EXISTING TOPICS:\n"
		for _, tc := range existingClusters {
			prompt += fmt.Sprintf("- Title: '%s' | Description: '%s'\n", tc.Title, tc.Description)
		}
		prompt += "\n"
	} else {
		prompt += "Your job is to read all the articles and group them into 3 to 7 major thematic event 'topics' (e.g. 'United States Election', 'European Security', 'Global Market Surge').\n"
	}

	prompt += "For each topic that has matching NEW articles, provide:\n"
	prompt += "- title: A short concise title for the topic. (If matching to an existing topic, you MUST use the exact Title string of the existing topic. Otherwise make a new one).\n"
	prompt += "- description: A 1 sentence description of what this topic is about.\n"
	prompt += "- article_indices: An array of integers containing the exact indices of the NEW articles that belong to this topic.\n\n"

	prompt += "Strictly return ONLY a valid JSON array of these topic objects. Do not include markdown code block backticks.\n\n"
	prompt += "NEW ARTICLES:\n"

	for i, a := range newArticles {
		prompt += fmt.Sprintf("[%d] Source: %s | Title: %s\n", i, a.SourceName, a.Title)
		if len(a.Summary) > 0 {
			// truncate summary to keep prompt small
			sum := a.Summary
			if len(sum) > 200 {
				sum = sum[:200] + "..."
			}
			prompt += fmt.Sprintf("    Summary: %s\n", sum)
		}
	}

	return prompt
}
