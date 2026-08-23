package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

type DB struct {
	db *sql.DB
	mu sync.RWMutex
}

var GlobalDB *DB

func InitDB(dbPath string, defaultDownloadDir string) (*DB, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, fmt.Errorf("failed to create db directory: %w", err)
	}

	sqlDB, err := sql.Open("sqlite", dbPath+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite db: %w", err)
	}

	// Limit connection pool for SQLite concurrency
	sqlDB.SetMaxOpenConns(1)

	database := &DB{db: sqlDB}
	if err := database.migrate(defaultDownloadDir); err != nil {
		return nil, fmt.Errorf("failed to migrate db: %w", err)
	}

	GlobalDB = database
	return database, nil
}

func (d *DB) migrate(defaultDownloadDir string) error {
	schema := `
	CREATE TABLE IF NOT EXISTS settings (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL
	);

	CREATE TABLE IF NOT EXISTS downloads (
		id TEXT PRIMARY KEY,
		url TEXT NOT NULL,
		video_id TEXT NOT NULL,
		playlist_id TEXT,
		playlist_title TEXT,
		playlist_index INTEGER,
		playlist_total INTEGER,
		title TEXT NOT NULL,
		channel TEXT NOT NULL,
		channel_url TEXT,
		duration INTEGER NOT NULL DEFAULT 0,
		thumbnail_url TEXT,
		thumbnail_path TEXT,
		format TEXT NOT NULL,
		quality TEXT NOT NULL,
		is_audio_only INTEGER NOT NULL DEFAULT 0,
		status TEXT NOT NULL,
		progress REAL NOT NULL DEFAULT 0.0,
		speed TEXT,
		eta TEXT,
		total_bytes INTEGER NOT NULL DEFAULT 0,
		downloaded_bytes INTEGER NOT NULL DEFAULT 0,
		current_step TEXT,
		output_dir TEXT NOT NULL,
		media_file_path TEXT,
		html_file_path TEXT,
		metadata_file_path TEXT,
		error_message TEXT,
		created_at DATETIME NOT NULL,
		completed_at DATETIME,
		comments_count INTEGER NOT NULL DEFAULT 0,
		custom_options TEXT,
		completed_stages TEXT DEFAULT ''
	);

	CREATE INDEX IF NOT EXISTS idx_downloads_video_id ON downloads(video_id);
	CREATE INDEX IF NOT EXISTS idx_downloads_status ON downloads(status);
	CREATE INDEX IF NOT EXISTS idx_downloads_created_at ON downloads(created_at);

	CREATE TABLE IF NOT EXISTS comments (
		id TEXT NOT NULL,
		video_id TEXT NOT NULL,
		parent_id TEXT,
		author TEXT NOT NULL,
		author_id TEXT,
		author_url TEXT,
		author_thumbnail TEXT,
		author_avatar_local TEXT,
		text TEXT NOT NULL,
		like_count INTEGER NOT NULL DEFAULT 0,
		timestamp INTEGER NOT NULL DEFAULT 0,
		time_text TEXT,
		is_favorited INTEGER NOT NULL DEFAULT 0,
		is_creator INTEGER NOT NULL DEFAULT 0,
		is_verified INTEGER NOT NULL DEFAULT 0,
		replies_count INTEGER NOT NULL DEFAULT 0,
		PRIMARY KEY (id, video_id)
	);

	CREATE INDEX IF NOT EXISTS idx_comments_video_id ON comments(video_id);
	CREATE INDEX IF NOT EXISTS idx_comments_parent_id ON comments(parent_id);

	CREATE TABLE IF NOT EXISTS channels (
		id TEXT PRIMARY KEY,
		channel_id TEXT NOT NULL,
		title TEXT NOT NULL,
		handle TEXT,
		url TEXT NOT NULL,
		avatar_url TEXT,
		subscriber_count INTEGER NOT NULL DEFAULT 0,
		last_synced DATETIME,
		auto_download INTEGER NOT NULL DEFAULT 0,
		total_videos INTEGER NOT NULL DEFAULT 0,
		min_duration_sec INTEGER NOT NULL DEFAULT 0,
		exclude_shorts INTEGER NOT NULL DEFAULT 0,
		exclude_livestreams INTEGER NOT NULL DEFAULT 0,
		max_auto_sync_count INTEGER NOT NULL DEFAULT 0,
		created_at DATETIME NOT NULL
	);

	CREATE INDEX IF NOT EXISTS idx_channels_channel_id ON channels(channel_id);
	CREATE INDEX IF NOT EXISTS idx_channels_url ON channels(url);
	`

	_, err := d.db.Exec(schema)
	if err != nil {
		return err
	}

	// Migration: add completed_stages and category column if they don't exist (for existing databases)
	_, _ = d.db.Exec("ALTER TABLE downloads ADD COLUMN completed_stages TEXT DEFAULT ''")
	_, _ = d.db.Exec("ALTER TABLE downloads ADD COLUMN category TEXT DEFAULT ''")
	_, _ = d.db.Exec("ALTER TABLE channels ADD COLUMN min_duration_sec INTEGER NOT NULL DEFAULT 0")
	_, _ = d.db.Exec("ALTER TABLE channels ADD COLUMN exclude_shorts INTEGER NOT NULL DEFAULT 0")
	_, _ = d.db.Exec("ALTER TABLE channels ADD COLUMN exclude_livestreams INTEGER NOT NULL DEFAULT 0")
	_, _ = d.db.Exec("ALTER TABLE channels ADD COLUMN max_auto_sync_count INTEGER NOT NULL DEFAULT 0")

	// Initialize default preferences if not present
	var count int
	err = d.db.QueryRow("SELECT COUNT(*) FROM settings WHERE key = 'preferences'").Scan(&count)
	if err != nil || count == 0 {
		defPrefs := DefaultPreferences(defaultDownloadDir)
		data, _ := json.Marshal(defPrefs)
		_, _ = d.db.Exec("INSERT OR REPLACE INTO settings (key, value) VALUES ('preferences', ?)", string(data))
	}

	return nil
}

