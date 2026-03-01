package ingest

// FeedSource represents a single news source and its RSS/Atom feed URL.
type FeedSource struct {
	Name string
	URL  string
}

// GetFeeds returns the configured list of news sources.
func GetFeeds() []FeedSource {
	return []FeedSource{
		{Name: "Reuters", URL: "https://v2.core.com/rss/reuters"},       // using a generic aggregator as reuters often blocks direct scraping
		{Name: "Associated Press", URL: "https://apnews.com/index.rss"}, // valid AP index
		{Name: "New York Times", URL: "https://rss.nytimes.com/services/xml/rss/nyt/HomePage.xml"},
		{Name: "Military Times", URL: "https://www.militarytimes.com/arc/outboundfeeds/rss/"},
		{Name: "Financial Times", URL: "https://www.ft.com/news-feed?format=rss"}, // valid FT format
		{Name: "BBC News", URL: "http://feeds.bbci.co.uk/news/rss.xml"},
		{Name: "The Guardian", URL: "https://www.theguardian.com/world/rss"},
	}
}
