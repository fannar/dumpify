package spotify

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"dumpify/internal/domain"
	"dumpify/internal/services"
)

const (
	authBaseURL  = "https://accounts.spotify.com/authorize"
	tokenURL     = "https://accounts.spotify.com/api/token"
	apiBaseURL   = "https://api.spotify.com/v1"
	playlistsURL = apiBaseURL + "/me/playlists?limit=50"

	maxRateLimitRetries = 6
)

type Config struct {
	ClientID     string
	ClientSecret string
	RedirectURI  string
	Scopes       []string
}

type Service struct {
	cfg        Config
	httpClient *http.Client
	callback   string
	pkceSecret [32]byte
}

func New(cfg Config) (*Service, error) {
	if cfg.ClientID == "" || cfg.RedirectURI == "" {
		return nil, errors.New("spotify client id and redirect uri are required")
	}

	u, err := url.Parse(cfg.RedirectURI)
	if err != nil {
		return nil, fmt.Errorf("parse redirect uri: %w", err)
	}
	if u.Path == "" {
		return nil, errors.New("spotify redirect uri must include a callback path")
	}

	scopes := cfg.Scopes
	if len(scopes) == 0 {
		scopes = []string{"playlist-read-private", "playlist-read-collaborative"}
	}

	var pkceSecret [32]byte
	if _, err := rand.Read(pkceSecret[:]); err != nil {
		return nil, fmt.Errorf("create pkce secret: %w", err)
	}

	return &Service{
		cfg: Config{
			ClientID:     cfg.ClientID,
			ClientSecret: cfg.ClientSecret,
			RedirectURI:  cfg.RedirectURI,
			Scopes:       scopes,
		},
		httpClient: &http.Client{Timeout: 20 * time.Second},
		callback:   u.Path,
		pkceSecret: pkceSecret,
	}, nil
}

func (s *Service) Name() string {
	return "spotify"
}

func (s *Service) CallbackPath() string {
	return s.callback
}

func (s *Service) AuthURL(state string) string {
	verifier := s.codeVerifier(state)
	challenge := codeChallenge(verifier)

	q := url.Values{}
	q.Set("client_id", s.cfg.ClientID)
	q.Set("response_type", "code")
	q.Set("redirect_uri", s.cfg.RedirectURI)
	q.Set("scope", strings.Join(s.cfg.Scopes, " "))
	q.Set("state", state)
	q.Set("code_challenge_method", "S256")
	q.Set("code_challenge", challenge)
	return authBaseURL + "?" + q.Encode()
}

func (s *Service) ExchangeCode(ctx context.Context, code, state string) (domain.AuthToken, error) {
	if state == "" {
		return domain.AuthToken{}, errors.New("missing oauth state for pkce")
	}

	vals := url.Values{}
	vals.Set("grant_type", "authorization_code")
	vals.Set("code", code)
	vals.Set("redirect_uri", s.cfg.RedirectURI)
	vals.Set("client_id", s.cfg.ClientID)
	vals.Set("code_verifier", s.codeVerifier(state))
	if s.cfg.ClientSecret != "" {
		vals.Set("client_secret", s.cfg.ClientSecret)
	}
	return s.requestToken(ctx, vals, domain.AuthToken{})
}

func (s *Service) CurrentUser(ctx context.Context, token domain.AuthToken) (domain.User, domain.AuthToken, error) {
	fresh, err := s.ensureFreshToken(ctx, token)
	if err != nil {
		return domain.User{}, domain.AuthToken{}, err
	}

	var me struct {
		ID          string `json:"id"`
		DisplayName string `json:"display_name"`
		Email       string `json:"email"`
	}
	if err := s.getJSON(ctx, apiBaseURL+"/me", fresh.AccessToken, &me); err != nil {
		return domain.User{}, domain.AuthToken{}, err
	}

	return domain.User{ID: me.ID, DisplayName: me.DisplayName, Email: me.Email}, fresh, nil
}

func (s *Service) ListPlaylists(ctx context.Context, token domain.AuthToken) ([]domain.Playlist, domain.AuthToken, error) {
	fresh, err := s.ensureFreshToken(ctx, token)
	if err != nil {
		return nil, domain.AuthToken{}, err
	}

	playlists, err := s.fetchPlaylists(ctx, fresh.AccessToken)
	if err != nil {
		return nil, domain.AuthToken{}, err
	}

	return playlists, fresh, nil
}

