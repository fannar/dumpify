package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Config struct {
	Addr      string
	DataDir   string
	DBPath    string
	ExportDir string

	SpotifyClientID     string
	SpotifyClientSecret string
	SpotifyRedirectURI  string
	SpotifyScopes       []string
	SpotifyMarket       string
}

func LoadConfig() (Config, error) {
	if err := LoadEnvFiles(defaultEnvFiles(os.Getenv("DUMPIFY_ENV_FILES"))...); err != nil {
		return Config{}, err
	}

	dataDir := envOrDefault("DUMPIFY_DATA_DIR", "data")
	addr, err := resolveAddr()
	if err != nil {
		return Config{}, err
	}

	cfg := Config{
		Addr:                addr,
		DataDir:             dataDir,
		DBPath:              filepath.Join(dataDir, "db.json"),
		ExportDir:           filepath.Join(dataDir, "exports"),
		SpotifyClientID:     strings.TrimSpace(os.Getenv("SPOTIFY_CLIENT_ID")),
		SpotifyClientSecret: strings.TrimSpace(os.Getenv("SPOTIFY_CLIENT_SECRET")),
		SpotifyRedirectURI:  strings.TrimSpace(os.Getenv("SPOTIFY_REDIRECT_URI")),
		SpotifyScopes:       parseScopes(os.Getenv("SPOTIFY_SCOPES")),
		SpotifyMarket:       strings.ToUpper(strings.TrimSpace(os.Getenv("SPOTIFY_MARKET"))),
	}
	return cfg, nil
}

func (c Config) Validate() error {
	if c.Addr == "" {
		return fmt.Errorf("DUMPIFY_ADDR cannot be empty")
	}
	return nil
}

func envOrDefault(key, fallback string) string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	return v
}

func parseScopes(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.Fields(raw)
	if len(parts) == 0 {
		return nil
	}
	return parts
}

func defaultEnvFiles(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return []string{".env", ".env.local"}
	}
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		switch r {
		case ',', ';', ' ', '\t', '\n', '\r':
			return true
		default:
			return false
		}
	})

	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func resolveAddr() (string, error) {
	override := strings.TrimSpace(os.Getenv("DUMPIFY_ADDR"))
	if override != "" {
		return override, nil
	}

	rawPort := strings.TrimSpace(os.Getenv("PORT"))
	if rawPort == "" {
		rawPort = "8080"
	}

	port, err := strconv.Atoi(rawPort)
	if err != nil {
		return "", fmt.Errorf("invalid PORT value %q", rawPort)
	}
	if port < 1 || port > 65535 {
		return "", fmt.Errorf("PORT out of range: %d", port)
	}

	return fmt.Sprintf(":%d", port), nil
}
