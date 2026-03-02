package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"google.golang.org/genai"
)

func getDigestCacheDir() string {
	dir := filepath.Join(".", ".cache", "digests")
	os.MkdirAll(dir, 0755)
	return dir
}

func getDigestHash(articles []CompressedArticle) string {
	links := make([]string, len(articles))
	for i, a := range articles {
		links[i] = a.Link
	}
	return GetDigestHashFromLinks(links)
}

// GetDigestHashFromLinks computes a stable hash from a list of article links
func GetDigestHashFromLinks(links []string) string {
	hashes := make([]string, len(links))
	for i, l := range links {
		h := sha256.Sum256([]byte(l))
		hashes[i] = hex.EncodeToString(h[:])
	}
	sort.Strings(hashes)

	combined := strings.Join(hashes, "")
	finalHash := sha256.Sum256([]byte(combined))
	return hex.EncodeToString(finalHash[:])
}

// IsDigestCached checks if a digest already exists for the given articles
func IsDigestCached(articles []CompressedArticle) bool {
	links := make([]string, len(articles))
	for i, a := range articles {
		links[i] = a.Link
	}
	_, exists := GetCachedDigest(links)
	return exists
}

// GetCachedDigest returns the cached digest content if it exists
func GetCachedDigest(links []string) (string, bool) {
	if len(links) == 0 {
		return "", false
	}
	hash := GetDigestHashFromLinks(links)
	path := filepath.Join(getDigestCacheDir(), hash+".md")
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	return string(data), true
}

// GenerateDigest takes a list of compressed article facts
// and returns a synthesized Markdown digest using Gemini.
func GenerateDigest(ctx context.Context, articles []CompressedArticle) (string, error) {
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

	digestHash := getDigestHash(articles)
	cachePath := filepath.Join(getDigestCacheDir(), digestHash+".md")

	// Check Digest Cache
	cachedData, err := os.ReadFile(cachePath)
	if err == nil {
		log.Println("Serving digest from cache!")
		return string(cachedData), nil
	}

	// 1. Build the prompt with all the compressed articles
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
	finalText := part.Text

	// Save to Digest Cache
	if err := os.WriteFile(cachePath, []byte(finalText), 0644); err != nil {
		log.Printf("Failed to cache digest: %v", err)
	}

	return finalText, nil
}

func buildPrompt(articles []CompressedArticle) string {
	promptText := "You are a professional news digester. I will provide you with a list of extracted facts from several recent news articles covering a SINGLE major event or topic. Your job is to create a well-structured, comprehensive Markdown digest of this particular event based on the given facts.\n\n"
	promptText += "Formatting & Hierarchy Guidelines:\n"
	promptText += "- Use Markdown headers (## and ###) to logically divide the digest into clear sections (e.g., 'Overview', 'Key Developments', 'Impact', 'Background').\n"
	promptText += "- Use bulleted lists appropriately to highlight lists of facts, figures, or key takeaways. Do not just write a flat block of text.\n"
	promptText += "- Provide a cohesive summary of the event as it unfolded across the articles.\n"
	promptText += "- YOU MUST CITE YOUR SOURCES inline. Use markdown links to the original article URLs provided when citing facts, e.g., 'According to [Article 2](https://...), ...'.\n"
	promptText += "- Do NOT hallucinate information not present in the articles.\n\n"

	promptText += "Here are the articles covering this topic:\n\n"

	for i, a := range articles {
		promptText += fmt.Sprintf("--- Article %d ---\n", i+1)
		promptText += fmt.Sprintf("Source: %s\n", a.SourceName)
		promptText += fmt.Sprintf("Title: %s\n", a.Title)
		promptText += fmt.Sprintf("URL: %s\n", a.Link)
		promptText += fmt.Sprintf("Extracted Facts:\n%s\n\n", a.Summary)
	}

	return promptText
}
