package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"youtube-downloader/internal/config"
	"youtube-downloader/internal/db"
	"youtube-downloader/internal/engine"
	"youtube-downloader/internal/generator"
	"youtube-downloader/internal/logger"
)

type CircuitBreaker struct {
	mu             sync.Mutex
	consecutive429 int
	cooldownUntil  time.Time
	reason         string
}

func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.consecutive429 = 0
}

func (cb *CircuitBreaker) RecordRateLimit(reason string) bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.consecutive429++
	if cb.consecutive429 >= 3 {
		cb.cooldownUntil = time.Now().Add(10 * time.Minute)
		cb.reason = reason
		return true
	}
	return false
}

func (cb *CircuitBreaker) IsTripped() (bool, time.Duration, string) {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	if time.Now().Before(cb.cooldownUntil) {
		return true, time.Until(cb.cooldownUntil), cb.reason
	}
	return false, 0, ""
}

func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.consecutive429 = 0
	cb.cooldownUntil = time.Time{}
	cb.reason = ""
}

type QueueManager struct {
	db             *db.DB
	mu             sync.Mutex
	isPausedAll    bool
	wakeUpCh       chan struct{}
	activeIDs      map[string]bool
	cancelFuncs    map[string]context.CancelFunc
	retryAt        map[string]time.Time
	circuitBreaker CircuitBreaker
}

var Manager *QueueManager

func InitQueueManager(database *db.DB) *QueueManager {
	qm := &QueueManager{
		db:          database,
		wakeUpCh:    make(chan struct{}, 10),
		activeIDs:   make(map[string]bool),
		cancelFuncs: make(map[string]context.CancelFunc),
		retryAt:     make(map[string]time.Time),
	}
	Manager = qm

	// Reset any stuck downloads from previous runs
	_ = database.ResetInterruptedDownloads()

	// Start worker loop in background
	go qm.workerLoop()
	go qm.autoRetryLoop()
	go qm.channelSyncLoop()

	return qm
}

func (qm *QueueManager) Enqueue(item *db.DownloadItem, customPrefs *db.UserPreferences) error {
	if customPrefs != nil {
		data, _ := json.Marshal(customPrefs)
		item.CustomOptions = string(data)
	}

	item.Status = "queued"
	item.Progress = 0
	item.CurrentStep = "Waiting in queue..."
	item.CreatedAt = time.Now()

	if err := qm.db.CreateDownload(item); err != nil {
		logger.Errorf("[Queue] Failed to create download item %s: %v", item.ID, err)
		return err
	}

	logger.Infof("[Queue] Enqueued item %s: %q (%s)", item.ID, item.Title, item.URL)
	Broadcaster.Broadcast("queue_update", item)
	qm.triggerWorker()
	return nil
}

// EnqueueBatchItems atomically enqueues a batch of download items with one DB transaction and one SSE broadcast
func (qm *QueueManager) EnqueueBatchItems(items []*db.DownloadItem, customPrefs *db.UserPreferences) (int, error) {
	if len(items) == 0 {
		return 0, nil
	}

	var customOptionsStr string
	if customPrefs != nil {
		data, _ := json.Marshal(customPrefs)
		customOptionsStr = string(data)
	}

	now := time.Now()
	for i, item := range items {
		if customOptionsStr != "" {
			item.CustomOptions = customOptionsStr
		}
		item.Status = "queued"
		item.Progress = 0
		item.CurrentStep = "Waiting in queue..."
		item.CreatedAt = now.Add(time.Duration(i) * time.Millisecond)
	}

	if err := qm.db.CreateDownloadsBatch(items); err != nil {
		logger.Errorf("[Queue] Failed to batch enqueue %d items: %v", len(items), err)
		return 0, err
	}

	logger.Infof("[Queue] Atomically enqueued batch of %d items", len(items))
	Broadcaster.Broadcast("queue_batch_added", map[string]interface{}{
		"count": len(items),
	})
	qm.triggerWorker()
	return len(items), nil
}

func (qm *QueueManager) Pause(id string) error {
	qm.mu.Lock()
	cancel, hasCancel := qm.cancelFuncs[id]
	qm.mu.Unlock()

	if hasCancel && cancel != nil {
		cancel()
	}
	engine.ProcessManager.Stop(id)

	logger.Infof("[Queue] Paused download %s", id)
	_ = qm.db.UpdateDownloadStatus(id, "paused", "")
	Broadcaster.Broadcast("status_change", map[string]string{"id": id, "status": "paused"})
	return nil
}

func (qm *QueueManager) Resume(id string) error {
	item, err := qm.db.GetDownload(id)
	if err != nil || item == nil {
		return fmt.Errorf("item not found")
	}

	logger.Infof("[Queue] Resumed download %s (%q)", id, item.Title)
	item.Status = "queued"
	item.ErrorMessage = ""
	item.CurrentStep = "Resuming download..."
	_ = qm.db.UpdateDownload(item)

	Broadcaster.Broadcast("status_change", map[string]string{"id": id, "status": "queued"})
	qm.triggerWorker()
	return nil
}

func (qm *QueueManager) Cancel(id string) error {
	qm.mu.Lock()
	cancel, hasCancel := qm.cancelFuncs[id]
	qm.mu.Unlock()

	if hasCancel && cancel != nil {
		cancel()
	}
	engine.ProcessManager.Stop(id)

	logger.Infof("[Queue] Cancelled download %s", id)
	_ = qm.db.UpdateDownloadStatus(id, "cancelled", "Cancelled by user")
	Broadcaster.Broadcast("status_change", map[string]string{"id": id, "status": "cancelled"})
	return nil
}

func (qm *QueueManager) Retry(id string) error {
	item, err := qm.db.GetDownload(id)
	if err != nil || item == nil {
		return fmt.Errorf("item not found")
	}

	logger.Infof("[Queue] Re-queued retry for download %s (%q)", id, item.Title)
	item.Status = "queued"
	item.Progress = 0
	item.ErrorMessage = ""
	item.CurrentStep = "Queued for retry..."
	_ = qm.db.UpdateDownload(item)
	qm.mu.Lock()
	delete(qm.retryAt, id)
	qm.mu.Unlock()

	Broadcaster.Broadcast("status_change", map[string]string{"id": id, "status": "queued"})
	qm.triggerWorker()
	return nil
}

func (qm *QueueManager) RetryWithAltClient(id string) error {
	item, err := qm.db.GetDownload(id)
	if err != nil || item == nil {
		return fmt.Errorf("item not found")
	}

	logger.Infof("[Queue] Re-queued retry with alternative player client for %s (%q)", id, item.Title)
	item.Status = "queued"
	item.Progress = 0
	item.ErrorMessage = ""
	item.CurrentStep = "Queued with alternative extractor client..."

	// Override custom options to force alternative extractor and best quality
	var opt map[string]interface{}
	if item.CustomOptions != "" {
		_ = json.Unmarshal([]byte(item.CustomOptions), &opt)
	}
	if opt == nil {
		opt = make(map[string]interface{})
	}
	opt["use_alt_extractor"] = true
	opt["video_quality"] = "best"
	optBytes, _ := json.Marshal(opt)
	item.CustomOptions = string(optBytes)

	_ = qm.db.UpdateDownload(item)
	qm.mu.Lock()
	delete(qm.retryAt, id)
	qm.mu.Unlock()

	Broadcaster.Broadcast("status_change", map[string]string{"id": id, "status": "queued"})
	qm.triggerWorker()
	return nil
}

