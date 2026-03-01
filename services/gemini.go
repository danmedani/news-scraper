package services

import (
	"context"
	"fmt"
	"os"

	"github.com/kdub/ag_news/ingest"
	"google.golang.org/genai"
)

// GenerateDigest takes a list of ingested articles, crosses them with Wikipedia context
// (by asking the model first what to query, or passing it directly), and returns a Markdown digest.
func GenerateDigest(ctx context.Context, articles []ingest.ArticleContent) (string, error) {
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		return "", fmt.Errorf("GEMINI_API_KEY environment variable is not set")
	}

	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey: apiKey,
	})
	if err != nil {
		return "", fmt.Errorf("failed to create genai client: %w", err)
	}

	// 1. Build the prompt with all the articles
	promptText := buildPrompt(articles)

	// In a fully robust system, we would:
	// 1. Ask Gemini to extract key *events* from the articles.
	// 2. Fetch those events from Wikipedia using services.SearchWikipedia.
	// 3. Feed the Wikipedia summaries back into a final generation step.
	//
	promptText += "\n\nCRITICAL: If you mention specific recent events or ongoing geopolitical/economic situations, you MUST ask the user to provide Wikipedia summaries for them. However, since I cannot provide them interactively right now, please synthesize the provided article text into a concise, easy-to-read Markdown digest. Group by topic."

	resp, err := client.Models.GenerateContent(ctx, "gemini-2.5-flash", genai.Text(promptText), nil)
	if err != nil {
		return "", fmt.Errorf("failed to generate digest: %w", err)
	}

	if len(resp.Candidates) == 0 || len(resp.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("no response generated")
	}

	part := resp.Candidates[0].Content.Parts[0]
	return part.Text, nil
}

func buildPrompt(articles []ingest.ArticleContent) string {
	prompt := "You are a professional news digester. I will provide you with several recent news articles from various sources. Your job is to create a concise, well-structured Markdown digest of the current events.\n\n"
	prompt += "Guidelines:\n"
	prompt += "- Group related articles into single thematic sections (e.g., 'Global Politics', 'Markets', 'Technology').\n"
	prompt += "- Provide a 2-3 sentence summary of the event.\n"
	prompt += "- YOU MUST CITE YOUR SOURCES. Always include inline links to the original article URLs provided.\n"
	prompt += "- Do NOT hallucinate information not present in the articles.\n\n"

	prompt += "Here are the articles:\n\n"

	for i, a := range articles {
		prompt += fmt.Sprintf("--- Article %d ---\n", i+1)
		prompt += fmt.Sprintf("Source: %s\n", a.SourceName)
		prompt += fmt.Sprintf("Title: %s\n", a.Title)
		prompt += fmt.Sprintf("URL: %s\n", a.Link)
		prompt += fmt.Sprintf("Content:\n%s\n\n", a.FullText)
	}

	return prompt
}
