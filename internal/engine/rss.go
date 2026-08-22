package engine

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type RSSFeed struct {
	XMLName xml.Name   `xml:"feed"`
	Title   string     `xml:"title"`
	Author  RSSAuthor  `xml:"author"`
	Entries []RSSEntry `xml:"entry"`
}

type RSSAuthor struct {
	Name string `xml:"name"`
	URI  string `xml:"uri"`
}

type RSSEntry struct {
	ID        string    `xml:"id"`
	VideoID   string    `xml:"videoId"`
	ChannelID string    `xml:"channelId"`
	Title     string    `xml:"title"`
	Link      RSSLink   `xml:"link"`
	Published time.Time `xml:"published"`
	Updated   time.Time `xml:"updated"`
}

type RSSLink struct {
	Href string `xml:"href,attr"`
}

// FetchChannelRSS fetches and parses the public YouTube RSS XML feed for a channel ID.
func FetchChannelRSS(ctx context.Context, channelID string) (*RSSFeed, error) {
	if channelID == "" {
		return nil, fmt.Errorf("channelID is required")
	}

	feedURL := fmt.Sprintf("https://www.youtube.com/feeds/videos.xml?channel_id=%s", channelID)

	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequestWithContext(ctx, "GET", feedURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64)")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("youtube RSS feed returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var feed RSSFeed
	if err := xml.Unmarshal(body, &feed); err != nil {
		return nil, fmt.Errorf("failed to parse RSS feed XML: %w", err)
	}

	// Normalize entry links and video IDs
	for i := range feed.Entries {
		e := &feed.Entries[i]
		if e.VideoID == "" && strings.Contains(e.ID, "yt:video:") {
			e.VideoID = strings.TrimPrefix(e.ID, "yt:video:")
		}
		if e.Link.Href == "" && e.VideoID != "" {
			e.Link.Href = "https://www.youtube.com/watch?v=s" + e.VideoID
		}
	}

	return &feed, nil
}
