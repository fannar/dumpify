package services

import (
	"context"

	"dumpify/internal/domain"
)

type DumpProgress struct {
	TotalPlaylists        int
	CompletedPlaylists    int
	SkippedPlaylists      int
	CurrentPlaylist       string
	LastCompletedPlaylist string
	LastSkippedPlaylist   string
}

type MusicService interface {
	Name() string
	CallbackPath() string
	AuthURL(state string) string
	ExchangeCode(ctx context.Context, code, state string) (domain.AuthToken, error)
	CurrentUser(ctx context.Context, token domain.AuthToken) (domain.User, domain.AuthToken, error)
	ListPlaylists(ctx context.Context, token domain.AuthToken) ([]domain.Playlist, domain.AuthToken, error)
	DumpPlaylist(ctx context.Context, token domain.AuthToken, playlistID string) (domain.Playlist, domain.AuthToken, error)
	DumpPlaylists(ctx context.Context, token domain.AuthToken, onProgress func(DumpProgress)) (domain.PlaylistDump, domain.AuthToken, error)
}
