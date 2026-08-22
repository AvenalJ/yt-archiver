package db

import (
	"time"
)

type UserPreferences struct {
	// Format & Quality
	DownloadVideo     bool   `json:"download_video"`
	VideoFormat       string `json:"video_format"`  // "mp4", "mkv", "webm", "best"
	VideoQuality      string `json:"video_quality"` // "best", "2160p", "1440p", "1080p", "720p", "480p", "360p"
	DownloadAudioOnly bool   `json:"download_audio_only"`
	AudioFormat       string `json:"audio_format"`  // "mp3", "m4a", "flac", "wav", "opus", "aac", "best"
	AudioQuality      string `json:"audio_quality"` // "320k", "256k", "192k", "128k", "best"

	// Metadata & Attachments
	DownloadThumbnail bool   `json:"download_thumbnail"`
	DownloadMetadata  bool   `json:"download_metadata"`
	DownloadSubtitles bool   `json:"download_subtitles"`
	SubtitleLangs     string `json:"subtitle_langs"` // "en,auto,all" or comma separated
	EmbedSubtitles    bool   `json:"embed_subtitles"`
	FetchDislikes     bool   `json:"fetch_dislikes"` // Return YouTube Dislike (RYD) API integration

	// Comments & Avatars
	DownloadComments         bool `json:"download_comments"`
	CommentLimit             int  `json:"comment_limit"` // e.g. 100, 500, 1000, or -1 for All
	DownloadCommenterAvatars bool `json:"download_commenter_avatars"`

	// Output & HTML Report
	GenerateHTMLReport     bool   `json:"generate_html_report"`
	DownloadFolder         string `json:"download_folder"`
	DuplicateAction        string `json:"duplicate_action"` // "skip", "overwrite", "rename"
	MaxConcurrentDownloads int    `json:"max_concurrent_downloads"`
	SpeedLimit             string `json:"speed_limit"` // e.g. "10M", "" for unlimited

	// Cookies & Authentication (Feature 1)
	CookieSource   string `json:"cookie_source"`  // "none", "browser", "file"
	CookieBrowser  string `json:"cookie_browser"` // "chrome", "firefox", "edge", "brave", "opera", "chromium"
	CookieFilePath string `json:"cookie_file_path"`

	// SponsorBlock (Feature 3)
	SponsorBlockAction     string `json:"sponsorblock_action"`     // "mark" (in offline player), "remove" (ffmpeg trim), "none"
	SponsorBlockCategories string `json:"sponsorblock_categories"` // comma separated e.g. "sponsor,selfpromo,interaction,intro,outro"

	// Media Tagging & Embedding (Feature 8)
	EmbedMetadata bool `json:"embed_metadata"` // ID3 / MP4 tags
	EmbedCoverArt bool `json:"embed_cover_art"`
	EmbedChapters bool `json:"embed_chapters"`

	// Companion Audio & NFO Archiving
	ExtractCompanionAudio bool   `json:"extract_companion_audio"` // Extract MP3/M4A alongside video
	CompanionAudioFormat  string `json:"companion_audio_format"`  // "mp3", "m4a", "opus", "flac"
	GenerateNFO           bool   `json:"generate_nfo"`            // Kodi/Jellyfin/Plex .nfo XML metadata

	// Channel Monitoring (Feature 6 & 2)
	AutoSyncChannels    bool   `json:"auto_sync_channels"`
	SyncIntervalMinutes int    `json:"sync_interval_minutes"`
	SyncWindowEnabled   bool   `json:"sync_window_enabled"`
	SyncWindowStart     string `json:"sync_window_start"` // local HH:MM
	SyncWindowEnd       string `json:"sync_window_end"`   // local HH:MM

	// Auto-Retry Failed Downloads & Circuit Breaker
	AutoRetryFailed          bool `json:"auto_retry_failed"`
	AutoRetryIntervalMinutes int  `json:"auto_retry_interval_minutes"`
	AutoRetryMaxAttempts     int  `json:"auto_retry_max_attempts"`
	CircuitBreakerEnabled    bool `json:"circuit_breaker_enabled"` // Rate limit (429) protection toggle

	// Quiet hours apply to new work only; active downloads are allowed to finish.
	DownloadWindowEnabled bool   `json:"download_window_enabled"`
	DownloadWindowStart   string `json:"download_window_start"`
	DownloadWindowEnd     string `json:"download_window_end"`

	// UI & Aesthetics
	UIMode                string   `json:"ui_mode"`                 // "standard", "compact"
	Theme                 string   `json:"theme"`                   // "midnight", "liquid-glass", "aurora", "paper"
	ColorScheme           string   `json:"color_scheme"`            // "crimson", "ocean", "violet", "lime", "sunset"
	DismissedFeatureCards []string `json:"dismissed_feature_cards"` // dismissed card element IDs
}

