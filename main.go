package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/joho/godotenv"
	"github.com/kdub/ag_news/ingest"
	"github.com/kdub/ag_news/services"
)

func main() {
	// Attempt to load .env file, ignore error if it doesn't exist
	_ = godotenv.Load()

	fmt.Println("Starting ag_news digester...")

	// 1. Get the list of feeds we care about
	feeds := ingest.GetFeeds()

	ctx := context.Background()

	// 2. Fetch the top 3 items from each feed to keep the prompt size reasonable
	fmt.Println("Fetching RSS feeds...")
	summaries := ingest.FetchFeeds(ctx, feeds, 3)

	if len(summaries) == 0 {
		log.Println("No articles found across any feeds.")
		return
	}

	// 3. Scrape the full text of each article
	fmt.Println("Scraping full article text...")
	var fullArticles []ingest.ArticleContent
	for _, summary := range summaries {
		// Scrape
		articleData, err := ingest.ScrapeArticle(summary)
		if err != nil {
			log.Printf("Failed to scrape %s: %v", summary.Link, err)
			continue
		}

		// If the article text is too short, it probably hit a hard paywall or captcha
		if len(articleData.FullText) < 100 {
			log.Printf("Skipping %s (too little content scraped)", summary.Link)
			continue
		}

		fullArticles = append(fullArticles, *articleData)
	}

	fmt.Printf("Successfully scraped content for %d articles.\n", len(fullArticles))
	if len(fullArticles) == 0 {
		log.Fatal("Could not scrape enough text to generate a digest.")
	}

	// 4. Send to Gemini for summarization
	fmt.Println("Generating digest with Gemini...")
	digest, err := services.GenerateDigest(ctx, fullArticles)
	if err != nil {
		log.Fatalf("Failed to generate digest: %v", err)
	}

	// 5. Output to Markdown file
	filename := "digest.md"
	err = os.WriteFile(filename, []byte(digest), 0644)
	if err != nil {
		log.Fatalf("Failed to write digest to file: %v", err)
	}

	fmt.Printf("Success! Digest written to %s\n", filename)
}
