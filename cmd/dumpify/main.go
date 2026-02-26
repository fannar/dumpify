package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"dumpify/internal/app"
	"dumpify/internal/db"
	"dumpify/internal/services"
	"dumpify/internal/services/spotify"
	"dumpify/internal/ui"
)

func main() {
	cfg, err := app.LoadConfig()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	if err := cfg.Validate(); err != nil {
		log.Fatalf("invalid config: %v", err)
	}

	store, err := db.Open(cfg.DBPath)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}

	renderer, err := ui.NewRenderer()
	if err != nil {
		log.Fatalf("init ui: %v", err)
	}

	providers := map[string]services.MusicService{}
	if cfg.SpotifyClientID != "" || cfg.SpotifyRedirectURI != "" {
		spotifySvc, err := spotify.New(spotify.Config{
			ClientID:     cfg.SpotifyClientID,
			ClientSecret: cfg.SpotifyClientSecret,
			RedirectURI:  cfg.SpotifyRedirectURI,
			Scopes:       cfg.SpotifyScopes,
			Market:       cfg.SpotifyMarket,
		})
		if err != nil {
			log.Fatalf("configure spotify provider: %v", err)
		}
		providers[spotifySvc.Name()] = spotifySvc
	} else {
		log.Printf("spotify provider not configured. Set SPOTIFY_CLIENT_ID and SPOTIFY_REDIRECT_URI (SPOTIFY_CLIENT_SECRET optional with PKCE)")
	}

	srv := app.NewServer(cfg, store, renderer, providers)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := app.Run(ctx, cfg, srv); err != nil {
		log.Fatalf("server exited with error: %v", err)
	}
}