func (d *DB) Close() error {
	if d.db != nil {
		return d.db.Close()
	}
	return nil
}

// Preferences
func (d *DB) GetPreferences() (*UserPreferences, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var val string
	err := d.db.QueryRow("SELECT value FROM settings WHERE key = 'preferences'").Scan(&val)
	if err != nil {
		return DefaultPreferences("./downloads"), nil
	}

	var prefs UserPreferences
	if err := json.Unmarshal([]byte(val), &prefs); err != nil {
		return DefaultPreferences("./downloads"), nil
	}
	// Sanitize legacy database state where cookie_browser was "chrome" but cookie_source was not explicitly "browser"
	if prefs.CookieSource != "browser" && prefs.CookieSource != "file" {
		prefs.CookieSource = "none"
		if prefs.CookieBrowser == "chrome" {
			prefs.CookieBrowser = "none"
		}
	}
	return &prefs, nil
}

func (d *DB) SavePreferences(prefs *UserPreferences) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	data, err := json.Marshal(prefs)
	if err != nil {
		return err
	}

	_, err = d.db.Exec("INSERT OR REPLACE INTO settings (key, value) VALUES ('preferences', ?)", string(data))
	return err
}

func (d *DB) GetProfiles() ([]DownloadProfile, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	var val string
	err := d.db.QueryRow("SELECT value FROM settings WHERE key = 'profiles'").Scan(&val)
	if err == sql.ErrNoRows {
		return []DownloadProfile{}, nil
	}
	if err != nil {
		return nil, err
	}
	var profiles []DownloadProfile
	if err := json.Unmarshal([]byte(val), &profiles); err != nil {
		return nil, err
	}
	return profiles, nil
}

func (d *DB) SaveProfiles(profiles []DownloadProfile) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	data, err := json.Marshal(profiles)
	if err != nil {
		return err
	}
	_, err = d.db.Exec("INSERT OR REPLACE INTO settings (key, value) VALUES ('profiles', ?)", string(data))
	return err
}

// Downloads CRUD
func (d *DB) CreateDownload(item *DownloadItem) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	query := `
	INSERT OR REPLACE INTO downloads (
		id, url, video_id, playlist_id, playlist_title, playlist_index, playlist_total,
		title, channel, channel_url, duration, thumbnail_url, thumbnail_path,
		format, quality, is_audio_only, status, progress, speed, eta,
		total_bytes, downloaded_bytes, current_step, output_dir, media_file_path,
		html_file_path, metadata_file_path, error_message, created_at, completed_at,
		comments_count, custom_options, completed_stages
	) VALUES (
		?, ?, ?, ?, ?, ?, ?,
		?, ?, ?, ?, ?, ?,
		?, ?, ?, ?, ?, ?, ?,
		?, ?, ?, ?, ?,
		?, ?, ?, ?, ?,
		?, ?, ?
	)`

	var isAudioOnly int
	if item.IsAudioOnly {
		isAudioOnly = 1
	}

	_, err := d.db.Exec(query,
		item.ID, item.URL, item.VideoID, item.PlaylistID, item.PlaylistTitle, item.PlaylistIndex, item.PlaylistTotal,
		item.Title, item.Channel, item.ChannelURL, item.Duration, item.ThumbnailURL, item.ThumbnailPath,
		item.Format, item.Quality, isAudioOnly, item.Status, item.Progress, item.Speed, item.ETA,
		item.TotalBytes, item.DownloadedBytes, item.CurrentStep, item.OutputDir, item.MediaFilePath,
		item.HTMLFilePath, item.MetadataFilePath, item.ErrorMessage, item.CreatedAt, item.CompletedAt,
		item.CommentsCount, item.CustomOptions, item.CompletedStages,
	)
	return err
}

