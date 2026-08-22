package generator

import (
	"bufio"
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"youtube-downloader/internal/engine"
	"youtube-downloader/internal/logger"
)

//go:embed portal_template.html
var portalTemplateHTML string

var portalMutex sync.Mutex

type PortalTranscriptCue struct {
	StartSec int    `json:"s"`
	Text     string `json:"t"`
}

type PortalCatalogItem struct {
	VideoID              string                `json:"video_id"`
	Title                string                `json:"title"`
	Channel              string                `json:"channel"`
	ChannelURL           string                `json:"channel_url"`
	Duration             int64                 `json:"duration"`
	FormattedDuration    string                `json:"formatted_duration"`
	ViewCount            int64                 `json:"view_count"`
	FormattedViews       string                `json:"formatted_views"`
	UploadDate           string                `json:"upload_date"`
	Description          string                `json:"description"`
	Tags                 []string              `json:"tags"`
	TagsString           string                `json:"tags_string"`
	RelativePlayerURL    string                `json:"relative_player_url"`
	RelativeThumbnailURL string                `json:"relative_thumbnail_url"`
	RelativeAvatarURL    string                `json:"relative_avatar_url"`
	RelativeChannelURL   string                `json:"relative_channel_url"`
	DownloadedAt         string                `json:"downloaded_at"`
	Transcripts          []PortalTranscriptCue `json:"transcripts,omitempty"`
}

type PortalChannelItem struct {
	ChannelID            string `json:"channel_id"`
	Title                string `json:"title"`
	Handle               string `json:"handle"`
	FormattedSubscribers string `json:"formatted_subscribers"`
	RelativeAvatarURL    string `json:"relative_avatar_url"`
	RelativeBannerURL    string `json:"relative_banner_url"`
	RelativeChannelURL   string `json:"relative_channel_url"`
	VideoCount           int    `json:"video_count"`
}

type PortalManifest struct {
	GeneratedAt string              `json:"generated_at"`
	Videos      []PortalCatalogItem `json:"videos"`
	Channels    []PortalChannelItem `json:"channels"`
}

type portalTemplateData struct {
	GeneratedAt string
	Videos      []PortalCatalogItem
	Channels    []PortalChannelItem
	TopTags     []string
	CatalogJSON template.JS
}