func (qm *QueueManager) ResetCircuitBreaker() {
	qm.circuitBreaker.Reset()
	logger.Infof("[CircuitBreaker] Rate limit cooldown manually reset by user")
	Broadcaster.Broadcast("circuit_breaker", map[string]interface{}{
		"active":            false,
		"seconds_remaining": 0,
		"reason":            "",
	})
	qm.triggerWorker()
}

func (qm *QueueManager) GetCircuitBreakerStatus() map[string]interface{} {
	tripped, remaining, reason := qm.circuitBreaker.IsTripped()
	return map[string]interface{}{
		"active":            tripped,
		"seconds_remaining": int(remaining.Seconds()),
		"reason":            reason,
	}
}

func (qm *QueueManager) RetryAllFailed() (int, error) {
	failedItems, err := qm.db.GetAllDownloads("failed", "")
	if err != nil {
		return 0, err
	}

	count := 0
	for _, item := range failedItems {
		item.Status = "queued"
		item.Progress = 0
		item.ErrorMessage = ""
		item.CurrentStep = "Queued for retry..."
		_ = qm.db.UpdateDownload(item)
		Broadcaster.Broadcast("status_change", map[string]string{"id": item.ID, "status": "queued"})
		count++
	}

	if count > 0 {
		qm.triggerWorker()
		Broadcaster.Broadcast("toast", map[string]string{"message": fmt.Sprintf("Re-queued %d failed download(s)", count), "type": "success"})
	}
	return count, nil
}

func (qm *QueueManager) autoRetryLoop() {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		func() {
			defer func() {
				if r := recover(); r != nil {
					logger.Errorf("[Queue] Panic recovered in autoRetryLoop: %v", r)
				}
			}()

			prefs, err := qm.db.GetPreferences()
			if err != nil || prefs == nil || !prefs.AutoRetryFailed {
				return
			}
			if prefs.AutoRetryMaxAttempts <= 0 {
				prefs.AutoRetryMaxAttempts = 3
			}

			failed, _ := qm.db.GetAllDownloads("failed", "")
			now := time.Now()
			for _, item := range failed {
				qm.mu.Lock()
				due, exists := qm.retryAt[item.ID]
				qm.mu.Unlock()
				if exists && now.Before(due) {
					continue
				}
				attempt := retryAttempt(item.ErrorMessage)
				if attempt >= prefs.AutoRetryMaxAttempts {
					continue
				}
				item.Status, item.Progress, item.ErrorMessage = "queued", 0, ""
				item.CurrentStep = fmt.Sprintf("Automatic retry %d/%d starting...", attempt+1, prefs.AutoRetryMaxAttempts)
				_ = qm.db.UpdateDownload(item)
				qm.triggerWorker()
			}
		}()
	}
}

func retryAttempt(message string) int {
	var attempt int
	_, _ = fmt.Sscanf(message, "[retry:%d]", &attempt)
	return attempt
}

func actionableError(raw string) string {
	lower := strings.ToLower(raw)
	switch {
	case strings.Contains(lower, "sign in"), strings.Contains(lower, "cookies"), strings.Contains(lower, "login"):
		return "Authentication was required. Add a fresh cookies.txt file in Preferences, then retry."
	case strings.Contains(lower, "ffmpeg"), strings.Contains(lower, "merg"):
		return "FFmpeg could not process the media. Run the Engine health check and update or reinstall FFmpeg."
	case strings.Contains(lower, "not found"), strings.Contains(lower, "yt-dlp"):
		return "yt-dlp could not be started. Run the Engine health check and update or reinstall yt-dlp."
	case strings.Contains(lower, "429"), strings.Contains(lower, "rate limit"), strings.Contains(lower, "too many requests"):
		return "The source is rate-limiting requests. The retry will wait longer; consider setting a bandwidth limit."
	case strings.Contains(lower, "network"), strings.Contains(lower, "timed out"), strings.Contains(lower, "connection"):
		return "Network connection failed. Check your internet connection; this item will retry automatically."
	default:
		return "Download failed. Open the item details, verify the URL is still available, then retry."
	}
}

func withinWindow(now time.Time, start, end string) bool {
	parse := func(v string) (int, bool) {
		var h, m int
		if _, err := fmt.Sscanf(v, "%d:%d", &h, &m); err != nil || h < 0 || h > 23 || m < 0 || m > 59 {
			return 0, false
		}
		return h*60 + m, true
	}
	s, okS := parse(start)
	e, okE := parse(end)
	if !okS || !okE || s == e {
		return true
	}
	n := now.Hour()*60 + now.Minute()
	if s < e {
		return n >= s && n < e
	}
	return n >= s || n < e
}

func (qm *QueueManager) channelSyncLoop() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		func() {
			defer func() {
				if r := recover(); r != nil {
					logger.Errorf("[Queue] Panic recovered in channelSyncLoop: %v", r)
				}
			}()

			prefs, _ := qm.db.GetPreferences()
			if prefs == nil || !prefs.AutoSyncChannels {
				return
			}
			interval := 60
			if prefs.SyncIntervalMinutes > 0 {
				interval = prefs.SyncIntervalMinutes
			}

			channels, _ := qm.db.GetAllChannels()
			for _, channel := range channels {
				if channel.LastSynced != nil && time.Since(*channel.LastSynced) < time.Duration(interval)*time.Minute {
					continue
				}

				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
				// Automatically sync profile metadata, subscribers, and video count
				_ = qm.RefreshChannelMetadata(ctx, channel.ID)

				// Only auto-download videos if AutoDownload is explicitly enabled for this channel
				if channel.AutoDownload {
					if !prefs.SyncWindowEnabled || withinWindow(time.Now(), prefs.SyncWindowStart, prefs.SyncWindowEnd) {
						_, _ = qm.SyncChannel(ctx, channel.ID)
					}
				}
				cancel()
			}
		}()
	}
}

// RefreshChannelMetadata updates channel info (subscribers, avatar, video count) without downloading videos
func (qm *QueueManager) RefreshChannelMetadata(ctx context.Context, channelID string) error {
	ch, err := qm.db.GetChannel(channelID)
	if err != nil || ch == nil {
		return fmt.Errorf("channel not found")
	}

	catalog, err := engine.InspectChannelCatalog(ctx, ch.URL, 0)
	if err != nil {
		return err
	}

	return qm.db.UpdateChannelFromCatalog(ch.ID, catalog.Title, catalog.Handle, catalog.AvatarURL, catalog.SubscriberCount, catalog.TotalVideos)
}

