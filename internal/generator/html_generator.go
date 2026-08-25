package generator

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/AvenalJ/yt-archiver/internal/db"
	"github.com/AvenalJ/yt-archiver/internal/engine"
)

//go:embed template.html
var templateHTML string

type HTMLGeneratorInput struct {
	Title                string            `json:"title"`
	VideoID              string            `json:"video_id"`
	Channel              string            `json:"channel"`
	ChannelURL           string            `json:"channel_url"`
	ChannelAvatar        string            `json:"channel_avatar"`
	ChannelAvatarFilename string           `json:"channel_avatar_filename"`
	SubscriberCount      int64             `json:"subscriber_count"`
	Duration             int64             `json:"duration"`
	ViewCount            int64             `json:"view_count"`
	LikeCount            int64             `json:"like_count"`
	DislikeCount         int64             `json:"dislike_count"`
	Rating               float64           `json:"rating,omitempty"`
	UploadDate           string            `json:"upload_date"`
	Description          string            `json:"description"`
	Categories           []string          `json:"categories"`
	Tags                 []string          `json:"tags"`
	MediaFilename        string            `json:"media_filename"`
	IsAudioOnly          bool              `json:"is_audio_only"`
	ThumbnailFilename      string
	SubtitlesFilename      string
	CompanionAudioFilename string            `json:"companion_audio_filename,omitempty"`
	CommentsCount          int               `json:"comments_count"`
	Comments               []*db.CommentItem `json:"comments"`
	AvatarMap              map[string]string `json:"avatar_map,omitempty"` // avatarHash -> Base64 Data URI
	Chapters               []db.VideoChapter `json:"chapters,omitempty"`
	SponsorSegments        []db.SponsorSegment `json:"sponsor_segments,omitempty"`
	Storyboard             *engine.StoryboardMetadata `json:"storyboard,omitempty"`
	LiveChat               []engine.LiveChatMessage `json:"live_chat,omitempty"`
	GeneratedAt            string            `json:"generated_at"`
	SourceURL              string            `json:"source_url"`
	VideoQuality           string            `json:"video_quality"`
	Filesize               int64             `json:"filesize"`
}

type templateData struct {
	Title                  string
	VideoID                string
	Channel                string
	ChannelURL             string
	ChannelAvatar          string
	ChannelAvatarFilename  string
	SubscriberCount        int64
	FormattedSubscribers   string
	Duration               int64
	FormattedDuration      string
	ViewCount              int64
	FormattedViews         string
	LikeCount              int64
	FormattedLikes         string
	DislikeCount           int64
	FormattedDislikes      string
	UploadDate             string
	Description            string
	DescriptionJSON        template.JS
	Categories             []string
	Tags                   []string
	TagsJSON               template.JS
	MediaFilename          string
	MediaMimeType          string
	IsAudioOnly            bool
	ThumbnailFilename      string
	SubtitlesFilename      string
	SubtitlesCuesJSON      template.JS
	CompanionAudioFilename string
	CommentsCount          string
	CommentsJSON           template.JS
	AvatarsJSON            template.JS
	Chapters               []db.VideoChapter
	ChaptersJSON           template.JS
	SponsorSegments        []db.SponsorSegment
	SponsorSegmentsJSON    template.JS
	Storyboard             *engine.StoryboardMetadata
	StoryboardJSON         template.JS
	StoryboardFilename     string
	LiveChat               []engine.LiveChatMessage
	LiveChatJSON           template.JS
	LiveChatCount          int
	GeneratedAt            string
	SourceURL              string
	VideoQuality           string
	Filesize               int64
	FormattedFilesize      string
}

type SubtitleCue struct {
	StartTime float64 `json:"start"`
	EndTime   float64 `json:"end"`
	Text      string  `json:"text"`
}