func (s *Service) DumpPlaylist(ctx context.Context, token domain.AuthToken, playlistID string) (domain.Playlist, domain.AuthToken, error) {
	if strings.TrimSpace(playlistID) == "" {
		return domain.Playlist{}, domain.AuthToken{}, errors.New("playlist id is required")
	}

	fresh, err := s.ensureFreshToken(ctx, token)
	if err != nil {
		return domain.Playlist{}, domain.AuthToken{}, err
	}

	playlist, err := s.fetchPlaylist(ctx, fresh.AccessToken, strings.TrimSpace(playlistID))
	if err != nil {
		return domain.Playlist{}, domain.AuthToken{}, err
	}

	return playlist, fresh, nil
}

func (s *Service) DumpPlaylists(ctx context.Context, token domain.AuthToken, onProgress func(services.DumpProgress)) (domain.PlaylistDump, domain.AuthToken, error) {
	user, freshToken, err := s.CurrentUser(ctx, token)
	if err != nil {
		return domain.PlaylistDump{}, domain.AuthToken{}, err
	}

	playlists, err := s.fetchPlaylists(ctx, freshToken.AccessToken)
	if err != nil {
		return domain.PlaylistDump{}, domain.AuthToken{}, err
	}

	total := len(playlists)
	completed := 0
	skipped := 0
	emitProgress := func(current, lastCompleted, lastSkipped string) {
		if onProgress == nil {
			return
		}
		onProgress(services.DumpProgress{
			TotalPlaylists:        total,
			CompletedPlaylists:    completed,
			SkippedPlaylists:      skipped,
			CurrentPlaylist:       current,
			LastCompletedPlaylist: lastCompleted,
			LastSkippedPlaylist:   lastSkipped,
		})
	}
	emitProgress("", "", "")

	for i := range playlists {
		emitProgress(playlists[i].Name, "", "")
		fullPlaylist, err := s.fetchPlaylist(ctx, freshToken.AccessToken, playlists[i].ID)
		if err != nil {
			var apiErr *spotifyAPIError
			if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusForbidden {
				log.Printf("spotify: skipping playlist %q (%s): forbidden", playlists[i].Name, playlists[i].ID)
				skipped++
				emitProgress(playlists[i].Name, "", playlists[i].Name)
				continue
			}
			return domain.PlaylistDump{}, domain.AuthToken{}, err
		}
		playlists[i] = fullPlaylist
		completed++
		emitProgress("", fullPlaylist.Name, "")
	}

	dump := domain.PlaylistDump{
		Provider:   s.Name(),
		ExportedAt: time.Now().UTC(),
		User:       user,
		Playlists:  playlists,
	}
	return dump, freshToken, nil
}

func (s *Service) ensureFreshToken(ctx context.Context, token domain.AuthToken) (domain.AuthToken, error) {
	if token.AccessToken == "" {
		return domain.AuthToken{}, errors.New("missing access token")
	}

	if token.ExpiresAt.IsZero() {
		return token, nil
	}

	if time.Now().UTC().Add(30 * time.Second).Before(token.ExpiresAt) {
		return token, nil
	}
	if token.RefreshToken == "" {
		return token, nil
	}

	vals := url.Values{}
	vals.Set("grant_type", "refresh_token")
	vals.Set("refresh_token", token.RefreshToken)
	vals.Set("client_id", s.cfg.ClientID)
	if s.cfg.ClientSecret != "" {
		vals.Set("client_secret", s.cfg.ClientSecret)
	}
	return s.requestToken(ctx, vals, token)
}