func (qm *QueueManager) PauseAll() {
	qm.mu.Lock()
	qm.isPausedAll = true
	// Stop all active downloads
	for id, cancel := range qm.cancelFuncs {
		if cancel != nil {
			cancel()
		}
		engine.ProcessManager.Stop(id)
	}
	qm.mu.Unlock()

	// Update all active and queued items in DB to paused
	_ = qm.db.PauseAllDownloads()

	logger.Infof("[Queue] Paused all active and queued downloads")
	Broadcaster.Broadcast("toast", map[string]string{"message": "All downloads paused", "type": "info"})
	Broadcaster.Broadcast("queue_update", nil)
}

func (qm *QueueManager) ResumeAll() {
	qm.mu.Lock()
	qm.isPausedAll = false
	qm.mu.Unlock()

	// Update all paused items in DB to queued
	_ = qm.db.ResumeAllDownloads()

	logger.Infof("[Queue] Resumed all paused downloads")
	Broadcaster.Broadcast("toast", map[string]string{"message": "All downloads resumed", "type": "info"})
	Broadcaster.Broadcast("queue_update", nil)
	qm.triggerWorker()
}

func (qm *QueueManager) Delete(id string, deleteFiles bool) error {
	item, err := qm.db.GetDownload(id)
	if err == nil && item != nil {
		qm.Cancel(id)
		if deleteFiles && item.OutputDir != "" {
			_ = os.RemoveAll(item.OutputDir)
		}
	}
	err = qm.db.DeleteDownload(id)
	Broadcaster.Broadcast("queue_update", map[string]string{"id": id, "action": "deleted"})
	return err
}

func (qm *QueueManager) ClearQueue() (int, error) {
	qm.mu.Lock()
	for id, cancel := range qm.cancelFuncs {
		if cancel != nil {
			cancel()
		}
		engine.ProcessManager.Stop(id)
	}
	qm.cancelFuncs = make(map[string]context.CancelFunc)
	qm.mu.Unlock()

	count, err := qm.db.ClearQueue()
	if err != nil {
		return 0, err
	}

	Broadcaster.Broadcast("queue_update", map[string]interface{}{"action": "cleared", "count": count})
	Broadcaster.Broadcast("toast", map[string]string{"message": fmt.Sprintf("Cleared %d items from queue", count), "type": "info"})
	return int(count), nil
}

// ReorderQueue sets the queue execution sequence
func (qm *QueueManager) ReorderQueue(orderedIDs []string) error {
	if len(orderedIDs) == 0 {
		return nil
	}
	if err := qm.db.ReorderQueue(orderedIDs); err != nil {
		logger.Errorf("[Queue] Failed to reorder queue: %v", err)
		return err
	}
	logger.Infof("[Queue] Reordered %d queued items in database", len(orderedIDs))
	Broadcaster.Broadcast("queue_reordered", map[string]interface{}{"ids": orderedIDs})
	qm.triggerWorker()
	return nil
}

func (qm *QueueManager) triggerWorker() {
	select {
	case qm.wakeUpCh <- struct{}{}:
	default:
	}
}

func (qm *QueueManager) workerLoop() {
	for {
		qm.mu.Lock()
		paused := qm.isPausedAll
		activeCount := len(qm.activeIDs)
		qm.mu.Unlock()

		if paused {
			select {
			case <-qm.wakeUpCh:
			case <-time.After(3 * time.Second):
			}
			continue
		}

		prefs, err := qm.db.GetPreferences()
		if err != nil || prefs == nil {
			prefs = db.DefaultPreferences("./downloads")
		}

		// Check Circuit Breaker for rate limiting (429) cooldown if enabled
		if prefs.CircuitBreakerEnabled {
			if tripped, remaining, reason := qm.circuitBreaker.IsTripped(); tripped {
				Broadcaster.Broadcast("circuit_breaker", map[string]interface{}{
					"active":            true,
					"seconds_remaining": int(remaining.Seconds()),
					"reason":            reason,
				})
				select {
				case <-qm.wakeUpCh:
				case <-time.After(5 * time.Second):
				}
				continue
			}
		}
		if prefs.DownloadWindowEnabled && !withinWindow(time.Now(), prefs.DownloadWindowStart, prefs.DownloadWindowEnd) {
			select {
			case <-qm.wakeUpCh:
			case <-time.After(30 * time.Second):
			}
			continue
		}

		maxConcurrent := prefs.MaxConcurrentDownloads
		if maxConcurrent <= 0 {
			maxConcurrent = 2
		}

		if activeCount >= maxConcurrent {
			select {
			case <-qm.wakeUpCh:
			case <-time.After(2 * time.Second):
			}
			continue
		}

		// Find next queued item
		queued, err := qm.db.GetAllDownloads("queued", "")
		if err != nil || len(queued) == 0 {
			select {
			case <-qm.wakeUpCh:
			case <-time.After(3 * time.Second):
			}
			continue
		}

		// Pick the first queued item not active
		var nextItem *db.DownloadItem
		qm.mu.Lock()
		for _, q := range queued {
			if !qm.activeIDs[q.ID] {
				nextItem = q
				qm.activeIDs[q.ID] = true
				break
			}
		}
		qm.mu.Unlock()

		if nextItem != nil {
			go qm.processItem(nextItem)
		} else {
			select {
			case <-qm.wakeUpCh:
			case <-time.After(2 * time.Second):
			}
		}
	}
}