// GenerateOfflineHTML produces a self-contained, interactive YouTube player HTML file
func GenerateOfflineHTML(outputDir string, input *HTMLGeneratorInput) (string, error) {
	if input.GeneratedAt == "" {
		input.GeneratedAt = time.Now().Format("2006-01-02 15:04:05")
	}

	commentsJSON, _ := json.Marshal(input.Comments)
	if string(commentsJSON) == "" || string(commentsJSON) == "null" {
		commentsJSON = []byte("[]")
	}

	tagsJSON, _ := json.Marshal(input.Tags)
	if string(tagsJSON) == "" || string(tagsJSON) == "null" {
		tagsJSON = []byte("[]")
	}

	descJSON, _ := json.Marshal(input.Description)

	avatarsJSON, _ := json.Marshal(input.AvatarMap)
	if string(avatarsJSON) == "" || string(avatarsJSON) == "null" {
		avatarsJSON = []byte("{}")
	}

	for i := range input.Chapters {
		if input.Chapters[i].FormattedStart == "" {
			input.Chapters[i].FormattedStart = formatDuration(int64(input.Chapters[i].StartTime))
		}
	}
	chaptersJSON, _ := json.Marshal(input.Chapters)
	if string(chaptersJSON) == "" || string(chaptersJSON) == "null" {
		chaptersJSON = []byte("[]")
	}

	sponsorJSON, _ := json.Marshal(input.SponsorSegments)
	if string(sponsorJSON) == "" || string(sponsorJSON) == "null" {
		sponsorJSON = []byte("[]")
	}

	storyboardJSON, _ := json.Marshal(input.Storyboard)
	if string(storyboardJSON) == "" || string(storyboardJSON) == "null" {
		storyboardJSON = []byte("null")
	}
	storyboardFilename := ""
	if input.Storyboard != nil && input.Storyboard.StoryboardFilename != "" {
		storyboardFilename = input.Storyboard.StoryboardFilename
	}

	liveChatJSON, _ := json.Marshal(input.LiveChat)
	if string(liveChatJSON) == "" || string(liveChatJSON) == "null" {
		liveChatJSON = []byte("[]")
	}

	var subCues []SubtitleCue
	if input.SubtitlesFilename != "" {
		subPath := filepath.Join(outputDir, input.SubtitlesFilename)
		subCues = parseSubtitleFile(subPath)
	}
	subtitlesJSON, _ := json.Marshal(subCues)
	if string(subtitlesJSON) == "" || string(subtitlesJSON) == "null" {
		subtitlesJSON = []byte("[]")
	}

	formattedDislikes := ""
	if input.DislikeCount > 0 {
		formattedDislikes = formatNumber(input.DislikeCount)
	}

	formattedUploadDate := formatUploadDate(input.UploadDate)

	companionAudio := input.CompanionAudioFilename
	if companionAudio == "" && !input.IsAudioOnly && input.MediaFilename != "" {
		baseName := strings.TrimSuffix(input.MediaFilename, filepath.Ext(input.MediaFilename))
		for _, ext := range []string{".mp3", ".m4a", ".opus", ".flac", ".wav"} {
			cand := baseName + ext
			if _, err := os.Stat(filepath.Join(outputDir, cand)); err == nil {
				companionAudio = cand
				break
			}
		}
	}

	sourceURL := input.SourceURL
	if sourceURL == "" && input.VideoID != "" {
		sourceURL = "https://www.youtube.com/watch?v=" + input.VideoID
	}

	data := templateData{
		Title:                  input.Title,
		VideoID:                input.VideoID,
		Channel:                input.Channel,
		ChannelURL:             input.ChannelURL,
		ChannelAvatar:          input.ChannelAvatar,
		ChannelAvatarFilename:  input.ChannelAvatarFilename,
		SubscriberCount:        input.SubscriberCount,
		FormattedSubscribers:   formatSubscribers(input.SubscriberCount),
		Duration:               input.Duration,
		FormattedDuration:      formatDuration(input.Duration),
		ViewCount:              input.ViewCount,
		FormattedViews:         formatNumber(input.ViewCount),
		LikeCount:              input.LikeCount,
		FormattedLikes:         formatNumber(input.LikeCount),
		DislikeCount:           input.DislikeCount,
		FormattedDislikes:      formattedDislikes,
		UploadDate:             formattedUploadDate,
		Description:            input.Description,
		DescriptionJSON:        template.JS(descJSON),
		Categories:             input.Categories,
		Tags:                   input.Tags,
		TagsJSON:               template.JS(tagsJSON),
		MediaFilename:          input.MediaFilename,
		MediaMimeType:          getMimeType(input.MediaFilename),
		IsAudioOnly:            input.IsAudioOnly,
		ThumbnailFilename:      input.ThumbnailFilename,
		SubtitlesFilename:      input.SubtitlesFilename,
		SubtitlesCuesJSON:      template.JS(subtitlesJSON),
		CompanionAudioFilename: companionAudio,
		CommentsCount:          formatNumber(int64(input.CommentsCount)),
		CommentsJSON:           template.JS(commentsJSON),
		AvatarsJSON:            template.JS(avatarsJSON),
		Chapters:               input.Chapters,
		ChaptersJSON:           template.JS(chaptersJSON),
		SponsorSegments:        input.SponsorSegments,
		SponsorSegmentsJSON:    template.JS(sponsorJSON),
		Storyboard:             input.Storyboard,
		StoryboardJSON:         template.JS(storyboardJSON),
		StoryboardFilename:     storyboardFilename,
		LiveChat:               input.LiveChat,
		LiveChatJSON:           template.JS(liveChatJSON),
		LiveChatCount:          len(input.LiveChat),
		GeneratedAt:            input.GeneratedAt,
		SourceURL:              sourceURL,
		VideoQuality:           input.VideoQuality,
		Filesize:               input.Filesize,
		FormattedFilesize:      formatBytes(input.Filesize),
	}

	tmpl, err := template.New("player").Parse(templateHTML)
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}

	destPath := filepath.Join(outputDir, "index.html")
	if err := os.WriteFile(destPath, buf.Bytes(), 0644); err != nil {
		return "", fmt.Errorf("failed to write index.html: %w", err)
	}

	return destPath, nil
}

