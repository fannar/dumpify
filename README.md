# Dumpify

Dumpify is a small Go web app that connects to the Spotify Web API and exports all your playlists (including track rows) to JSON or CSV.

It is structured so new providers can be added later (Apple Music, Tidal, etc.) by implementing `internal/services.MusicService`.

## Project Structure

- `cmd/dumpify`: app entrypoint
- `internal/app`: HTTP server, routes, config
- `internal/services`: provider interface
- `internal/services/spotify`: Spotify OAuth + API implementation
- `internal/db`: persistence layer (JSON-backed store)
- `internal/exporter`: JSON/CSV writers
- `internal/ui`: HTML templates and renderer
- `data/`: generated database and export files

UI templates are embedded into the Go binary via `go:embed`, so deployment is a single executable.
On first run, Dumpify creates the `data/` directory automatically.

## Register a Spotify App

1. Open the Spotify Developer Dashboard: <https://developer.spotify.com/dashboard>.
2. Sign in, click **Create app**, and create a new app.
3. Open your app settings and copy the **Client ID**.
4. Add a redirect URI that exactly matches what you will set as `SPOTIFY_REDIRECT_URI`.
5. Open **User Management** in the app settings and add the Spotify account(s) that should be allowed to log in during development/testing.
6. Save the app settings.
7. Set your local environment values (for example in `.env.local`):

```bash
SPOTIFY_CLIENT_ID="your-client-id"
SPOTIFY_REDIRECT_URI="http://127.0.0.1:8080/api/spotify/callback"
# Optional with PKCE:
# SPOTIFY_CLIENT_SECRET="your-client-secret"
# SPOTIFY_MARKET="IS" # optional override
```

## Spotify Callback URI

Either callback style is fine, for example:

- `http://127.0.0.1:8080/api/spotify/callback`
- `http://127.0.0.1:8080/callback/spotify`

The important rule is: the URI in Spotify Dashboard must **exactly match** `SPOTIFY_REDIRECT_URI` in this app, including path and trailing slash behavior.

This implementation auto-registers whatever callback path is in `SPOTIFY_REDIRECT_URI`.

## Environment

Dumpify automatically loads environment files (if present) in this order:

1. `.env`
2. `.env.local`

Later files override earlier ones. Existing shell environment variables still win over file values.
You can override file list with `DUMPIFY_ENV_FILES` (comma/space/semicolon separated).

```bash
export SPOTIFY_CLIENT_ID="your-client-id"
export SPOTIFY_REDIRECT_URI="http://127.0.0.1:8080/api/spotify/callback"
export PORT="8080"
# Optional:
# export SPOTIFY_CLIENT_SECRET="optional-secret"
# export SPOTIFY_MARKET="IS" # optional override
# export DUMPIFY_DATA_DIR="data"
# export SPOTIFY_SCOPES="playlist-read-private playlist-read-collaborative user-read-private user-read-email"
# export DUMPIFY_ENV_FILES=".env .env.local"
# export DUMPIFY_ADDR=":8080" # explicit override (wins over PORT)
```

Spotify auth default uses Authorization Code with PKCE (`S256`), so `SPOTIFY_CLIENT_SECRET` is optional.
`SPOTIFY_MARKET` controls the market query for playlist items. If omitted, Dumpify uses the authenticated user's country from Spotify profile (`/me`). If neither is available, playlist export requests fail.

## Run

```bash
go run ./cmd/dumpify
```

Then open `http://localhost:8080`, connect Spotify, and export to JSON/CSV.

You can also load your playlist list in the UI and export a single selected playlist as JSON or CSV.

## Build

```bash
make build        # host platform
make build-macos  # darwin amd64 + arm64
make build-linux  # linux amd64 + arm64
make release      # all of the above
```

Binaries are written to `dist/`.

## Add Another Provider

1. Create a package (for example `internal/services/applemusic`).
2. Implement `internal/services.MusicService`.
3. Register it in `cmd/dumpify/main.go`.
4. Add connect button logic (already generic in UI via provider map).