func (qm *QueueManager) processItem(item *db.DownloadItem) {
	defer func() {
		if r := recover(); r != nil {
			logger.Errorf("[Queue] Panic recovered in processItem for %s: %v", item.ID, r)
			_ = qm.db.UpdateDownloadStatus(item.ID, "failed", fmt.Sprintf("Internal worker error: %v", r))
		}
		qm.mu.Lock()
		delete(qm.activeIDs, item.ID)
		delete(qm.cancelFuncs, item.ID)
		qm.mu.Unlock()
		qm.triggerWorker()
	}()

	ctx, cancel := context.WithCancel(context.Background())
	qm.mu.Lock()
	qm.cancelFuncs[item.ID] = cancel
	qm.mu.Unlock()

	// Load effective preferences (global with item overrides)
	prefs, _ := qm.db.GetPreferences()
	if prefs == nil {
		prefs = db.DefaultPreferences("./downloads")
	}

	useAltExtractor := false
	if item.CustomOptions != "" {
		var override map[string]interface{}
		if err := json.Unmarshal([]byte(item.CustomOptions), &override); err == nil {
			if v, ok := override["use_alt_extractor"].(bool); ok {
				useAltExtractor = v
			}
			if v, ok := override["download_video"].(bool); ok {
				prefs.DownloadVideo = v
			}
			if v, ok := override["video_format"].(string); ok && v != "" {
				prefs.VideoFormat = v
			}
			if v, ok := override["video_quality"].(string); ok && v != "" {
				prefs.VideoQuality = v
			}
			if v, ok := override["download_audio_only"].(bool); ok {
				prefs.DownloadAudioOnly = v
			}
			if v, ok := override["audio_format"].(string); ok && v != "" {
				prefs.AudioFormat = v
			}
			if v, ok := override["audio_quality"].(string); ok && v != "" {
				prefs.AudioQuality = v
			}
			if v, ok := override["download_thumbnail"].(bool); ok {
				prefs.DownloadThumbnail = v
			}
			if v, ok := override["download_metadata"].(bool); ok {
				prefs.DownloadMetadata = v
			}
			if v, ok := override["download_subtitles"].(bool); ok {
				prefs.DownloadSubtitles = v
			}
			if v, ok := override["download_comments"].(bool); ok {
				prefs.DownloadComments = v
			}
			if v, ok := override["comment_limit"].(float64); ok {
				prefs.CommentLimit = int(v)
			}
			if v, ok := override["download_commenter_avatars"].(bool); ok {
				prefs.DownloadCommenterAvatars = v
			}
			if v, ok := override["generate_html_report"].(bool); ok {
				prefs.GenerateHTMLReport = v
			}
			if v, ok := override["speed_limit"].(string); ok {
				prefs.SpeedLimit = v
			}
			if v, ok := override["download_folder"].(string); ok && v != "" {
				prefs.DownloadFolder = v
			}
			if v, ok := override["download_subtitles"].(bool); ok {
				prefs.DownloadSubtitles = v
			}
			if v, ok := override["subtitle_langs"].(string); ok {
				prefs.SubtitleLangs = v
			}
			if v, ok := override["fetch_dislikes"].(bool); ok {
				prefs.FetchDislikes = v
			}
			if v, ok := override["embed_metadata"].(bool); ok {
				prefs.EmbedMetadata = v
			}
			if v, ok := override["embed_cover_art"].(bool); ok {
				prefs.EmbedCoverArt = v
			}
			if v, ok := override["embed_chapters"].(bool); ok {
				prefs.EmbedChapters = v
			}
		}
	}

	logger.Infof("[Pipeline] Starting download pipeline for: %q (ID: %s, VideoID: %s, Format: %s, Quality: %s, AltExtractor: %t)", item.Title, item.ID, item.VideoID, prefs.VideoFormat, prefs.VideoQuality, useAltExtractor)

	// Update status to downloading
	item.Status = "downloading"
	item.CurrentStep = "Preparing pipeline..."
	_ = qm.db.UpdateDownload(item)
	Broadcaster.Broadcast("status_change", item)

	// Helper: check if a stage was already completed in a prior run
	hasStage := func(stage string) bool {
		for _, s := range splitStages(item.CompletedStages) {
			if s == stage {
				return true
			}
		}
		return false
	}

	// Helper: mark a stage as completed and persist to DB
	addStage := func(stage string) {
		stages := splitStages(item.CompletedStages)
		for _, s := range stages {
			if s == stage {
				return
			}
		}
		stages = append(stages, stage)
		item.CompletedStages = joinStages(stages)
		_ = qm.db.UpdateDownload(item)
	}

	// Track the yt-dlp download result for later stages
	var res *engine.DownloadExecutionResult

	// Step 1: Media Download (skip if already completed)
	if hasStage("media") {
		logger.Infof("[Pipeline] Media already downloaded for %s, skipping step 1", item.VideoID)
		item.CurrentStep = "Media already downloaded, skipping..."
		Broadcaster.Broadcast("progress", item)
	} else {
		item.CurrentStep = "Downloading media stream..."
		_ = qm.db.UpdateDownload(item)
		Broadcaster.Broadcast("progress", item)

		var lastBroadcast time.Time
		var err error
		progressCb := func(progress float64, speed, eta string, downloaded, total int64, step string) {
			item.Progress = progress
			item.Speed = speed
			item.ETA = eta
			item.DownloadedBytes = downloaded
			item.TotalBytes = total
			item.CurrentStep = step

			// Throttle DB writes & SSE broadcasts to avoid high load
			if time.Since(lastBroadcast) > 300*time.Millisecond || progress >= 99.0 {
				lastBroadcast = time.Now()
				_ = qm.db.UpdateDownloadProgress(item.ID, progress, speed, eta, downloaded, total, step)
				Broadcaster.Broadcast("progress", item)
			}
		}

		if useAltExtractor {
			res, err = engine.ExecuteMediaDownloadWithAltClient(ctx, item.ID, item, prefs, progressCb)
		} else {
			res, err = engine.ExecuteMediaDownload(ctx, item.ID, item, prefs, progressCb)
		}

		if err != nil {
			select {
			case <-ctx.Done():
				logger.Warnf("[Pipeline] Item cancelled or paused: %s (%q)", item.ID, item.Title)
				return
			default:
			}
			attempt := retryAttempt(item.ErrorMessage) + 1
			diag := engine.ClassifyError(err.Error())

			if prefs.CircuitBreakerEnabled && (diag.Category == engine.ErrCategoryRateLimited || diag.Category == engine.ErrCategoryAuthRequired) {
				if qm.circuitBreaker.RecordRateLimit(diag.ActionableMessage) {
					logger.Warnf("[CircuitBreaker] Activated 10-minute rate limit cooldown. Reason: %s", diag.ActionableMessage)
					Broadcaster.Broadcast("circuit_breaker", map[string]interface{}{
						"active":            true,
						"seconds_remaining": 600,
						"reason":            diag.ActionableMessage,
					})
				}
			}

			item.Status = "failed"
			item.ErrorMessage = fmt.Sprintf("[%s] [retry:%d] %s", diag.Badge, attempt, diag.ActionableMessage)
			item.CurrentStep = diag.Badge + ": " + diag.ActionableMessage
			_ = qm.db.UpdateDownload(item)
			logger.Errorf("[Pipeline] Media download failed for %q (ID: %s) [%s]: %s | Raw Error: %v", item.Title, item.ID, diag.Badge, diag.ActionableMessage, err)

			if !diag.IsPermanent && prefs.AutoRetryFailed && attempt < prefs.AutoRetryMaxAttempts {
				base := prefs.AutoRetryIntervalMinutes
				if base <= 0 {
					base = 5
				}
				delay := time.Duration(base) * time.Minute * time.Duration(1<<(attempt-1))
				qm.mu.Lock()
				qm.retryAt[item.ID] = time.Now().Add(delay)
				qm.mu.Unlock()
				item.CurrentStep = fmt.Sprintf("%s (Retry %d/%d in %s)", diag.Badge, attempt, prefs.AutoRetryMaxAttempts, delay.Round(time.Minute))
				_ = qm.db.UpdateDownload(item)
				logger.Infof("[Pipeline] Scheduled retry %d/%d for %s in %s", attempt, prefs.AutoRetryMaxAttempts, item.ID, delay.Round(time.Minute))
			}
			Broadcaster.Broadcast("status_change", item)
			return
		}

		// Record successful download on circuit breaker
		qm.circuitBreaker.RecordSuccess()

		if res != nil {
			item.MediaFilePath = res.MediaFilePath
			item.ThumbnailPath = res.ThumbnailPath
			item.MetadataFilePath = res.MetadataFilePath
		}

		// Stage completed — persist immediately so a crash after this point won't re-download
		addStage("media")
	}

	// Read metadata from .info.json if available
	var meta db.VideoMetadata
	if item.MetadataFilePath != "" {
		if metaBytes, err := os.ReadFile(item.MetadataFilePath); err == nil {
			var rawMeta map[string]interface{}
			if err := json.Unmarshal(metaBytes, &rawMeta); err == nil {
				meta.Title, _ = rawMeta["title"].(string)
				meta.Description, _ = rawMeta["description"].(string)
				meta.Channel, _ = rawMeta["uploader"].(string)
				if meta.Channel == "" {
					meta.Channel, _ = rawMeta["channel"].(string)
				}
				meta.ChannelURL, _ = rawMeta["channel_url"].(string)
				if meta.ChannelURL == "" {
					meta.ChannelURL, _ = rawMeta["uploader_url"].(string)
				}
				if meta.ChannelURL == "" && item.ChannelURL != "" {
					meta.ChannelURL = item.ChannelURL
				}
				if meta.Channel == "" && item.Channel != "" {
					meta.Channel = item.Channel
				}
				meta.ChannelAvatar, _ = rawMeta["channel_avatar"].(string)
				meta.UploadDate, _ = rawMeta["upload_date"].(string)
				if lk, ok := rawMeta["like_count"].(float64); ok {
					meta.LikeCount = int64(lk)
				}
				if vw, ok := rawMeta["view_count"].(float64); ok {
					meta.ViewCount = int64(vw)
				}
				if subs, ok := rawMeta["channel_follower_count"].(float64); ok {
					meta.SubscriberCount = int64(subs)
				} else if subs, ok := rawMeta["subscriber_count"].(float64); ok {
					meta.SubscriberCount = int64(subs)
				}
				if tags, ok := rawMeta["tags"].([]interface{}); ok {
					for _, t := range tags {
						if ts, ok := t.(string); ok {
							meta.Tags = append(meta.Tags, ts)
						}
					}
				}
				if chList, ok := rawMeta["chapters"].([]interface{}); ok {
					for _, ch := range chList {
						if chMap, ok := ch.(map[string]interface{}); ok {
							cTitle, _ := chMap["title"].(string)
							cStart, _ := chMap["start_time"].(float64)
							cEnd, _ := chMap["end_time"].(float64)
							meta.Chapters = append(meta.Chapters, db.VideoChapter{
								Title:     cTitle,
								StartTime: cStart,
								EndTime:   cEnd,
							})
						}
					}
				}

				if meta.Title != "" {
					item.Title = meta.Title
				}
				if meta.Channel != "" {
					item.Channel = meta.Channel
				}
			}
		}
	}

	// Fetch SponsorBlock segments if enabled
	if sbSegments := engine.FetchSponsorBlockSegments(ctx, item.VideoID, item.OutputDir); len(sbSegments) > 0 {
		meta.SponsorSegments = sbSegments
	}

	// Fetch Dislikes via Return YouTube Dislike (RYD) API if enabled
	if prefs.FetchDislikes {
		if ryd, err := engine.FetchReturnYouTubeDislike(ctx, item.VideoID); err == nil && ryd != nil {
			meta.DislikeCount = ryd.Dislikes
			meta.Rating = ryd.Rating
			logger.Infof("[Dislikes] Fetched ReturnYouTubeDislike for %s (%q): %d dislikes, rating %.2f", item.VideoID, item.Title, ryd.Dislikes, ryd.Rating)
		}
	}

	// Step 2: Fetch Comments & Avatars (if enabled, skip if already completed)
	var comments []*db.CommentItem
	var avatarMap map[string]string

	if prefs.DownloadComments {
		if hasStage("comments") {
			// Comments were already fetched in a prior run — load them from the DB or comments.json
			item.CurrentStep = "Comments already downloaded, loading from database..."
			Broadcaster.Broadcast("progress", item)
			dbComments, err := qm.db.GetComments(item.VideoID)
			if (err != nil || len(dbComments) == 0) && item.OutputDir != "" {
				commentsJSONPath := filepath.Join(item.OutputDir, "comments.json")
				if data, err := os.ReadFile(commentsJSONPath); err == nil {
					var diskComments []*db.CommentItem
					if json.Unmarshal(data, &diskComments) == nil && len(diskComments) > 0 {
						dbComments = diskComments
						_ = qm.db.SaveComments(item.VideoID, diskComments)
					}
				}
			}
			if len(dbComments) > 0 {
				comments = dbComments
				item.CommentsCount = countAllComments(dbComments)
			}
		} else {
			item.CurrentStep = "Extracting comments & replies..."
			_ = qm.db.UpdateDownload(item)
			Broadcaster.Broadcast("progress", item)

			commRes, err := engine.FetchComments(ctx, item.URL, item.VideoID, prefs.CommentLimit, prefs.DownloadCommenterAvatars, item.OutputDir, func(step string, count int) {
				item.CurrentStep = step
				Broadcaster.Broadcast("progress", item)
			})

			if err != nil {
				logger.Warnf("[Pipeline] Non-fatal error during comment extraction for %s: %v", item.ID, err)
			} else if commRes != nil {
				comments = commRes.Comments
				avatarMap = commRes.AvatarMap
				item.CommentsCount = commRes.TotalCount

				// Save comments to DB and write comments.json to output directory
				if len(comments) > 0 {
					_ = qm.db.SaveComments(item.VideoID, comments)
					if item.OutputDir != "" {
						commentsJSONPath := filepath.Join(item.OutputDir, "comments.json")
						if cBytes, err := json.MarshalIndent(comments, "", "  "); err == nil {
							_ = os.WriteFile(commentsJSONPath, cBytes, 0644)
						}
					}
				}
				addStage("comments")
			}
		}
	}

	// Step 3: Generate Interactive Offline YouTube HTML Player
	if prefs.GenerateHTMLReport {
		item.CurrentStep = "Generating offline YouTube experience..."
		_ = qm.db.UpdateDownload(item)
		Broadcaster.Broadcast("progress", item)

		// Discover media, thumbnail, subtitle files in output directory if not recorded
		// Discover media, thumbnail, subtitle files in output directory if not recorded
		if item.MediaFilePath == "" {
			item.MediaFilePath = findFileWithExts(item.OutputDir, []string{".mp4", ".mkv", ".webm", ".mp3", ".m4a", ".flac", ".wav", ".opus"})
		}
		if item.ThumbnailPath == "" {
			item.ThumbnailPath = findFileWithExts(item.OutputDir, []string{".jpg", ".jpeg", ".webp", ".png"})
		}
		subtitlesPath := findFileWithExts(item.OutputDir, []string{".vtt", ".srt"})

		var fileSize int64
		if item.MediaFilePath != "" {
			if fi, err := os.Stat(item.MediaFilePath); err == nil {
				fileSize = fi.Size()
				item.TotalBytes = fileSize
				item.DownloadedBytes = fileSize
			}
		}

		// Fetch rich channel metadata, banner, avatar & save channel.json
		channelAvatarFilename := "channel_avatar.jpg"
		chanMeta := engine.FetchChannelMetadata(ctx, meta.ChannelURL, item.Channel, item.OutputDir)
		if chanMeta != nil {
			if chanMeta.AvatarFilename != "" {
				channelAvatarFilename = chanMeta.AvatarFilename
			}
			if chanMeta.SubscriberCount > 0 {
				meta.SubscriberCount = chanMeta.SubscriberCount
			}
			if chanMeta.Description != "" && meta.Description == "" {
				meta.Description = chanMeta.Description
			}
		}

		mediaFilename := ""
		if item.MediaFilePath != "" {
			mediaFilename = filepath.Base(item.MediaFilePath)
		}
		thumbnailFilename := ""
		if item.ThumbnailPath != "" {
			thumbnailFilename = filepath.Base(item.ThumbnailPath)
		}
		subtitlesFilename := ""
		if subtitlesPath != "" {
			subtitlesFilename = filepath.Base(subtitlesPath)
		}

		// Fetch SponsorBlock segments if not already present
		if len(meta.SponsorSegments) == 0 {
			meta.SponsorSegments = engine.FetchSponsorBlockSegments(ctx, item.VideoID, item.OutputDir)
		}

		// Generate or extract Storyboard spritesheet for hover scrubbing
		storyboardMeta := engine.GenerateOrExtractStoryboard(ctx, item.MediaFilePath, item.Duration, item.OutputDir)

		// Extract & process Live Chat Replay
		liveChatMessages := engine.ProcessLiveChatFile(item.OutputDir)

		// Extract companion audio if enabled and not audio-only
		companionAudioFilename := ""
		if prefs.ExtractCompanionAudio && !item.IsAudioOnly && item.MediaFilePath != "" {
			audioFmt := prefs.CompanionAudioFormat
			if audioFmt == "" {
				audioFmt = "mp3"
			}
			companionAudioFilename = engine.ExtractCompanionAudioFile(ctx, item.MediaFilePath, item.OutputDir, audioFmt)
		}

		// Generate Jellyfin/Plex/Kodi .nfo metadata
		if prefs.GenerateNFO {
			bannerFilename := "channel_banner.jpg"
			if chanMeta != nil && chanMeta.BannerFilename != "" {
				bannerFilename = chanMeta.BannerFilename
			}
			_ = engine.GenerateNFOFile(item.OutputDir, item.Title, item.VideoID, item.Channel, item.Duration, meta.UploadDate, meta.Description, meta.Categories, meta.Tags, thumbnailFilename, bannerFilename, channelAvatarFilename)
		}

		// Generate Standalone Channel Page (channel.html)
		chanVideos := []generator.ChannelVideoItem{
			{
				Title:             item.Title,
				VideoID:           item.VideoID,
				RelativeURL:       "index.html",
				ThumbnailFilename: thumbnailFilename,
				Duration:          item.Duration,
				UploadDate:        meta.UploadDate,
			},
		}
		_, _ = generator.GenerateChannelHTML(item.OutputDir, chanMeta, chanVideos)

		genInput := &generator.HTMLGeneratorInput{
			Title:                  item.Title,
			VideoID:                item.VideoID,
			Channel:                item.Channel,
			ChannelURL:             item.ChannelURL,
			ChannelAvatar:          meta.ChannelAvatar,
			ChannelAvatarFilename:  channelAvatarFilename,
			SubscriberCount:        meta.SubscriberCount,
			Duration:               item.Duration,
			ViewCount:              meta.ViewCount,
			LikeCount:              meta.LikeCount,
			DislikeCount:           meta.DislikeCount,
			Rating:                 meta.Rating,
			UploadDate:             meta.UploadDate,
			Description:            meta.Description,
			Categories:             meta.Categories,
			Tags:                   meta.Tags,
			MediaFilename:          mediaFilename,
			IsAudioOnly:            item.IsAudioOnly,
			ThumbnailFilename:      thumbnailFilename,
			SubtitlesFilename:      subtitlesFilename,
			CompanionAudioFilename: companionAudioFilename,
			CommentsCount:          item.CommentsCount,
			Comments:               comments,
			AvatarMap:              avatarMap,
			Chapters:               meta.Chapters,
			SponsorSegments:        meta.SponsorSegments,
			Storyboard:             storyboardMeta,
			LiveChat:               liveChatMessages,
			SourceURL:              item.URL,
			VideoQuality:           item.Quality,
			Filesize:               fileSize,
		}

		htmlPath, err := generator.GenerateOfflineHTML(item.OutputDir, genInput)
		if err == nil {
			item.HTMLFilePath = htmlPath

			// Register in Master Portal & Catalog
			baseDownloadsDir := ""
			if config.GlobalConfig != nil {
				baseDownloadsDir = config.GlobalConfig.DefaultOut
			}
			if baseDownloadsDir != "" {
				relPlayerPath, _ := filepath.Rel(baseDownloadsDir, htmlPath)
				relThumbPath := ""
				if item.ThumbnailPath != "" {
					relThumbPath, _ = filepath.Rel(baseDownloadsDir, item.ThumbnailPath)
				}
				relAvatarPath := ""
				avatarPath := filepath.Join(item.OutputDir, "channel_avatar.jpg")
				if _, err := os.Stat(avatarPath); err == nil {
					relAvatarPath, _ = filepath.Rel(baseDownloadsDir, avatarPath)
				}
				relChanPath := ""
				chanHTMLPath := filepath.Join(item.OutputDir, "channel.html")
				if _, err := os.Stat(chanHTMLPath); err == nil {
					relChanPath, _ = filepath.Rel(baseDownloadsDir, chanHTMLPath)
				}

				portalItem := &generator.PortalCatalogItem{
					VideoID:              item.VideoID,
					Title:                item.Title,
					Channel:              item.Channel,
					ChannelURL:           meta.ChannelURL,
					Duration:             item.Duration,
					ViewCount:            meta.ViewCount,
					UploadDate:           meta.UploadDate,
					Description:          meta.Description,
					Tags:                 meta.Tags,
					RelativePlayerURL:    filepath.ToSlash(relPlayerPath),
					RelativeThumbnailURL: filepath.ToSlash(relThumbPath),
					RelativeAvatarURL:    filepath.ToSlash(relAvatarPath),
					RelativeChannelURL:   filepath.ToSlash(relChanPath),
				}
				_ = generator.RegisterVideoInMasterPortal(baseDownloadsDir, portalItem, chanMeta)
			}
		}
		addStage("html")
	}

	// Step 4: Mark Completed
	now := time.Now()
	item.Status = "completed"
	item.Progress = 100.0
	item.CompletedAt = &now
	item.CurrentStep = "Complete! Ready to watch offline"
	item.Speed = ""
	item.ETA = ""
	_ = qm.db.UpdateDownload(item)
	Broadcaster.Broadcast("status_change", item)
}

