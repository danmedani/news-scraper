package ingest

import (
	"bytes"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

// ArticleContent contains the fully scraped text of an article.
type ArticleContent struct {
	ArticleSummary
	FullText string
}

// ScrapeArticle attempts to fetch the raw HTML of an article and extract its content.
func ScrapeArticle(summary ArticleSummary) (*ArticleContent, error) {
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	// Resolve Google News redirection to get the real article URL
	finalURL, err := ResolveGoogleNewsURL(summary.Link)
	if err != nil {
		finalURL = summary.Link
	}

	req, err := http.NewRequest("GET", finalURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	// NYT NYT Paywall bypass via cookies
	if summary.SourceName == "New York Times" {
		nytS := os.Getenv("NYT_S_COOKIE")
		nytJK := os.Getenv("NYT_JK_COOKIE")
		if nytS != "" {
			req.AddCookie(&http.Cookie{Name: "nyt-s", Value: nytS})
		}
		if nytJK != "" {
			req.AddCookie(&http.Cookie{Name: "nyt-jk", Value: nytJK})
		}
	}

	// Use a standard browser User-Agent so we don't get blocked immediately by CDNs
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/114.0.0.0 Safari/537.36")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bad status code: %d from %s", resp.StatusCode, summary.Link)
	}

	// We no longer read to a string and use simpleExtract, we pass the body to goquery
	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to parse HTML: %w", err)
	}

	extractedText := extractArticleText(doc)

	return &ArticleContent{
		ArticleSummary: summary,
		FullText:       extractedText,
	}, nil
}

// extractArticleText uses goquery to isolate the actual content of the reading material
// while avoiding sidebars, footers, scripts, and navbars.
func extractArticleText(doc *goquery.Document) string {
	// Remove common noisy tags before extracting text
	doc.Find("script, style, nav, footer, header, aside, .ad, .advertisement, .related").Remove()

	var contentBuilder bytes.Buffer

	// Target tags that usually hold the reading material
	doc.Find("p, h1, h2, h3, h4, li, blockquote, article, .article-body, .article-content").Each(func(i int, s *goquery.Selection) {
		text := strings.TrimSpace(s.Text())
		// Filter out strings that are likely noise or short nav/meta links
		if len(text) > 30 && !strings.HasPrefix(text, "http") {
			contentBuilder.WriteString(text)
			contentBuilder.WriteString("\n\n")
		}
	})

	extracted := strings.TrimSpace(contentBuilder.String())
	if extracted == "" {
		// Fallback: If no structured p/h tags worked, just grab the body text
		extracted = strings.TrimSpace(doc.Find("body").Text())
	}
	return extracted
}
