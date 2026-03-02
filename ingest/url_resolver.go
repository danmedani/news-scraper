package ingest

import (
	"encoding/base64"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// ResolveGoogleNewsURL attempts to find the final landing page for a news.google.com link.
// Recently, Google News has moved from 302 redirects to a JS-heavy landing page.
func ResolveGoogleNewsURL(encodedUrl string) (string, error) {
	if !strings.Contains(encodedUrl, "news.google.com/rss/articles/") {
		return encodedUrl, nil
	}

	// Extract the base64-like part from the URL
	re := regexp.MustCompile(`articles/([^?]+)`)
	matches := re.FindStringSubmatch(encodedUrl)
	if len(matches) < 2 {
		return encodedUrl, nil
	}

	encodedPart := matches[1]

	// This encoded part is actually a complex protobuffer-like structure
	// but often contains the final URL in a predictable way for common articles.
	// For a more robust solution, we fetch the page and extract the data-url if it exists,
	// or fallback to decoding the base64.

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	req, err := http.NewRequest("GET", encodedUrl, nil)
	if err != nil {
		return encodedUrl, nil
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/114.0.0.0 Safari/537.36")

	resp, err := client.Do(req)
	if err != nil {
		return encodedUrl, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		bodyStr := string(body)

		// Look for common patterns in the JS/HTML that reveal the destination
		// often found in data-n-a-id or inside scripts
		reURL := regexp.MustCompile(`"(https?://[^"]+)"`)
		matches := reURL.FindAllStringSubmatch(bodyStr, -1)
		for _, m := range matches {
			u := m[1]
			if !strings.Contains(u, "google.com") && !strings.Contains(u, "gstatic.com") {
				return u, nil
			}
		}
	}

	// Fallback: Try a naive base64 decode (rarely works directly for news links anymore but good to have)
	decoded, err := base64.StdEncoding.DecodeString(encodedPart)
	if err == nil {
		// Clean up binary noise and look for http
		s := string(decoded)
		if idx := strings.Index(s, "http"); idx != -1 {
			// Find ending
			end := len(s)
			for i, char := range s[idx:] {
				if char < 32 || char > 126 {
					end = idx + i
					break
				}
			}
			return s[idx:end], nil
		}
	}

	return encodedUrl, nil
}
