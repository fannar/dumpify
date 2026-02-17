package exporter

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"dumpify/internal/domain"
)

var safePart = regexp.MustCompile(`[^a-zA-Z0-9_-]+`)

func BuildFilename(provider, userID, format string, ts time.Time) string {
	provider = sanitizePart(provider)
	userID = sanitizePart(userID)
	if provider == "" {
		provider = "unknown"
	}
	if userID == "" {
		userID = "user"
	}
	return fmt.Sprintf("%s-%s-%s.%s", provider, userID, ts.UTC().Format("20060102-150405"), format)
}

func WriteJSON(path string, dump domain.PlaylistDump) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create export dir: %w", err)
	}

	b, err := json.MarshalIndent(dump, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal json: %w", err)
	}

	if err := os.WriteFile(path, b, 0o644); err != nil {
		return fmt.Errorf("write json: %w", err)
	}
	return nil
}

func WriteCSV(path string, dump domain.PlaylistDump) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create export dir: %w", err)
	}

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create csv: %w", err)
	}
	defer f.Close()

	w := csv.NewWriter(f)
	defer w.Flush()

	headers := []string{
		"provider",
		"exported_at",
		"user_id",
		"playlist_id",
		"playlist_name",
		"playlist_description",
		"playlist_owner",
		"playlist_public",
		"playlist_collaborative",
		"track_id",
		"track_name",
		"track_artists",
		"track_album",
		"track_duration_ms",
		"track_uri",
		"added_at",
		"added_by",
	}
	if err := w.Write(headers); err != nil {
		return fmt.Errorf("write csv headers: %w", err)
	}

	exportedAt := dump.ExportedAt.UTC().Format(time.RFC3339)
	for _, pl := range dump.Playlists {
		base := []string{
			dump.Provider,
			exportedAt,
			dump.User.ID,
			pl.ID,
			pl.Name,
			pl.Description,
			pl.OwnerID,
			fmt.Sprintf("%t", pl.Public),
			fmt.Sprintf("%t", pl.Collaborative),
		}

		if len(pl.Tracks) == 0 {
			row := append(base, "", "", "", "", "", "", "")
			if err := w.Write(row); err != nil {
				return fmt.Errorf("write csv row: %w", err)
			}
			continue
		}

		for _, tr := range pl.Tracks {
			addedAt := ""
			if !tr.AddedAt.IsZero() {
				addedAt = tr.AddedAt.UTC().Format(time.RFC3339)
			}

			row := append(base,
				tr.ID,
				tr.Name,
				strings.Join(tr.Artists, ", "),
				tr.Album,
				fmt.Sprintf("%d", tr.DurationMS),
				tr.URI,
				addedAt,
				tr.AddedBy,
			)
			if err := w.Write(row); err != nil {
				return fmt.Errorf("write csv row: %w", err)
			}
		}
	}

	if err := w.Error(); err != nil {
		return fmt.Errorf("flush csv: %w", err)
	}

	return nil
}

func sanitizePart(v string) string {
	v = strings.TrimSpace(v)
	v = safePart.ReplaceAllString(v, "-")
	v = strings.Trim(v, "-")
	return strings.ToLower(v)
}
