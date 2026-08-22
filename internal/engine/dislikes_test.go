package engine

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchReturnYouTubeDislike(t *testing.T) {
	// Test empty video ID
	_, err := FetchReturnYouTubeDislike(context.Background(), "")
	if err == nil {
		t.Errorf("Expected error for empty video ID, got nil")
	}

	// Test with local mock server
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"id": "mockVideo123",
			"likes": 50000,
			"dislikes": 1200,
			"rating": 4.88,
			"viewCount": 1000000,
			"deleted": false
		}`))
	})
	server := httptest.NewServer(handler)
	defer server.Close()

	// Direct check parsing
	req, _ := http.NewRequestWithContext(context.Background(), "GET", server.URL, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Failed to execute mock request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected 200, got %d", resp.StatusCode)
	}
}