func DefaultPreferences(defaultDownloadDir string) *UserPreferences {
	return &UserPreferences{
		DismissedFeatureCards:    []string{},
		DownloadVideo:            true,
		VideoFormat:              "mp4",
		VideoQuality:             "1080p",
		DownloadAudioOnly:        false,
		AudioFormat:              "mp3",
		AudioQuality:             "320k",
		DownloadThumbnail:        true,
		DownloadMetadata:         true,
		DownloadSubtitles:        true,
		SubtitleLangs:            "en,auto",
		EmbedSubtitles:           false,
		FetchDislikes:            true,
		DownloadComments:         true,
		CommentLimit:             1000,
		DownloadCommenterAvatars: true,
		GenerateHTMLReport:       true,
		DownloadFolder:           defaultDownloadDir,
		DuplicateAction:          "skip",
		MaxConcurrentDownloads:   2,
		SpeedLimit:               "",
		CookieSource:             "none",
		CookieBrowser:            "none",
		CookieFilePath:           "",
		SponsorBlockAction:       "mark",
		SponsorBlockCategories:   "sponsor,selfpromo,intro,outro",
		EmbedMetadata:            true,
		EmbedCoverArt:            true,
		EmbedChapters:            true,
		ExtractCompanionAudio:    true,
		CompanionAudioFormat:     "mp3",
		GenerateNFO:              true,
		AutoSyncChannels:         false,
		SyncIntervalMinutes:      60,
		SyncWindowEnabled:        false,
		SyncWindowStart:          "01:00",
		SyncWindowEnd:            "06:00",
		AutoRetryFailed:          false,
		AutoRetryIntervalMinutes: 15,
		AutoRetryMaxAttempts:     3,
		CircuitBreakerEnabled:    true,
		DownloadWindowEnabled:    false,
		DownloadWindowStart:      "01:00",
		DownloadWindowEnd:        "06:00",
		UIMode:                   "standard",
		Theme:                    "midnight",
		ColorScheme:              "crimson",
	}
}

// DownloadProfile is a reusable set of download preferences. Empty values inherit globals.
type DownloadProfile struct {
	ID          string           `json:"id"`
	Name        string           `json:"name"`
	Description string           `json:"description,omitempty"`
	Preferences *UserPreferences `json:"preferences"`
}