// EnqueueBatch queues multiple URLs with optional custom preferences
func (qm *QueueManager) EnqueueBatch(urls []string, customPrefs *db.UserPreferences) (int, error) {
	prefs, _ := qm.db.GetPreferences()
	if prefs == nil {
		prefs = db.DefaultPreferences("./downloads")
	}
	if customPrefs != nil && customPrefs.DuplicateAction != "" {
		prefs.DuplicateAction = customPrefs.DuplicateAction
	}

	queued := 0
	for _, rawURL := range urls {
		rawURL = strings.TrimSpace(rawURL)
		if rawURL == "" || strings.HasPrefix(rawURL, "#") {
			continue
		}

		videoID := engine.ExtractVideoID(rawURL)
		if prefs.DuplicateAction == "skip" || prefs.DuplicateAction == "" {
			if dup, _ := engine.CheckDuplicate(videoID, rawURL, qm.db); dup != nil && dup.IsDuplicate && (dup.FileExists || (dup.ExistingItem != nil && (dup.ExistingItem.Status == "downloading" || dup.ExistingItem.Status == "queued"))) {
				continue
			}
		}

		format := prefs.VideoFormat
		quality := prefs.VideoQuality
		if format == "" {
			format = "mp4"
		}
		if quality == "" {
			quality = "best"
		}
		if prefs.DownloadAudioOnly {
			format = prefs.AudioFormat
			quality = prefs.AudioQuality
		}

		downloadID := fmt.Sprintf("dl_%d_%d", time.Now().UnixNano(), queued)

		item := &db.DownloadItem{
			ID:          downloadID,
			URL:         rawURL,
			VideoID:     videoID,
			Title:       "Queued Item (" + rawURL + ")",
			Format:      format,
			Quality:     quality,
			IsAudioOnly: prefs.DownloadAudioOnly,
			Status:      "queued",
			CurrentStep: "Waiting in queue...",
			Progress:    0,
			CreatedAt:   time.Now(),
		}

		if err := qm.Enqueue(item, customPrefs); err == nil {
			queued++
		}
	}
	return queued, nil
}

