package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"youtube-downloader/internal/db"
	"youtube-downloader/internal/engine"
	"youtube-downloader/internal/generator"
)

func main() {
	// Initialize database
	dbPath := filepath.Join("data", "yt_downloader.db")
	database, err := db.InitDB(dbPath, "downloads")
	if err != nil {
		fmt.Printf("Warning: failed to open DB: %v\n", err)
	}

	downloadsDir := "downloads"
	entries, err := os.ReadDir(downloadsDir)
	if err != nil {
		fmt.Printf("Error reading downloads dir: %v\n", err)
		return
	}

	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == "Playlists" {
			continue
		}
		processFolder(database, filepath.Join(downloadsDir, entry.Name()), downloadsDir)
	}

	// Also check Playlists folder
	pEntries, err := os.ReadDir(filepath.Join(downloadsDir, "Playlists"))
	if err == nil {
		for _, pe := range pEntries {
			if !pe.IsDir() {
				continue
			}
			pSub, err := os.ReadDir(filepath.Join(downloadsDir, "Playlists", pe.Name()))
			if err == nil {
				for _, sub := range pSub {
					if sub.IsDir() {
						processFolder(database, filepath.Join(downloadsDir, "Playlists", pe.Name(), sub.Name()), downloadsDir)
					}
				}
			}
		}
	}
}

func processFolder(database *db.DB, dir string, baseDownloadsDir string) {
	files, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	var infoPath, mediaFile, thumbFile, subFile, audioFile string
	for _, f := range files {
		name := f.Name()
		if strings.HasSuffix(name, ".info.json") {
			infoPath = filepath.Join(dir, name)
		} else if strings.HasSuffix(name, ".mp4") || strings.HasSuffix(name, ".mkv") || strings.HasSuffix(name, ".webm") {
			mediaFile = name
		} else if strings.HasSuffix(name, ".mp3") || strings.HasSuffix(name, ".m4a") || strings.HasSuffix(name, ".opus") || strings.HasSuffix(name, ".flac") {
			audioFile = name
		} else if strings.HasSuffix(name, ".jpg") || strings.HasSuffix(name, ".webp") || strings.HasSuffix(name, ".png") {
			if !strings.HasPrefix(name, "channel_") && !strings.HasPrefix(name, "storyboard") {
				thumbFile = name
			}
		} else if strings.HasSuffix(name, ".vtt") || strings.HasSuffix(name, ".srt") {
			subFile = name
		}
	}

	if infoPath == "" || (mediaFile == "" && audioFile == "") {
		return
	}

	data, err := os.ReadFile(infoPath)
	if err != nil {
		return
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return
	}

	title, _ := raw["title"].(string)
	id, _ := raw["id"].(string)
	channel, _ := raw["channel"].(string)
	if channel == "" {
		channel, _ = raw["uploader"].(string)
	}
	channelURL, _ := raw["channel_url"].(string)
	if channelURL == "" {
		channelURL, _ = raw["uploader_url"].(string)
	}
	duration, _ := raw["duration"].(float64)
	views, _ := raw["view_count"].(float64)
	likes, _ := raw["like_count"].(float64)
	uploadDate, _ := raw["upload_date"].(string)
	desc, _ := raw["description"].(string)

	var sbMeta *engine.StoryboardMetadata
	if sbData, err := os.ReadFile(filepath.Join(dir, "storyboard.json")); err == nil {
		var s engine.StoryboardMetadata
		if json.Unmarshal(sbData, &s) == nil {
			sbMeta = &s
		}
	}

	var liveChat []engine.LiveChatMessage
	if lcData, err := os.ReadFile(filepath.Join(dir, "live_chat.json")); err == nil {
		_ = json.Unmarshal(lcData, &liveChat)
	}

	var sponsorSegs []db.SponsorSegment
	if spData, err := os.ReadFile(filepath.Join(dir, "sponsorblock.json")); err == nil {
		_ = json.Unmarshal(spData, &sponsorSegs)
	}

	// Load comments from DB or disk comments.json
	var comments []*db.CommentItem
	if database != nil && id != "" {
		comments, _ = database.GetComments(id)
	}
	if len(comments) == 0 {
		if cData, err := os.ReadFile(filepath.Join(dir, "comments.json")); err == nil {
			var diskComments []*db.CommentItem
			if json.Unmarshal(cData, &diskComments) == nil && len(diskComments) > 0 {
				comments = diskComments
				if database != nil && id != "" {
					_ = database.SaveComments(id, diskComments)
				}
			}
		}
	} else {
		// Ensure raw comments.json is synced to disk
		commentsPath := filepath.Join(dir, "comments.json")
		if _, err := os.Stat(commentsPath); err != nil {
			if cBytes, err := json.MarshalIndent(comments, "", "  "); err == nil {
				_ = os.WriteFile(commentsPath, cBytes, 0644)
			}
		}
	}

	// Load avatars from zip
	avatarMap, _ := engine.LoadAvatarMapFromZip(filepath.Join(dir, "avatars.zip"))

	// Extract companion audio if missing
	ctx := context.Background()
	companionAudio := audioFile
	if companionAudio == "" && mediaFile != "" {
		companionAudio = engine.ExtractCompanionAudioFile(ctx, filepath.Join(dir, mediaFile), dir, "mp3")
	}

	// Generate NFO metadata
	_ = engine.GenerateNFOFile(dir, title, id, channel, int64(duration), uploadDate, desc, nil, nil, thumbFile, "channel_banner.jpg", "channel_avatar.jpg")

	chosenMedia := mediaFile
	isAudioOnly := false
	if chosenMedia == "" {
		chosenMedia = audioFile
		isAudioOnly = true
	}

	input := &generator.HTMLGeneratorInput{
		Title:                  title,
		VideoID:                id,
		Channel:                channel,
		ChannelURL:             channelURL,
		ChannelAvatarFilename:  "channel_avatar.jpg",
		Duration:               int64(duration),
		ViewCount:              int64(views),
		LikeCount:              int64(likes),
		UploadDate:             uploadDate,
		Description:            desc,
		MediaFilename:          chosenMedia,
		IsAudioOnly:            isAudioOnly,
		ThumbnailFilename:      thumbFile,
		SubtitlesFilename:      subFile,
		CompanionAudioFilename: companionAudio,
		CommentsCount:          len(comments),
		Comments:               comments,
		AvatarMap:              avatarMap,
		Storyboard:             sbMeta,
		LiveChat:               liveChat,
		SponsorSegments:        sponsorSegs,
	}

	outPath, err := generator.GenerateOfflineHTML(dir, input)
	if err != nil {
		fmt.Printf("Error generating HTML for %s: %v\n", dir, err)
		return
	}
	fmt.Printf("Successfully refreshed HTML: %s (Comments: %d, Subtitles: %s, Audio: %s)\n", outPath, len(comments), subFile, companionAudio)
}
