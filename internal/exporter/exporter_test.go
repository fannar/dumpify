package exporter

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"dumpify/internal/domain"
)

func TestBuildFilename(t *testing.T) {
	ts := time.Date(2026, 2, 17, 10, 11, 12, 0, time.UTC)
	got := BuildFilename("Spotify", "User 123", "json", ts)
	want := "spotify-user-123-20260217-101112.json"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestWriteCSV(t *testing.T) {
	d := t.TempDir()
	file := filepath.Join(d, "out.csv")

	dump := domain.PlaylistDump{
		Provider:   "spotify",
		ExportedAt: time.Date(2026, 2, 17, 1, 2, 3, 0, time.UTC),
		User:       domain.User{ID: "u1"},
		Playlists: []domain.Playlist{
			{
				ID:          "p1",
				Name:        "Test",
				Description: "Desc",
				OwnerID:     "owner",
				Tracks: []domain.Track{{
					ID:         "t1",
					Name:       "Song",
					Artists:    []string{"A", "B"},
					Album:      "Album",
					DurationMS: 123,
					URI:        "spotify:track:t1",
					AddedAt:    time.Date(2026, 2, 17, 3, 4, 5, 0, time.UTC),
					AddedBy:    "u1",
				}},
			},
		},
	}

	if err := WriteCSV(file, dump); err != nil {
		t.Fatalf("write csv: %v", err)
	}

	b, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("read csv: %v", err)
	}
	text := string(b)
	if !strings.Contains(text, "playlist_id") {
		t.Fatalf("missing header in csv")
	}
	if !strings.Contains(text, "spotify:track:t1") {
		t.Fatalf("missing track row in csv")
	}
}