// SyncChannel checks a channel catalog and enqueues unarchived videos
func (qm *QueueManager) SyncChannel(ctx context.Context, channelID string) (int, error) {
	ch, err := qm.db.GetChannel(channelID)
	if err != nil || ch == nil {
		return 0, fmt.Errorf("channel not found")
	}

	prefs, _ := qm.db.GetPreferences()
	if prefs == nil {
		prefs = db.DefaultPreferences("./downloads")
	}

	catalog, err := engine.InspectChannelCatalog(ctx, ch.URL, 0)
	if err != nil {
		return 0, fmt.Errorf("failed to inspect channel catalog: %w", err)
	}

	var videoIDs []string
	for _, v := range catalog.Videos {
		if v.ID != "" {
			videoIDs = append(videoIDs, v.ID)
		}
	}

	existingMap, _ := qm.db.FindDuplicatesBatch(videoIDs, nil)

	format := prefs.VideoFormat
	quality := prefs.VideoQuality
	if format == "" {
		format = "mp4"
	}
	if quality == "" {
		quality = "best"
	}
	if prefs.DownloadAudioOnly {
		format = prefs.AudioFormat
		quality = prefs.AudioQuality
	}

	var itemsToEnqueue []*db.DownloadItem
	for _, v := range catalog.Videos {
		if existingMap[v.ID] != nil {
			continue
		}

		// Apply per-channel filtering rules
		if ch.ExcludeShorts && (strings.Contains(v.URL, "/shorts/") || (v.Duration > 0 && v.Duration <= 60)) {
			continue
		}
		if ch.ExcludeLiveStreams && strings.Contains(v.URL, "/live") {
			continue
		}
		if ch.MinDurationSec > 0 && v.Duration > 0 && v.Duration < ch.MinDurationSec {
			continue
		}

		cat := "Videos"
		if strings.Contains(v.URL, "/shorts/") || (v.Duration > 0 && v.Duration <= 60) {
			cat = "Shorts"
		} else if strings.Contains(v.URL, "/live") {
			cat = "Live Streams"
		}

		downloadID := fmt.Sprintf("dl_%d_%s", time.Now().UnixNano(), v.ID)
		item := &db.DownloadItem{
			ID:           downloadID,
			URL:          v.URL,
			VideoID:      v.ID,
			Title:        v.Title,
			Channel:      catalog.Title,
			ChannelURL:   ch.URL,
			Duration:     v.Duration,
			ThumbnailURL: v.Thumbnail,
			Format:       format,
			Quality:      quality,
			IsAudioOnly:  prefs.DownloadAudioOnly,
			Category:     cat,
			Status:       "queued",
			CurrentStep:  "Queued from Channel Sync",
			CreatedAt:    time.Now(),
		}
		itemsToEnqueue = append(itemsToEnqueue, item)
		if ch.MaxAutoSyncCount > 0 && len(itemsToEnqueue) >= ch.MaxAutoSyncCount {
			break
		}
	}

	newCount, _ := qm.EnqueueBatchItems(itemsToEnqueue, nil)
	_ = qm.db.UpdateChannelFromCatalog(ch.ID, catalog.Title, catalog.Handle, catalog.AvatarURL, catalog.SubscriberCount, catalog.TotalVideos)
	return newCount, nil
}

