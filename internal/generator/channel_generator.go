package generator

import (
	"bytes"
	_ "embed"
	"fmt"
	"html/template"
	"os"
	"path/filepath"

	"youtube-downloader/internal/engine"
)

//go:embed channel_template.html
var channelTemplateHTML string

type ChannelVideoItem struct {
	Title             string `json:"title"`
	VideoID           string `json:"video_id"`
	RelativeURL       string `json:"relative_url"`
	ThumbnailFilename string `json:"thumbnail_filename"`
	Duration          int64  `json:"duration"`
	FormattedDuration string `json:"formatted_duration"`
	FormattedViews    string `json:"formatted_views"`
	UploadDate        string `json:"upload_date"`
}

type channelTemplateData struct {
	Title                string
	Handle               string
	URL                  string
	CanonicalURL         string
	AvatarFilename       string
	BannerFilename       string
	SubscriberCount      int64
	FormattedSubscribers string
	FormattedViews       string
	FormattedTotalViews  string
	TotalVideos          int
	TotalVideosText      string
	JoinedDate           string
	Country              string
	Description          string
	IsVerified           bool
	Links                []engine.ChannelLink
	Community            []engine.ChannelCommunityPost
	Playlists            []engine.ChannelPlaylistItem
	Videos               []ChannelVideoItem
}

// GenerateChannelHTML creates a rich standalone channel.html page
func GenerateChannelHTML(outputDir string, meta *engine.ChannelMetadata, videos []ChannelVideoItem) (string, error) {
	if meta == nil {
		meta = ChannelMetadataPlaceholder()
	}

	formattedViews := meta.FormattedTotalViews
	if formattedViews == "" && meta.TotalViews > 0 {
		formattedViews = formatNumber(meta.TotalViews)
	}

	avatarFilename := meta.AvatarFilename
	if avatarFilename == "" {
		if _, err := os.Stat(filepath.Join(outputDir, "channel_avatar.jpg")); err == nil {
			avatarFilename = "channel_avatar.jpg"
		}
	}

	bannerFilename := meta.BannerFilename
	if bannerFilename == "" {
		if _, err := os.Stat(filepath.Join(outputDir, "channel_banner.jpg")); err == nil {
			bannerFilename = "channel_banner.jpg"
		}
	}

	for i := range videos {
		if videos[i].FormattedDuration == "" && videos[i].Duration > 0 {
			videos[i].FormattedDuration = formatDuration(videos[i].Duration)
		}
		if videos[i].RelativeURL == "" {
			videos[i].RelativeURL = "index.html"
		}
		if videos[i].UploadDate != "" {
			videos[i].UploadDate = formatUploadDate(videos[i].UploadDate)
		}
	}

	data := channelTemplateData{
		Title:                meta.Title,
		Handle:               meta.Handle,
		URL:                  meta.URL,
		CanonicalURL:         meta.CanonicalURL,
		AvatarFilename:       avatarFilename,
		BannerFilename:       bannerFilename,
		SubscriberCount:      meta.SubscriberCount,
		FormattedSubscribers: meta.FormattedSubscribers,
		FormattedViews:       formattedViews,
		FormattedTotalViews:  formattedViews,
		TotalVideos:          meta.TotalVideos,
		TotalVideosText:      meta.TotalVideosText,
		JoinedDate:           meta.JoinedDate,
		Country:              meta.Country,
		Description:          meta.Description,
		IsVerified:           meta.IsVerified,
		Links:                meta.Links,
		Community:            meta.Community,
		Playlists:            meta.Playlists,
		Videos:               videos,
	}

	tmpl, err := template.New("channel").Parse(channelTemplateHTML)
	if err != nil {
		return "", fmt.Errorf("failed to parse channel template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute channel template: %w", err)
	}

	destPath := filepath.Join(outputDir, "channel.html")
	if err := os.WriteFile(destPath, buf.Bytes(), 0644); err != nil {
		return "", fmt.Errorf("failed to write channel.html: %w", err)
	}

	return destPath, nil
}

func ChannelMetadataPlaceholder() *engine.ChannelMetadata {
	return &engine.ChannelMetadata{
		Title:          "YouTube Channel",
		AvatarFilename: "channel_avatar.jpg",
		BannerFilename: "channel_banner.jpg",
		Links:          []engine.ChannelLink{},
	}
}