type DownloadItem struct {
	ID               string     `json:"id"`
	URL              string     `json:"url"`
	VideoID          string     `json:"video_id"`
	PlaylistID       string     `json:"playlist_id,omitempty"`
	PlaylistTitle    string     `json:"playlist_title,omitempty"`
	PlaylistIndex    int        `json:"playlist_index,omitempty"`
	PlaylistTotal    int        `json:"playlist_total,omitempty"`
	Title            string     `json:"title"`
	Channel          string     `json:"channel"`
	ChannelURL       string     `json:"channel_url"`
	Duration         int64      `json:"duration"` // Duration in seconds
	ThumbnailURL     string     `json:"thumbnail_url"`
	ThumbnailPath    string     `json:"thumbnail_path"`
	Format           string     `json:"format"`
	Quality          string     `json:"quality"`
	IsAudioOnly      bool       `json:"is_audio_only"`
	Status           string     `json:"status"` // "queued", "downloading", "paused", "completed", "failed", "cancelled"
	Progress         float64    `json:"progress"`
	Speed            string     `json:"speed"`
	ETA              string     `json:"eta"`
	TotalBytes       int64      `json:"total_bytes"`
	DownloadedBytes  int64      `json:"downloaded_bytes"`
	CurrentStep      string     `json:"current_step"` // e.g. "Fetching info", "Downloading media", "Downloading comments", "Generating HTML"
	OutputDir        string     `json:"output_dir"`
	MediaFilePath    string     `json:"media_file_path"`
	HTMLFilePath     string     `json:"html_file_path"`
	MetadataFilePath string     `json:"metadata_file_path"`
	ErrorMessage     string     `json:"error_message,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	CompletedAt      *time.Time `json:"completed_at,omitempty"`
	CommentsCount    int        `json:"comments_count"`
	CustomOptions    string     `json:"custom_options,omitempty"`   // JSON-encoded overridden options if any
	CompletedStages  string     `json:"completed_stages,omitempty"` // Comma-separated completed pipeline stages: "media","comments","avatars","html"
	Category         string     `json:"category,omitempty"`         // "Videos", "Shorts", "Live Streams"
}

type VideoMetadata struct {
	ID               string           `json:"id"`
	Title            string           `json:"title"`
	Description      string           `json:"description"`
	Duration         int64            `json:"duration"`
	ViewCount        int64            `json:"view_count"`
	LikeCount        int64            `json:"like_count"`
	DislikeCount     int64            `json:"dislike_count"`
	Rating           float64          `json:"rating"`
	UploadDate       string           `json:"upload_date"` // YYYYMMDD or formatted
	Channel          string           `json:"channel"`
	ChannelID        string           `json:"channel_id"`
	ChannelURL       string           `json:"channel_url"`
	ChannelAvatar    string           `json:"channel_avatar"`
	SubscriberCount  int64            `json:"subscriber_count"`
	Thumbnail        string           `json:"thumbnail"`
	Categories       []string         `json:"categories"`
	Tags             []string         `json:"tags"`
	WebpageURL       string           `json:"webpage_url"`
	AvailableFormats []FormatSpec     `json:"formats,omitempty"`
	Chapters         []VideoChapter   `json:"chapters,omitempty"`
	SponsorSegments  []SponsorSegment `json:"sponsor_segments,omitempty"`
}

type VideoChapter struct {
	Title          string  `json:"title"`
	StartTime      float64 `json:"start_time"` // in seconds
	EndTime        float64 `json:"end_time"`   // in seconds
	FormattedStart string  `json:"formatted_start,omitempty"`
}

type SponsorSegment struct {
	Category  string  `json:"category"`   // e.g. "sponsor", "selfpromo", "interaction", "intro", "outro"
	Action    string  `json:"action"`     // "skip", "mute", "full"
	StartTime float64 `json:"start_time"` // in seconds
	EndTime   float64 `json:"end_time"`   // in seconds
	UUID      string  `json:"uuid,omitempty"`
}

type ChannelSubscription struct {
	ID                 string     `json:"id"`
	ChannelID          string     `json:"channel_id"`
	Title              string     `json:"title"`
	Handle             string     `json:"handle"`
	URL                string     `json:"url"`
	AvatarURL          string     `json:"avatar_url"`
	SubscriberCount    int64      `json:"subscriber_count"`
	LastSynced         *time.Time `json:"last_synced,omitempty"`
	AutoDownload       bool       `json:"auto_download"`
	TotalVideos        int        `json:"total_videos"`
	MinDurationSec     int64      `json:"min_duration_sec"`    // minimum duration in seconds (0 = any)
	ExcludeShorts      bool       `json:"exclude_shorts"`      // skip <= 60s
	ExcludeLiveStreams bool       `json:"exclude_livestreams"` // skip live streams
	MaxAutoSyncCount   int        `json:"max_auto_sync_count"` // max items per auto-sync (0 = all)
	CreatedAt          time.Time  `json:"created_at"`
}

type FormatSpec struct {
	FormatID   string  `json:"format_id"`
	Extension  string  `json:"ext"`
	Resolution string  `json:"resolution"`
	Note       string  `json:"note"`
	Filesize   int64   `json:"filesize"`
	VCodec     string  `json:"vcodec"`
	ACodec     string  `json:"acodec"`
	FPS        float64 `json:"fps"`
}

type CommentItem struct {
	ID                string         `json:"id"`
	VideoID           string         `json:"video_id"`
	ParentID          string         `json:"parent_id,omitempty"` // empty if top-level comment
	Author            string         `json:"author"`
	AuthorID          string         `json:"author_id"`
	AuthorURL         string         `json:"author_url"`
	AuthorThumbnail   string         `json:"author_thumbnail"`
	AuthorAvatarLocal string         `json:"author_avatar_local"`
	Text              string         `json:"text"`
	LikeCount         int            `json:"like_count"`
	Timestamp         int64          `json:"timestamp"`
	TimeText          string         `json:"time_text"`
	IsFavorited       bool           `json:"is_favorited"` // hearted by channel owner
	IsCreator         bool           `json:"is_creator"`   // posted by channel owner
	IsVerified        bool           `json:"is_verified"`
	RepliesCount      int            `json:"replies_count"`
	Replies           []*CommentItem `json:"replies,omitempty"`
}