// EnqueueChannelSelectedVideos enqueues only specifically selected videos from the Channel Studio
func (qm *QueueManager) EnqueueChannelSelectedVideos(ctx context.Context, channelID string, videoIDs []string) (int, error) {
	if len(videoIDs) == 0 {
		return 0, fmt.Errorf("no videos selected")
	}

	ch, err := qm.db.GetChannel(channelID)
	if err != nil || ch == nil {
		return 0, fmt.Errorf("channel not found")
	}

	prefs, _ := qm.db.GetPreferences()
	if prefs == nil {
		prefs = db.DefaultPreferences("./downloads")
	}

	catalog, err := engine.InspectChannelCatalog(ctx, ch.URL, 0)
	if err != nil {
		return 0, fmt.Errorf("failed to inspect channel catalog: %w", err)
	}

	selectedSet := make(map[string]bool)
	for _, id := range videoIDs {
		selectedSet[id] = true
	}

	existingMap, _ := qm.db.FindDuplicatesBatch(videoIDs, nil)

	format := prefs.VideoFormat
	quality := prefs.VideoQuality
	if format == "" {
		format = "mp4"
	}
	if quality == "" {
		quality = "best"
	}
	if prefs.DownloadAudioOnly {
		format = prefs.AudioFormat
		quality = prefs.AudioQuality
	}

	var itemsToEnqueue []*db.DownloadItem
	for _, v := range catalog.Videos {
		if !selectedSet[v.ID] || existingMap[v.ID] != nil {
			continue
		}

		cat := "Videos"
		if strings.Contains(v.URL, "/shorts/") || (v.Duration > 0 && v.Duration <= 60) {
			cat = "Shorts"
		} else if strings.Contains(v.URL, "/live") {
			cat = "Live Streams"
		}

		downloadID := fmt.Sprintf("dl_%d_%s", time.Now().UnixNano(), v.ID)
		item := &db.DownloadItem{
			ID:           downloadID,
			URL:          v.URL,
			VideoID:      v.ID,
			Title:        v.Title,
			Channel:      catalog.Title,
			ChannelURL:   ch.URL,
			Duration:     v.Duration,
			ThumbnailURL: v.Thumbnail,
			Format:       format,
			Quality:      quality,
			IsAudioOnly:  prefs.DownloadAudioOnly,
			Category:     cat,
			Status:       "queued",
			CurrentStep:  "Queued from Channel Studio",
			CreatedAt:    time.Now(),
		}
		itemsToEnqueue = append(itemsToEnqueue, item)
	}

	// For any selected video IDs not returned in the first batch, fallback to direct watch URL
	for _, vID := range videoIDs {
		if existingMap[vID] != nil {
			continue
		}
		found := false
		for _, item := range itemsToEnqueue {
			if item.VideoID == vID {
				found = true
				break
			}
		}
		if !found {
			downloadID := fmt.Sprintf("dl_%d_%s", time.Now().UnixNano(), vID)
			item := &db.DownloadItem{
				ID:          downloadID,
				URL:         "https://www.youtube.com/watch?v=" + vID,
				VideoID:     vID,
				Title:       "YouTube Video [" + vID + "]",
				Channel:     ch.Title,
				ChannelURL:  ch.URL,
				Format:      format,
				Quality:     quality,
				IsAudioOnly: prefs.DownloadAudioOnly,
				Category:    "Videos",
				Status:      "queued",
				CurrentStep: "Queued from Channel Studio",
				CreatedAt:   time.Now(),
			}
			itemsToEnqueue = append(itemsToEnqueue, item)
		}
	}

	newCount, _ := qm.EnqueueBatchItems(itemsToEnqueue, nil)
	return newCount, nil
}

