package db

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func TestFTS5Support(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	_, err = db.Exec("CREATE VIRTUAL TABLE search_index USING fts5(video_id, title, author, text, source_type);")
	if err != nil {
		t.Fatalf("FTS5 table creation failed: %v", err)
	}

	_, err = db.Exec("INSERT INTO search_index (video_id, title, author, text, source_type) VALUES ('v1', 'Learn Go Programming', 'John Doe', 'This is a tutorial about concurrency and goroutines', 'comment');")
	if err != nil {
		t.Fatalf("Insert into FTS5 failed: %v", err)
	}

	var vid, title, author string
	err = db.QueryRow("SELECT video_id, title, author FROM search_index WHERE search_index MATCH 'concurrency'").Scan(&vid, &title, &author)
	if err != nil {
		t.Fatalf("FTS5 match query failed: %v", err)
	}

	if vid != "v1" {
		t.Errorf("Expected v1, got %s", vid)
	}
}