func (s *Service) requestToken(ctx context.Context, vals url.Values, previous domain.AuthToken) (domain.AuthToken, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(vals.Encode()))
	if err != nil {
		return domain.AuthToken{}, fmt.Errorf("create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return domain.AuthToken{}, fmt.Errorf("token request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return domain.AuthToken{}, fmt.Errorf("read token response: %w", err)
	}

	if resp.StatusCode >= 300 {
		return domain.AuthToken{}, fmt.Errorf("spotify token error (%d): %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var tr struct {
		AccessToken  string `json:"access_token"`
		TokenType    string `json:"token_type"`
		Scope        string `json:"scope"`
		ExpiresIn    int    `json:"expires_in"`
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.Unmarshal(body, &tr); err != nil {
		return domain.AuthToken{}, fmt.Errorf("parse token response: %w", err)
	}

	if tr.AccessToken == "" {
		return domain.AuthToken{}, errors.New("spotify token response missing access_token")
	}

	refreshToken := tr.RefreshToken
	if refreshToken == "" {
		refreshToken = previous.RefreshToken
	}

	scope := tr.Scope
	if scope == "" {
		scope = previous.Scope
	}

	expiresAt := time.Time{}
	if tr.ExpiresIn > 0 {
		expiresAt = time.Now().UTC().Add(time.Duration(tr.ExpiresIn) * time.Second)
	}

	return domain.AuthToken{
		AccessToken:  tr.AccessToken,
		TokenType:    tr.TokenType,
		RefreshToken: refreshToken,
		Scope:        scope,
		ExpiresAt:    expiresAt,
	}, nil
}

func (s *Service) fetchPlaylists(ctx context.Context, accessToken string) ([]domain.Playlist, error) {
	var playlists []domain.Playlist
	endpoint := playlistsURL

	for endpoint != "" {
		var page struct {
			Items []struct {
				ID            string `json:"id"`
				Name          string `json:"name"`
				Description   string `json:"description"`
				Collaborative bool   `json:"collaborative"`
				Public        *bool  `json:"public"`
				SnapshotID    string `json:"snapshot_id"`
				URI           string `json:"uri"`
				Owner         struct {
					ID string `json:"id"`
				} `json:"owner"`
			} `json:"items"`
			Next string `json:"next"`
		}

		if err := s.getJSON(ctx, endpoint, accessToken, &page); err != nil {
			return nil, err
		}

		for _, item := range page.Items {
			pub := false
			if item.Public != nil {
				pub = *item.Public
			}
			playlists = append(playlists, domain.Playlist{
				ID:            item.ID,
				Name:          item.Name,
				Description:   item.Description,
				OwnerID:       item.Owner.ID,
				Public:        pub,
				Collaborative: item.Collaborative,
				SnapshotID:    item.SnapshotID,
				URI:           item.URI,
			})
		}

		endpoint = page.Next
	}

	return playlists, nil
}

func (s *Service) fetchPlaylist(ctx context.Context, accessToken, playlistID string) (domain.Playlist, error) {
	u, _ := url.Parse(apiBaseURL)
	u.Path = path.Join(u.Path, "playlists", playlistID)
	q := u.Query()
	q.Set("additional_types", "track")
	u.RawQuery = q.Encode()
	endpoint := u.String()

	var payload struct {
		ID            string `json:"id"`
		Name          string `json:"name"`
		Description   string `json:"description"`
		Collaborative bool   `json:"collaborative"`
		Public        *bool  `json:"public"`
		SnapshotID    string `json:"snapshot_id"`
		URI           string `json:"uri"`
		Owner         struct {
			ID string `json:"id"`
		} `json:"owner"`
		Items  *spotifyTrackPage `json:"items"`
		Tracks *spotifyTrackPage `json:"tracks"`
	}
	if err := s.getJSON(ctx, endpoint, accessToken, &payload); err != nil {
		return domain.Playlist{}, err
	}

	pub := false
	if payload.Public != nil {
		pub = *payload.Public
	}

	initialPage := firstTrackPage(payload.Items, payload.Tracks)

	playlist := domain.Playlist{
		ID:            payload.ID,
		Name:          payload.Name,
		Description:   payload.Description,
		OwnerID:       payload.Owner.ID,
		Public:        pub,
		Collaborative: payload.Collaborative,
		SnapshotID:    payload.SnapshotID,
		URI:           payload.URI,
		Tracks:        parseSpotifyTrackItems(initialPage.Items),
	}

	next := initialPage.Next
	for next != "" {
		page, err := s.fetchTrackPage(ctx, accessToken, next)
		if err != nil {
			return domain.Playlist{}, err
		}
		playlist.Tracks = append(playlist.Tracks, parseSpotifyTrackItems(page.Items)...)
		next = page.Next
	}

	return playlist, nil
}

func (s *Service) fetchTrackPage(ctx context.Context, accessToken, endpoint string) (spotifyTrackPage, error) {
	var page spotifyTrackPage
	if err := s.getJSON(ctx, endpoint, accessToken, &page); err != nil {
		return spotifyTrackPage{}, err
	}
	return page, nil
}

func firstTrackPage(items, tracks *spotifyTrackPage) spotifyTrackPage {
	if items != nil {
		return *items
	}
	if tracks != nil {
		return *tracks
	}
	return spotifyTrackPage{}
}

func parseSpotifyTrackItems(items []spotifyTrackItem) []domain.Track {
	out := make([]domain.Track, 0, len(items))
	for _, item := range items {
		if item.Track == nil {
			continue
		}

		artists := make([]string, 0, len(item.Track.Artists))
		for _, ar := range item.Track.Artists {
			artists = append(artists, ar.Name)
		}

		addedAt := time.Time{}
		if item.AddedAt != "" {
			if ts, err := time.Parse(time.RFC3339, item.AddedAt); err == nil {
				addedAt = ts
			}
		}

		out = append(out, domain.Track{
			ID:         item.Track.ID,
			Name:       item.Track.Name,
			Artists:    artists,
			Album:      item.Track.Album.Name,
			DurationMS: item.Track.DurationMS,
			URI:        item.Track.URI,
			AddedAt:    addedAt,
			AddedBy:    item.AddedBy.ID,
		})
	}
	return out
}

type spotifyTrackPage struct {
	Items []spotifyTrackItem `json:"items"`
	Next  string             `json:"next"`
}

type spotifyTrackItem struct {
	AddedAt string `json:"added_at"`
	AddedBy struct {
		ID string `json:"id"`
	} `json:"added_by"`
	Track *spotifyTrack `json:"track"`
}

type spotifyTrack struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	DurationMS int    `json:"duration_ms"`
	URI        string `json:"uri"`
	Album      struct {
		Name string `json:"name"`
	} `json:"album"`
	Artists []struct {
		Name string `json:"name"`
	} `json:"artists"`
}

func (s *Service) getJSON(ctx context.Context, endpoint, accessToken string, out any) error {
	for attempt := 0; attempt < maxRateLimitRetries; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return fmt.Errorf("create spotify request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+accessToken)
		req.Header.Set("Accept", "application/json")

		resp, err := s.httpClient.Do(req)
		if err != nil {
			return fmt.Errorf("spotify request failed: %w", err)
		}

		body, readErr := io.ReadAll(resp.Body)
		closeErr := resp.Body.Close()
		if readErr != nil {
			return fmt.Errorf("read spotify response: %w", readErr)
		}
		if closeErr != nil {
			return fmt.Errorf("close spotify response body: %w", closeErr)
		}

		if resp.StatusCode == http.StatusTooManyRequests {
			apiErr := parseSpotifyAPIError(endpoint, resp.StatusCode, body)
			if attempt == maxRateLimitRetries-1 {
				return apiErr
			}

			waitFor := retryAfterDuration(resp.Header.Get("Retry-After"), attempt)
			log.Printf("spotify: rate limited on %s; retrying in %s (attempt %d/%d)", endpoint, waitFor.Round(time.Millisecond), attempt+1, maxRateLimitRetries)
			if err := sleepWithContext(ctx, waitFor); err != nil {
				return fmt.Errorf("waiting after spotify 429: %w", err)
			}
			continue
		}

		if resp.StatusCode >= 300 {
			apiErr := parseSpotifyAPIError(endpoint, resp.StatusCode, body)
			return apiErr
		}

		if err := json.Unmarshal(body, out); err != nil {
			return fmt.Errorf("parse spotify response: %w", err)
		}
		return nil
	}
	return fmt.Errorf("spotify request exhausted retries")
}

type spotifyAPIError struct {
	Endpoint   string
	StatusCode int
	Message    string
}

func (e *spotifyAPIError) Error() string {
	return fmt.Sprintf("spotify api error (%d) on %s: %s", e.StatusCode, e.Endpoint, e.Message)
}

func parseSpotifyAPIError(endpoint string, statusCode int, body []byte) *spotifyAPIError {
	msg := strings.TrimSpace(string(body))
	var payload struct {
		Error struct {
			Status  int    `json:"status"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &payload); err == nil {
		if payload.Error.Message != "" {
			msg = payload.Error.Message
		}
	}
	if msg == "" {
		msg = http.StatusText(statusCode)
	}
	return &spotifyAPIError{
		Endpoint:   endpoint,
		StatusCode: statusCode,
		Message:    msg,
	}
}

func retryAfterDuration(header string, attempt int) time.Duration {
	header = strings.TrimSpace(header)
	if header != "" {
		if seconds, err := strconv.Atoi(header); err == nil && seconds > 0 {
			return time.Duration(seconds) * time.Second
		}
		if when, err := http.ParseTime(header); err == nil {
			if d := time.Until(when); d > 0 {
				return d
			}
		}
	}

	backoff := time.Duration(1<<attempt) * time.Second
	if backoff > 30*time.Second {
		backoff = 30 * time.Second
	}
	return backoff
}

func sleepWithContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (s *Service) codeVerifier(state string) string {
	mac := hmac.New(sha256.New, s.pkceSecret[:])
	_, _ = mac.Write([]byte(state))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func codeChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}