// splitStages parses "media,comments,html" into a slice
func splitStages(s string) []string {
	if s == "" {
		return nil
	}
	var result []string
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}

// joinStages combines a slice back into "media,comments,html"
func joinStages(stages []string) string {
	return strings.Join(stages, ",")
}

// countAllComments recursively counts top-level comments + all replies
func countAllComments(comments []*db.CommentItem) int {
	count := 0
	for _, c := range comments {
		count++
		count += countAllComments(c.Replies)
	}
	return count
}

func findFileWithExts(dir string, exts []string) string {
	if dir == "" {
		return ""
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		for _, ext := range exts {
			if strings.EqualFold(filepath.Ext(name), ext) {
				return filepath.Join(dir, name)
			}
		}
	}
	return ""
}

// ScanAndQueueMissingAssets inspects all "completed" downloads and re-queues any that are missing
// enabled assets (comments, avatars, companion audio, NFO, or HTML report)
// without re-downloading the media file. Videos that already have all assets are skipped.
func (qm *QueueManager) ScanAndQueueMissingAssets() (int, int, error) {
	completed, err := qm.db.GetAllDownloads("completed", "")
	if err != nil {
		return 0, 0, err
	}

	globalPrefs, _ := qm.db.GetPreferences()
	if globalPrefs == nil {
		globalPrefs = db.DefaultPreferences("./downloads")
	}

	totalScanned := len(completed)
	queuedCount := 0

	for _, item := range completed {
		if item.OutputDir == "" {
			continue
		}

		// Determine effective preferences for this item
		prefs := *globalPrefs
		if item.CustomOptions != "" {
			var override map[string]interface{}
			if err := json.Unmarshal([]byte(item.CustomOptions), &override); err == nil {
				if v, ok := override["download_comments"].(bool); ok {
					prefs.DownloadComments = v
				}
				if v, ok := override["extract_companion_audio"].(bool); ok {
					prefs.ExtractCompanionAudio = v
				}
				if v, ok := override["generate_nfo"].(bool); ok {
					prefs.GenerateNFO = v
				}
				if v, ok := override["generate_html_report"].(bool); ok {
					prefs.GenerateHTMLReport = v
				}
				if v, ok := override["download_commenter_avatars"].(bool); ok {
					prefs.DownloadCommenterAvatars = v
				}
			}
		}

		missingComments := false
		missingAudio := false
		missingNFO := false
		missingHTML := false

		// 1. Check Comments & Avatars
		if prefs.DownloadComments {
			hasDBComments := false
			if dbComments, err := qm.db.GetComments(item.VideoID); err == nil && len(dbComments) > 0 {
				hasDBComments = true
			}
			avatarsZip := filepath.Join(item.OutputDir, "avatars.zip")
			_, avatarErr := os.Stat(avatarsZip)

			if !hasDBComments || item.CommentsCount == 0 || (prefs.DownloadCommenterAvatars && avatarErr != nil) {
				missingComments = true
			}
		}

		// 2. Check Companion Audio
		if prefs.ExtractCompanionAudio && !item.IsAudioOnly {
			audioFile := findFileWithExts(item.OutputDir, []string{".mp3", ".m4a", ".opus", ".flac", ".wav", ".aac"})
			if audioFile == "" {
				missingAudio = true
			}
		}

		// 3. Check NFO
		if prefs.GenerateNFO {
			nfoFile := findFileWithExts(item.OutputDir, []string{".nfo"})
			if nfoFile == "" {
				missingNFO = true
			}
		}

		// 4. Check Offline HTML Report
		if prefs.GenerateHTMLReport {
			htmlFile := filepath.Join(item.OutputDir, "index.html")
			if _, err := os.Stat(htmlFile); err != nil {
				missingHTML = true
			}
		}

		// If any enabled asset is missing, re-queue ONLY the missing stages
		if missingComments || missingAudio || missingNFO || missingHTML {
			stages := splitStages(item.CompletedStages)
			hasMedia := false
			var newStages []string

			for _, s := range stages {
				if s == "media" {
					hasMedia = true
					newStages = append(newStages, "media")
				} else if s == "comments" && !missingComments {
					newStages = append(newStages, "comments")
				}
			}

			// Ensure media stage is preserved so we NEVER re-download video/audio streams
			if !hasMedia {
				newStages = append([]string{"media"}, newStages...)
			}

			item.CompletedStages = joinStages(newStages)
			item.Status = "queued"
			item.CurrentStep = "Queued to fetch missing assets..."
			item.ErrorMessage = ""
			_ = qm.db.UpdateDownload(item)
			Broadcaster.Broadcast("status_change", item)
			queuedCount++
			logger.Infof("[Queue] Re-queued %s (%q) to fetch missing assets (Comments: %t, Audio: %t, NFO: %t, HTML: %t)", item.VideoID, item.Title, missingComments, missingAudio, missingNFO, missingHTML)
		}
	}

	if queuedCount > 0 {
		qm.triggerWorker()
	}

	return totalScanned, queuedCount, nil
}
