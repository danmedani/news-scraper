package ingest

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/mmcdole/gofeed"
)

// ArticleSummary represents a parsed item from an RSS feed.
type ArticleSummary struct {
	SourceName string
	Title      string
	Link       string
	Summary    string // Usually the RSS description
	Published  *time.Time
}

// FetchFeeds grabs the latest items from all provided feeds concurrently.
// It limits to the top maxItemsPerFeed items.
func FetchFeeds(ctx context.Context, feeds []FeedSource, maxItemsPerFeed int) []ArticleSummary {
	var wg sync.WaitGroup
	var mu sync.Mutex
	var allArticles []ArticleSummary

	fp := gofeed.NewParser()

	for _, feed := range feeds {
		wg.Add(1)
		go func(f FeedSource) {
			defer wg.Done()

			// Parse URL with context to allow cancellation/timeouts
			feedData, err := fp.ParseURLWithContext(f.URL, ctx)
			if err != nil {
				log.Printf("Error parsing feed %s (%s): %v", f.Name, f.URL, err)
				return
			}

			var sourceArticles []ArticleSummary
			for i, item := range feedData.Items {
				if i >= maxItemsPerFeed {
					break
				}

				art := ArticleSummary{
					SourceName: f.Name,
					Title:      item.Title,
					Link:       item.Link,
					Summary:    item.Description,
					Published:  item.PublishedParsed,
				}
				sourceArticles = append(sourceArticles, art)
			}

			mu.Lock()
			allArticles = append(allArticles, sourceArticles...)
			mu.Unlock()

		}(feed)
	}

	wg.Wait()
	fmt.Printf("Fetched %d articles across %d sources\n", len(allArticles), len(feeds))
	return allArticles
}