// CreateDownloadsBatch inserts or replaces multiple download items in a single atomic transaction
func (d *DB) CreateDownloadsBatch(items []*DownloadItem) error {
	if len(items) == 0 {
		return nil
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	query := `
	INSERT OR REPLACE INTO downloads (
		id, url, video_id, playlist_id, playlist_title, playlist_index, playlist_total,
		title, channel, channel_url, duration, thumbnail_url, thumbnail_path,
		format, quality, is_audio_only, status, progress, speed, eta,
		total_bytes, downloaded_bytes, current_step, output_dir, media_file_path,
		html_file_path, metadata_file_path, error_message, created_at, completed_at,
		comments_count, custom_options, completed_stages
	) VALUES (
		?, ?, ?, ?, ?, ?, ?,
		?, ?, ?, ?, ?, ?,
		?, ?, ?, ?, ?, ?, ?,
		?, ?, ?, ?, ?,
		?, ?, ?, ?, ?,
		?, ?, ?
	)`

	stmt, err := tx.Prepare(query)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, item := range items {
		var isAudioOnly int
		if item.IsAudioOnly {
			isAudioOnly = 1
		}

		_, err := stmt.Exec(
			item.ID, item.URL, item.VideoID, item.PlaylistID, item.PlaylistTitle, item.PlaylistIndex, item.PlaylistTotal,
			item.Title, item.Channel, item.ChannelURL, item.Duration, item.ThumbnailURL, item.ThumbnailPath,
			item.Format, item.Quality, isAudioOnly, item.Status, item.Progress, item.Speed, item.ETA,
			item.TotalBytes, item.DownloadedBytes, item.CurrentStep, item.OutputDir, item.MediaFilePath,
			item.HTMLFilePath, item.MetadataFilePath, item.ErrorMessage, item.CreatedAt, item.CompletedAt,
			item.CommentsCount, item.CustomOptions, item.CompletedStages,
		)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (d *DB) UpdateDownload(item *DownloadItem) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	query := `
	UPDATE downloads SET
		title = ?, channel = ?, channel_url = ?, duration = ?, thumbnail_url = ?, thumbnail_path = ?,
		format = ?, quality = ?, is_audio_only = ?, status = ?, progress = ?, speed = ?, eta = ?,
		total_bytes = ?, downloaded_bytes = ?, current_step = ?, output_dir = ?, media_file_path = ?,
		html_file_path = ?, metadata_file_path = ?, error_message = ?, completed_at = ?, comments_count = ?,
		completed_stages = ?
	WHERE id = ?`

	var isAudioOnly int
	if item.IsAudioOnly {
		isAudioOnly = 1
	}

	_, err := d.db.Exec(query,
		item.Title, item.Channel, item.ChannelURL, item.Duration, item.ThumbnailURL, item.ThumbnailPath,
		item.Format, item.Quality, isAudioOnly, item.Status, item.Progress, item.Speed, item.ETA,
		item.TotalBytes, item.DownloadedBytes, item.CurrentStep, item.OutputDir, item.MediaFilePath,
		item.HTMLFilePath, item.MetadataFilePath, item.ErrorMessage, item.CompletedAt, item.CommentsCount,
		item.CompletedStages,
		item.ID,
	)
	return err
}

func (d *DB) UpdateDownloadProgress(id string, progress float64, speed, eta string, downloaded, total int64, step string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	query := `
	UPDATE downloads SET
		progress = ?, speed = ?, eta = ?, downloaded_bytes = ?, total_bytes = ?, current_step = ?
	WHERE id = ?`

	_, err := d.db.Exec(query, progress, speed, eta, downloaded, total, step, id)
	return err
}

func (d *DB) UpdateDownloadStatus(id string, status, errorMsg string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	var completedAt *time.Time
	if status == "completed" {
		now := time.Now()
		completedAt = &now
	}

	query := `
	UPDATE downloads SET
		status = ?, error_message = ?, completed_at = COALESCE(?, completed_at)
	WHERE id = ?`

	_, err := d.db.Exec(query, status, errorMsg, completedAt, id)
	return err
}

func (d *DB) GetDownload(id string) (*DownloadItem, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	query := `
	SELECT
		id, url, video_id, COALESCE(playlist_id, ''), COALESCE(playlist_title, ''), COALESCE(playlist_index, 0), COALESCE(playlist_total, 0),
		title, channel, COALESCE(channel_url, ''), duration, COALESCE(thumbnail_url, ''), COALESCE(thumbnail_path, ''),
		format, quality, is_audio_only, status, progress, COALESCE(speed, ''), COALESCE(eta, ''),
		total_bytes, downloaded_bytes, COALESCE(current_step, ''), output_dir, COALESCE(media_file_path, ''),
		COALESCE(html_file_path, ''), COALESCE(metadata_file_path, ''), COALESCE(error_message, ''),
		created_at, completed_at, comments_count, COALESCE(custom_options, ''), COALESCE(completed_stages, '')
	FROM downloads WHERE id = ?`

	row := d.db.QueryRow(query, id)
	return scanDownloadItem(row)
}

func (d *DB) GetDownloadByVideoID(videoID string) (*DownloadItem, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	query := `
	SELECT
		id, url, video_id, COALESCE(playlist_id, ''), COALESCE(playlist_title, ''), COALESCE(playlist_index, 0), COALESCE(playlist_total, 0),
		title, channel, COALESCE(channel_url, ''), duration, COALESCE(thumbnail_url, ''), COALESCE(thumbnail_path, ''),
		format, quality, is_audio_only, status, progress, COALESCE(speed, ''), COALESCE(eta, ''),
		total_bytes, downloaded_bytes, COALESCE(current_step, ''), output_dir, COALESCE(media_file_path, ''),
		COALESCE(html_file_path, ''), COALESCE(metadata_file_path, ''), COALESCE(error_message, ''),
		created_at, completed_at, comments_count, COALESCE(custom_options, ''), COALESCE(completed_stages, '')
	FROM downloads WHERE video_id = ?
	ORDER BY created_at DESC LIMIT 1`

	row := d.db.QueryRow(query, videoID)
	item, err := scanDownloadItem(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return item, err
}

func (d *DB) FindDuplicate(videoID, url string) (*DownloadItem, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	query := `
	SELECT
		id, url, video_id, COALESCE(playlist_id, ''), COALESCE(playlist_title, ''), COALESCE(playlist_index, 0), COALESCE(playlist_total, 0),
		title, channel, COALESCE(channel_url, ''), duration, COALESCE(thumbnail_url, ''), COALESCE(thumbnail_path, ''),
		format, quality, is_audio_only, status, progress, COALESCE(speed, ''), COALESCE(eta, ''),
		total_bytes, downloaded_bytes, COALESCE(current_step, ''), output_dir, COALESCE(media_file_path, ''),
		COALESCE(html_file_path, ''), COALESCE(metadata_file_path, ''), COALESCE(error_message, ''),
		created_at, completed_at, comments_count, COALESCE(custom_options, ''), COALESCE(completed_stages, '')
	FROM downloads
	WHERE (video_id = ? AND video_id != '') OR url = ?
	ORDER BY created_at DESC LIMIT 1`

	row := d.db.QueryRow(query, videoID, url)
	item, err := scanDownloadItem(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return item, err
}

// FindDuplicatesBatch queries existing download items for a list of video IDs and/or URLs in chunks
func (d *DB) FindDuplicatesBatch(videoIDs []string, urls []string) (map[string]*DownloadItem, error) {
	result := make(map[string]*DownloadItem)
	if len(videoIDs) == 0 && len(urls) == 0 {
		return result, nil
	}

	d.mu.RLock()
	defer d.mu.RUnlock()

	chunkSize := 200

	// 1. Query by video_id
	var cleanVideoIDs []string
	for _, id := range videoIDs {
		if strings.TrimSpace(id) != "" {
			cleanVideoIDs = append(cleanVideoIDs, id)
		}
	}

	for i := 0; i < len(cleanVideoIDs); i += chunkSize {
		end := i + chunkSize
		if end > len(cleanVideoIDs) {
			end = len(cleanVideoIDs)
		}
		chunk := cleanVideoIDs[i:end]

		placeholders := make([]string, len(chunk))
		args := make([]interface{}, len(chunk))
		for j, id := range chunk {
			placeholders[j] = "?"
			args[j] = id
		}

		query := fmt.Sprintf(`
		SELECT
			id, url, video_id, COALESCE(playlist_id, ''), COALESCE(playlist_title, ''), COALESCE(playlist_index, 0), COALESCE(playlist_total, 0),
			title, channel, COALESCE(channel_url, ''), duration, COALESCE(thumbnail_url, ''), COALESCE(thumbnail_path, ''),
			format, quality, is_audio_only, status, progress, COALESCE(speed, ''), COALESCE(eta, ''),
			total_bytes, downloaded_bytes, COALESCE(current_step, ''), output_dir, COALESCE(media_file_path, ''),
			COALESCE(html_file_path, ''), COALESCE(metadata_file_path, ''), COALESCE(error_message, ''),
			created_at, completed_at, comments_count, COALESCE(custom_options, ''), COALESCE(completed_stages, '')
		FROM downloads
		WHERE video_id IN (%s)
		ORDER BY created_at DESC`, strings.Join(placeholders, ","))

		rows, err := d.db.Query(query, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			item, err := scanDownloadItemRow(rows)
			if err != nil {
				continue
			}
			if item.VideoID != "" && result[item.VideoID] == nil {
				result[item.VideoID] = item
			}
			if item.URL != "" && result[item.URL] == nil {
				result[item.URL] = item
			}
		}
		rows.Close()
	}

	// 2. Query by URL for items not yet matched
	var cleanURLs []string
	for _, u := range urls {
		if strings.TrimSpace(u) != "" && result[u] == nil {
			cleanURLs = append(cleanURLs, u)
		}
	}

	for i := 0; i < len(cleanURLs); i += chunkSize {
		end := i + chunkSize
		if end > len(cleanURLs) {
			end = len(cleanURLs)
		}
		chunk := cleanURLs[i:end]

		placeholders := make([]string, len(chunk))
		args := make([]interface{}, len(chunk))
		for j, u := range chunk {
			placeholders[j] = "?"
			args[j] = u
		}

		query := fmt.Sprintf(`
		SELECT
			id, url, video_id, COALESCE(playlist_id, ''), COALESCE(playlist_title, ''), COALESCE(playlist_index, 0), COALESCE(playlist_total, 0),
			title, channel, COALESCE(channel_url, ''), duration, COALESCE(thumbnail_url, ''), COALESCE(thumbnail_path, ''),
			format, quality, is_audio_only, status, progress, COALESCE(speed, ''), COALESCE(eta, ''),
			total_bytes, downloaded_bytes, COALESCE(current_step, ''), output_dir, COALESCE(media_file_path, ''),
			COALESCE(html_file_path, ''), COALESCE(metadata_file_path, ''), COALESCE(error_message, ''),
			created_at, completed_at, comments_count, COALESCE(custom_options, ''), COALESCE(completed_stages, '')
		FROM downloads
		WHERE url IN (%s)
		ORDER BY created_at DESC`, strings.Join(placeholders, ","))

		rows, err := d.db.Query(query, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			item, err := scanDownloadItemRow(rows)
			if err != nil {
				continue
			}
			if item.VideoID != "" && result[item.VideoID] == nil {
				result[item.VideoID] = item
			}
			if item.URL != "" && result[item.URL] == nil {
				result[item.URL] = item
			}
		}
		rows.Close()
	}

	return result, nil
}

// GetDownloadsPaged retrieves a paginated slice of downloads with total count
func (d *DB) GetDownloadsPaged(statusFilter string, search string, limit int, offset int) ([]*DownloadItem, int, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	baseWhere := " WHERE 1=1"
	var args []interface{}
	if statusFilter != "" && statusFilter != "all" {
		baseWhere += " AND status = ?"
		args = append(args, statusFilter)
	}

	if search != "" {
		baseWhere += " AND (title LIKE ? OR channel LIKE ? OR video_id LIKE ?)"
		pattern := "%" + search + "%"
		args = append(args, pattern, pattern, pattern)
	}

	// 1. Count total matching rows
	countQuery := "SELECT COUNT(*) FROM downloads" + baseWhere
	var total int
	if err := d.db.QueryRow(countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	// 2. Fetch page rows
	orderClause := " ORDER BY created_at DESC"
	if statusFilter == "queued" {
		orderClause = " ORDER BY created_at ASC"
	}
	query := `
	SELECT
		id, url, video_id, COALESCE(playlist_id, ''), COALESCE(playlist_title, ''), COALESCE(playlist_index, 0), COALESCE(playlist_total, 0),
		title, channel, COALESCE(channel_url, ''), duration, COALESCE(thumbnail_url, ''), COALESCE(thumbnail_path, ''),
		format, quality, is_audio_only, status, progress, COALESCE(speed, ''), COALESCE(eta, ''),
		total_bytes, downloaded_bytes, COALESCE(current_step, ''), output_dir, COALESCE(media_file_path, ''),
		COALESCE(html_file_path, ''), COALESCE(metadata_file_path, ''), COALESCE(error_message, ''),
		created_at, completed_at, comments_count, COALESCE(custom_options, ''), COALESCE(completed_stages, '')
	FROM downloads` + baseWhere + orderClause

	if limit > 0 {
		query += " LIMIT ? OFFSET ?"
		args = append(args, limit, offset)
	}

	rows, err := d.db.Query(query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var items []*DownloadItem
	for rows.Next() {
		item, err := scanDownloadItemRow(rows)
		if err != nil {
			continue
		}
		items = append(items, item)
	}

	if items == nil {
		items = []*DownloadItem{}
	}

	return items, total, nil
}

func (d *DB) GetAllDownloads(statusFilter string, search string) ([]*DownloadItem, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	query := `
	SELECT
		id, url, video_id, COALESCE(playlist_id, ''), COALESCE(playlist_title, ''), COALESCE(playlist_index, 0), COALESCE(playlist_total, 0),
		title, channel, COALESCE(channel_url, ''), duration, COALESCE(thumbnail_url, ''), COALESCE(thumbnail_path, ''),
		format, quality, is_audio_only, status, progress, COALESCE(speed, ''), COALESCE(eta, ''),
		total_bytes, downloaded_bytes, COALESCE(current_step, ''), output_dir, COALESCE(media_file_path, ''),
		COALESCE(html_file_path, ''), COALESCE(metadata_file_path, ''), COALESCE(error_message, ''),
		created_at, completed_at, comments_count, COALESCE(custom_options, ''), COALESCE(completed_stages, '')
	FROM downloads
	WHERE 1=1`

	var args []interface{}
	if statusFilter != "" && statusFilter != "all" {
		query += " AND status = ?"
		args = append(args, statusFilter)
	}

	if search != "" {
		query += " AND (title LIKE ? OR channel LIKE ? OR video_id LIKE ?)"
		pattern := "%" + search + "%"
		args = append(args, pattern, pattern, pattern)
	}

	if statusFilter == "queued" {
		query += " ORDER BY created_at ASC"
	} else {
		query += " ORDER BY created_at DESC"
	}

	rows, err := d.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []*DownloadItem
	for rows.Next() {
		item, err := scanDownloadItemRow(rows)
		if err != nil {
			continue
		}
		items = append(items, item)
	}
	return items, nil
}

func (d *DB) DeleteDownload(id string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	_, err := d.db.Exec("DELETE FROM downloads WHERE id = ?", id)
	return err
}

func (d *DB) ClearQueue() (int64, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	res, err := d.db.Exec("DELETE FROM downloads WHERE status IN ('queued', 'downloading', 'paused', 'failed')")
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// ReorderQueue updates created_at timestamps for a list of queued download IDs in sequential order
func (d *DB) ReorderQueue(orderedIDs []string) error {
	if len(orderedIDs) == 0 {
		return nil
	}
	d.mu.Lock()
	defer d.mu.Unlock()

	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare("UPDATE downloads SET created_at = ? WHERE id = ?")
	if err != nil {
		return err
	}
	defer stmt.Close()

	baseTime := time.Now().Add(-time.Duration(len(orderedIDs)) * time.Minute)
	for i, id := range orderedIDs {
		itemTime := baseTime.Add(time.Duration(i) * time.Second)
		if _, err := stmt.Exec(itemTime, id); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (d *DB) ResetInterruptedDownloads() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Reset downloading / paused downloads on app restart
	_, err := d.db.Exec("UPDATE downloads SET status = 'paused', current_step = 'Paused' WHERE status IN ('downloading', 'queued')")
	return err
}

func (d *DB) PauseAllDownloads() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	_, err := d.db.Exec("UPDATE downloads SET status = 'paused', current_step = 'Paused' WHERE status IN ('downloading', 'queued')")
	return err
}

func (d *DB) ResumeAllDownloads() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	_, err := d.db.Exec("UPDATE downloads SET status = 'queued', current_step = 'Waiting in queue...' WHERE status = 'paused'")
	return err
}

// Comments
func (d *DB) SaveComments(videoID string, comments []*CommentItem) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
	INSERT OR REPLACE INTO comments (
		id, video_id, parent_id, author, author_id, author_url,
		author_thumbnail, author_avatar_local, text, like_count,
		timestamp, time_text, is_favorited, is_creator, is_verified, replies_count
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	var insertComment func(c *CommentItem) error
	insertComment = func(c *CommentItem) error {
		isFav := 0
		if c.IsFavorited {
			isFav = 1
		}
		isCreator := 0
		if c.IsCreator {
			isCreator = 1
		}
		isVer := 0
		if c.IsVerified {
			isVer = 1
		}

		_, err := stmt.Exec(
			c.ID, videoID, c.ParentID, c.Author, c.AuthorID, c.AuthorURL,
			c.AuthorThumbnail, c.AuthorAvatarLocal, c.Text, c.LikeCount,
			c.Timestamp, c.TimeText, isFav, isCreator, isVer, len(c.Replies),
		)
		if err != nil {
			return err
		}

		for _, reply := range c.Replies {
			if err := insertComment(reply); err != nil {
				return err
			}
		}
		return nil
	}

	for _, c := range comments {
		if err := insertComment(c); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (d *DB) GetComments(videoID string) ([]*CommentItem, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	query := `
	SELECT
		id, video_id, COALESCE(parent_id, ''), author, COALESCE(author_id, ''), COALESCE(author_url, ''),
		COALESCE(author_thumbnail, ''), COALESCE(author_avatar_local, ''), text, like_count,
		timestamp, COALESCE(time_text, ''), is_favorited, is_creator, is_verified, replies_count
	FROM comments
	WHERE video_id = ?
	ORDER BY like_count DESC, timestamp DESC`

	rows, err := d.db.Query(query, videoID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	allComments := make(map[string]*CommentItem)
	var topLevel []*CommentItem

	for rows.Next() {
		var c CommentItem
		var isFav, isCreator, isVer int
		err := rows.Scan(
			&c.ID, &c.VideoID, &c.ParentID, &c.Author, &c.AuthorID, &c.AuthorURL,
			&c.AuthorThumbnail, &c.AuthorAvatarLocal, &c.Text, &c.LikeCount,
			&c.Timestamp, &c.TimeText, &isFav, &isCreator, &isVer, &c.RepliesCount,
		)
		if err != nil {
			continue
		}
		c.IsFavorited = isFav == 1
		c.IsCreator = isCreator == 1
		c.IsVerified = isVer == 1
		c.Replies = make([]*CommentItem, 0)
		allComments[c.ID] = &c
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for _, c := range allComments {
		if c.ParentID == "" {
			topLevel = append(topLevel, c)
		} else if parent, exists := allComments[c.ParentID]; exists {
			parent.Replies = append(parent.Replies, c)
		} else {
			topLevel = append(topLevel, c)
		}
	}

	return topLevel, nil
}

type rowScanner interface {
	Scan(dest ...interface{}) error
}

func scanDownloadItem(row rowScanner) (*DownloadItem, error) {
	var item DownloadItem
	var isAudioOnly int

	err := row.Scan(
		&item.ID, &item.URL, &item.VideoID, &item.PlaylistID, &item.PlaylistTitle, &item.PlaylistIndex, &item.PlaylistTotal,
		&item.Title, &item.Channel, &item.ChannelURL, &item.Duration, &item.ThumbnailURL, &item.ThumbnailPath,
		&item.Format, &item.Quality, &isAudioOnly, &item.Status, &item.Progress, &item.Speed, &item.ETA,
		&item.TotalBytes, &item.DownloadedBytes, &item.CurrentStep, &item.OutputDir, &item.MediaFilePath,
		&item.HTMLFilePath, &item.MetadataFilePath, &item.ErrorMessage,
		&item.CreatedAt, &item.CompletedAt, &item.CommentsCount, &item.CustomOptions, &item.CompletedStages,
	)
	if err != nil {
		return nil, err
	}
	item.IsAudioOnly = isAudioOnly == 1
	return &item, nil
}

func scanDownloadItemRow(rows *sql.Rows) (*DownloadItem, error) {
	return scanDownloadItem(rows)
}

// GetCommentCountForVideo returns the number of comments stored in the DB for a given video ID
func (d *DB) GetCommentCountForVideo(videoID string) (int, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var count int
	err := d.db.QueryRow("SELECT COUNT(*) FROM comments WHERE video_id = ?", videoID).Scan(&count)
	return count, err
}

// Channels CRUD
func (d *DB) SaveChannel(c *ChannelSubscription) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	query := `
	INSERT OR REPLACE INTO channels (
		id, channel_id, title, handle, url, avatar_url,
		subscriber_count, last_synced, auto_download, total_videos,
		min_duration_sec, exclude_shorts, exclude_livestreams, max_auto_sync_count, created_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	autoDl := 0
	if c.AutoDownload {
		autoDl = 1
	}
	exShorts := 0
	if c.ExcludeShorts {
		exShorts = 1
	}
	exLive := 0
	if c.ExcludeLiveStreams {
		exLive = 1
	}

	_, err := d.db.Exec(query,
		c.ID, c.ChannelID, c.Title, c.Handle, c.URL, c.AvatarURL,
		c.SubscriberCount, c.LastSynced, autoDl, c.TotalVideos,
		c.MinDurationSec, exShorts, exLive, c.MaxAutoSyncCount, c.CreatedAt,
	)
	return err
}

func (d *DB) GetChannel(id string) (*ChannelSubscription, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	query := `
	SELECT id, channel_id, title, COALESCE(handle, ''), url, COALESCE(avatar_url, ''),
	       subscriber_count, last_synced, auto_download, total_videos,
	       COALESCE(min_duration_sec, 0), COALESCE(exclude_shorts, 0), COALESCE(exclude_livestreams, 0), COALESCE(max_auto_sync_count, 0), created_at
	FROM channels WHERE id = ?`

	var c ChannelSubscription
	var autoDl, exShorts, exLive, maxCount int
	var minDur int64
	err := d.db.QueryRow(query, id).Scan(
		&c.ID, &c.ChannelID, &c.Title, &c.Handle, &c.URL, &c.AvatarURL,
		&c.SubscriberCount, &c.LastSynced, &autoDl, &c.TotalVideos,
		&minDur, &exShorts, &exLive, &maxCount, &c.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	c.AutoDownload = autoDl == 1
	c.MinDurationSec = minDur
	c.ExcludeShorts = exShorts == 1
	c.ExcludeLiveStreams = exLive == 1
	c.MaxAutoSyncCount = maxCount
	return &c, nil
}

func (d *DB) GetChannelByURLOrID(channelID, url string) (*ChannelSubscription, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	query := `
	SELECT id, channel_id, title, COALESCE(handle, ''), url, COALESCE(avatar_url, ''),
	       subscriber_count, last_synced, auto_download, total_videos,
	       COALESCE(min_duration_sec, 0), COALESCE(exclude_shorts, 0), COALESCE(exclude_livestreams, 0), COALESCE(max_auto_sync_count, 0), created_at
	FROM channels WHERE (channel_id = ? AND channel_id != '') OR url = ?
	LIMIT 1`

	var c ChannelSubscription
	var autoDl, exShorts, exLive, maxCount int
	var minDur int64
	err := d.db.QueryRow(query, channelID, url).Scan(
		&c.ID, &c.ChannelID, &c.Title, &c.Handle, &c.URL, &c.AvatarURL,
		&c.SubscriberCount, &c.LastSynced, &autoDl, &c.TotalVideos,
		&minDur, &exShorts, &exLive, &maxCount, &c.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	c.AutoDownload = autoDl == 1
	c.MinDurationSec = minDur
	c.ExcludeShorts = exShorts == 1
	c.ExcludeLiveStreams = exLive == 1
	c.MaxAutoSyncCount = maxCount
	return &c, nil
}

func (d *DB) GetAllChannels() ([]*ChannelSubscription, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	query := `
	SELECT id, channel_id, title, COALESCE(handle, ''), url, COALESCE(avatar_url, ''),
	       subscriber_count, last_synced, auto_download, total_videos,
	       COALESCE(min_duration_sec, 0), COALESCE(exclude_shorts, 0), COALESCE(exclude_livestreams, 0), COALESCE(max_auto_sync_count, 0), created_at
	FROM channels
	ORDER BY title ASC`

	rows, err := d.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []*ChannelSubscription
	for rows.Next() {
		var c ChannelSubscription
		var autoDl, exShorts, exLive, maxCount int
		var minDur int64
		err := rows.Scan(
			&c.ID, &c.ChannelID, &c.Title, &c.Handle, &c.URL, &c.AvatarURL,
			&c.SubscriberCount, &c.LastSynced, &autoDl, &c.TotalVideos,
			&minDur, &exShorts, &exLive, &maxCount, &c.CreatedAt,
		)
		if err != nil {
			continue
		}
		c.AutoDownload = autoDl == 1
		c.MinDurationSec = minDur
		c.ExcludeShorts = exShorts == 1
		c.ExcludeLiveStreams = exLive == 1
		c.MaxAutoSyncCount = maxCount
		list = append(list, &c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return list, nil
}

func (d *DB) UpdateChannelRules(id string, autoDownload bool, minDuration int64, excludeShorts, excludeLiveStreams bool, maxAutoSync int) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	autoDl := 0
	if autoDownload {
		autoDl = 1
	}
	exShorts := 0
	if excludeShorts {
		exShorts = 1
	}
	exLive := 0
	if excludeLiveStreams {
		exLive = 1
	}

	_, err := d.db.Exec(`
		UPDATE channels
		SET auto_download = ?,
		    min_duration_sec = ?,
		    exclude_shorts = ?,
		    exclude_livestreams = ?,
		    max_auto_sync_count = ?
		WHERE id = ?`,
		autoDl, minDuration, exShorts, exLive, maxAutoSync, id,
	)
	return err
}

func (d *DB) DeleteChannel(id string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	_, err := d.db.Exec("DELETE FROM channels WHERE id = ?", id)
	return err
}

func (d *DB) UpdateChannelSync(id string, totalVideos int) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	now := time.Now()
	_, err := d.db.Exec("UPDATE channels SET last_synced = ?, total_videos = ? WHERE id = ?", now, totalVideos, id)
	return err
}

func (d *DB) UpdateChannelFromCatalog(id, title, handle, avatarURL string, subs int64, totalVideos int) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	now := time.Now()
	_, err := d.db.Exec(`
		UPDATE channels
		SET title = CASE WHEN ? != '' THEN ? ELSE title END,
		    handle = CASE WHEN ? != '' THEN ? ELSE handle END,
		    avatar_url = CASE WHEN ? != '' THEN ? ELSE avatar_url END,
		    subscriber_count = CASE WHEN ? > 0 THEN ? ELSE subscriber_count END,
		    total_videos = CASE WHEN ? > 0 THEN ? ELSE total_videos END,
		    last_synced = ?
		WHERE id = ?`,
		title, title,
		handle, handle,
		avatarURL, avatarURL,
		subs, subs,
		totalVideos, totalVideos,
		now, id,
	)
	return err
}