// RegisterVideoInMasterPortal updates the root catalog.json and regenerates downloads/index.html
func RegisterVideoInMasterPortal(baseDownloadsDir string, item *PortalCatalogItem, chanMeta *engine.ChannelMetadata) error {
	portalMutex.Lock()
	defer portalMutex.Unlock()

	if baseDownloadsDir == "" {
		return fmt.Errorf("base downloads dir cannot be empty")
	}

	catalogPath := filepath.Join(baseDownloadsDir, "catalog.json")
	var manifest PortalManifest

	if data, err := os.ReadFile(catalogPath); err == nil {
		_ = json.Unmarshal(data, &manifest)
	}

	if item.DownloadedAt == "" {
		item.DownloadedAt = time.Now().Format("2006-01-02 15:04:05")
	}
	if item.FormattedDuration == "" && item.Duration > 0 {
		item.FormattedDuration = formatDuration(item.Duration)
	}
	if item.FormattedViews == "" && item.ViewCount > 0 {
		item.FormattedViews = formatNumber(item.ViewCount)
	}
	if item.UploadDate != "" {
		item.UploadDate = formatUploadDate(item.UploadDate)
	}
	if len(item.Tags) > 0 {
		item.TagsString = strings.Join(item.Tags, " ")
	}

	// Extract transcript cues if not present and player path exists
	if len(item.Transcripts) == 0 && item.RelativePlayerURL != "" {
		playerFull := filepath.Join(baseDownloadsDir, item.RelativePlayerURL)
		videoDir := filepath.Dir(playerFull)
		item.Transcripts = extractTranscriptCues(videoDir)
	}

	// Update or append video in manifest
	found := false
	for i, v := range manifest.Videos {
		if v.VideoID == item.VideoID {
			if len(item.Transcripts) == 0 && len(v.Transcripts) > 0 {
				item.Transcripts = v.Transcripts
			}
			manifest.Videos[i] = *item
			found = true
			break
		}
	}
	if !found {
		manifest.Videos = append([]PortalCatalogItem{*item}, manifest.Videos...)
	}

	// Rebuild and aggregate channel list
	channelMap := make(map[string]*PortalChannelItem)
	for _, v := range manifest.Videos {
		chName := strings.TrimSpace(v.Channel)
		if chName == "" {
			chName = "Unknown Channel"
		}
		chItem, exists := channelMap[chName]
		if !exists {
			chItem = &PortalChannelItem{
				Title:              chName,
				RelativeAvatarURL:  v.RelativeAvatarURL,
				RelativeChannelURL: v.RelativeChannelURL,
				VideoCount:         0,
			}
			channelMap[chName] = chItem
		}
		chItem.VideoCount++
		if v.RelativeAvatarURL != "" && chItem.RelativeAvatarURL == "" {
			chItem.RelativeAvatarURL = v.RelativeAvatarURL
		}
		if v.RelativeChannelURL != "" && chItem.RelativeChannelURL == "" {
			chItem.RelativeChannelURL = v.RelativeChannelURL
		}
	}

	if chanMeta != nil && chanMeta.Title != "" {
		if chItem, ok := channelMap[chanMeta.Title]; ok {
			chItem.Handle = chanMeta.Handle
			chItem.FormattedSubscribers = chanMeta.FormattedSubscribers
			if chanMeta.ChannelID != "" {
				chItem.ChannelID = chanMeta.ChannelID
			}
		}
	}

	manifest.Channels = make([]PortalChannelItem, 0, len(channelMap))
	for _, ch := range channelMap {
		manifest.Channels = append(manifest.Channels, *ch)
	}
	sort.Slice(manifest.Channels, func(i, j int) bool {
		return manifest.Channels[i].VideoCount > manifest.Channels[j].VideoCount
	})

	manifest.GeneratedAt = time.Now().Format("2006-01-02 15:04:05")

	// Save catalog.json
	manifestBytes, err := json.MarshalIndent(manifest, "", "  ")
	if err == nil {
		_ = os.WriteFile(catalogPath, manifestBytes, 0644)
	}

	// Aggregate Top Tags
	tagCounts := make(map[string]int)
	for _, v := range manifest.Videos {
		for _, t := range v.Tags {
			t = strings.TrimSpace(t)
			if t != "" && len(t) <= 24 {
				tagCounts[t]++
			}
		}
	}
	type tagFreq struct {
		tag   string
		count int
	}
	var tagList []tagFreq
	for t, c := range tagCounts {
		tagList = append(tagList, tagFreq{tag: t, count: c})
	}
	sort.Slice(tagList, func(i, j int) bool {
		return tagList[i].count > tagList[j].count
	})
	var topTags []string
	for i := 0; i < len(tagList) && i < 15; i++ {
		topTags = append(topTags, tagList[i].tag)
	}

	catJSONBytes, _ := json.Marshal(manifest)

	data := portalTemplateData{
		GeneratedAt: manifest.GeneratedAt,
		Videos:      manifest.Videos,
		Channels:    manifest.Channels,
		TopTags:     topTags,
		CatalogJSON: template.JS(catJSONBytes),
	}

	tmpl, err := template.New("portal").Parse(portalTemplateHTML)
	if err != nil {
		return fmt.Errorf("failed to parse portal template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return fmt.Errorf("failed to execute portal template: %w", err)
	}

	indexPath := filepath.Join(baseDownloadsDir, "index.html")
	if err := os.WriteFile(indexPath, buf.Bytes(), 0644); err != nil {
		return fmt.Errorf("failed to write portal index.html: %w", err)
	}

	logger.Infof("[Portal] Updated master catalog with %d video(s) and %d channel(s)", len(manifest.Videos), len(manifest.Channels))
	return nil
}

var (
	vttTimeRegex  = regexp.MustCompile(`(\d{1,2}:)?(\d{2}):(\d{2})[\.,](\d{3})\s*-->`)
	vttTagCleaner = regexp.MustCompile(`<[^>]+>`)
)

func extractTranscriptCues(videoDir string) []PortalTranscriptCue {
	entries, err := os.ReadDir(videoDir)
	if err != nil {
		return nil
	}

	var subFile string
	for _, e := range entries {
		if !e.IsDir() && (strings.HasSuffix(e.Name(), ".vtt") || strings.HasSuffix(e.Name(), ".srt")) {
			subFile = filepath.Join(videoDir, e.Name())
			break
		}
	}
	if subFile == "" {
		return nil
	}

	file, err := os.Open(subFile)
	if err != nil {
		return nil
	}
	defer file.Close()

	var cues []PortalTranscriptCue
	scanner := bufio.NewScanner(file)
	var currentSec int = -1
	var currentText strings.Builder

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "WEBVTT") || strings.HasPrefix(line, "NOTE") {
			if currentSec >= 0 && currentText.Len() > 0 {
				clean := strings.TrimSpace(vttTagCleaner.ReplaceAllString(currentText.String(), ""))
				if clean != "" {
					cues = append(cues, PortalTranscriptCue{
						StartSec: currentSec,
						Text:     clean,
					})
				}
				currentSec = -1
				currentText.Reset()
			}
			continue
		}

		if m := vttTimeRegex.FindStringSubmatch(line); len(m) >= 4 {
			if currentSec >= 0 && currentText.Len() > 0 {
				clean := strings.TrimSpace(vttTagCleaner.ReplaceAllString(currentText.String(), ""))
				if clean != "" {
					cues = append(cues, PortalTranscriptCue{
						StartSec: currentSec,
						Text:     clean,
					})
				}
				currentText.Reset()
			}
			h := 0
			if m[1] != "" {
				h, _ = strconv.Atoi(strings.TrimSuffix(m[1], ":"))
			}
			min, _ := strconv.Atoi(m[2])
			sec, _ := strconv.Atoi(m[3])
			currentSec = h*3600 + min*60 + sec
			continue
		}

		if currentSec >= 0 && !strings.Contains(line, "-->") {
			if currentText.Len() > 0 {
				currentText.WriteString(" ")
			}
			currentText.WriteString(line)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil
	}

	if currentSec >= 0 && currentText.Len() > 0 {
		clean := strings.TrimSpace(vttTagCleaner.ReplaceAllString(currentText.String(), ""))
		if clean != "" {
			cues = append(cues, PortalTranscriptCue{
				StartSec: currentSec,
				Text:     clean,
			})
		}
	}

	return cues
}
