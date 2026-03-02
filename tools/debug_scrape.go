package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"
)

func main() {
	// The specific AP article URL that failed
	url := "https://news.google.com/rss/articles/CBMijgFBVV95cUxQaF9xM2N2Yjh4ZlM2MndpNVdBRHMzX0QxVGV4bl83eTBabEVLcEQ5MFZzMHNTOUlSdVRSeG44dDlLeHlfYzhjcDFBZHZvTmtUVjBHdGQ0eXdoNldvRFA3aXZmZVJtYTNGS1d0cGY3OS1FaTJaMnBDT0lfYXQySExiLVVUd2F2S0oxOXh6aXZ3?oc=5"

	client := &http.Client{
		Timeout: 15 * time.Second,
		// We want to follow redirects to see the final AP URL
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.Fatalf("Failed to create request: %v", err)
	}

	// Use the same User-Agent as the main scraper
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/114.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")

	resp, err := client.Do(req)
	if err != nil {
		log.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	fmt.Printf("Initial URL: %s\n", url)
	fmt.Printf("Status Code: %d\n", resp.StatusCode)
	fmt.Printf("Final URL:   %s\n", resp.Request.URL.String())

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatalf("Failed to read body: %v", err)
	}

	err = os.WriteFile("scrape_debug.html", body, 0644)
	if err != nil {
		log.Fatalf("Failed to write file: %v", err)
	}

	fmt.Printf("Raw HTML saved to scrape_debug.html (%d bytes)\n", len(body))
}