func formatSubscribers(n int64) string {
	if n <= 0 {
		return ""
	}
	return formatNumber(n) + " subscribers"
}

func formatNumber(n int64) string {
	if n >= 1000000000 {
		val := float64(n) / 1000000000.0
		return formatFloatWithSuffix(val, "B")
	}
	if n >= 1000000 {
		val := float64(n) / 1000000.0
		return formatFloatWithSuffix(val, "M")
	}
	if n >= 1000 {
		val := float64(n) / 1000.0
		return formatFloatWithSuffix(val, "K")
	}
	return fmt.Sprintf("%d", n)
}

func formatFloatWithSuffix(val float64, suffix string) string {
	if val >= 100 {
		return fmt.Sprintf("%.0f%s", val, suffix)
	}
	s := fmt.Sprintf("%.2f", val)
	s = strings.TrimRight(s, "0")
	s = strings.TrimRight(s, ".")
	return s + suffix
}

func formatDuration(sec int64) string {
	if sec <= 0 {
		return "0:00"
	}
	h := sec / 3600
	m := (sec % 3600) / 60
	s := sec % 60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}

func formatBytes(bytes int64) string {
	if bytes >= 1024*1024*1024 {
		return fmt.Sprintf("%.2f GB", float64(bytes)/(1024*1024*1024))
	}
	if bytes >= 1024*1024 {
		return fmt.Sprintf("%.1f MB", float64(bytes)/(1024*1024))
	}
	if bytes >= 1024 {
		return fmt.Sprintf("%.1f KB", float64(bytes)/1024)
	}
	return fmt.Sprintf("%d B", bytes)
}

func getMimeType(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".mp4":
		return "video/mp4"
	case ".mkv":
		return "video/x-matroska"
	case ".webm":
		return "video/webm"
	case ".mp3":
		return "audio/mpeg"
	case ".m4a":
		return "audio/mp4"
	case ".flac":
		return "audio/flac"
	case ".wav":
		return "audio/wav"
	case ".opus", ".ogg":
		return "audio/ogg"
	default:
		return "video/mp4"
	}
}

func formatUploadDate(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if len(raw) == 8 {
		if t, err := time.Parse("20060102", raw); err == nil {
			return t.Format("2 Jan 2006")
		}
	}
	if strings.Contains(raw, "-") && len(raw) == 10 {
		if t, err := time.Parse("2006-01-02", raw); err == nil {
			return t.Format("2 Jan 2006")
		}
	}
	return raw
}

var (
	cueTimeRegex  = regexp.MustCompile(`(?:(\d{1,2}):)?(\d{2}):(\d{2})[\.,](\d{1,3})\s*-->\s*(?:(\d{1,2}):)?(\d{2}):(\d{2})[\.,](\d{1,3})`)
	cueTagCleaner = regexp.MustCompile(`<[^>]+>`)
)

func parseSubtitleFile(filePath string) []SubtitleCue {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil
	}

	lines := strings.Split(string(data), "\n")
	var cues []SubtitleCue

	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if m := cueTimeRegex.FindStringSubmatch(line); len(m) >= 9 {
			sh, _ := strconv.Atoi(m[1])
			sm, _ := strconv.Atoi(m[2])
			ss, _ := strconv.Atoi(m[3])
			smsStr := m[4]
			if len(smsStr) == 1 {
				smsStr += "00"
			} else if len(smsStr) == 2 {
				smsStr += "0"
			} else if len(smsStr) > 3 {
				smsStr = smsStr[:3]
			}
			sms, _ := strconv.Atoi(smsStr)
			startSec := float64(sh*3600+sm*60+ss) + float64(sms)/1000.0

			eh, _ := strconv.Atoi(m[5])
			em, _ := strconv.Atoi(m[6])
			es, _ := strconv.Atoi(m[7])
			emsStr := m[8]
			if len(emsStr) == 1 {
				emsStr += "00"
			} else if len(emsStr) == 2 {
				emsStr += "0"
			} else if len(emsStr) > 3 {
				emsStr = emsStr[:3]
			}
			ems, _ := strconv.Atoi(emsStr)
			endSec := float64(eh*3600+em*60+es) + float64(ems)/1000.0

			var textLines []string
			for j := i + 1; j < len(lines); j++ {
				nextL := strings.TrimSpace(lines[j])
				if nextL == "" || cueTimeRegex.MatchString(nextL) {
					break
				}
				clean := strings.TrimSpace(cueTagCleaner.ReplaceAllString(nextL, ""))
				if clean != "" {
					textLines = append(textLines, clean)
				}
			}

			cueText := strings.Join(textLines, " ")
			if cueText != "" {
				cues = append(cues, SubtitleCue{
					StartTime: startSec,
					EndTime:   endSec,
					Text:      cueText,
				})
			}
		}
	}
	return cues
}
