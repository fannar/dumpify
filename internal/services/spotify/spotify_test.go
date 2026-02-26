package spotify

import (
	"context"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

func TestNewWithoutClientSecretAndPKCEAuthURL(t *testing.T) {
	svc, err := New(Config{
		ClientID:    "client-id",
		RedirectURI: "https://example.com/api/spotify/callback",
	})
	if err != nil {
		t.Fatalf("new spotify service: %v", err)
	}

	authURL := svc.AuthURL("state123")
	u, err := url.Parse(authURL)
	if err != nil {
		t.Fatalf("parse auth url: %v", err)
	}
	q := u.Query()

	if q.Get("response_type") != "code" {
		t.Fatalf("unexpected response_type: %q", q.Get("response_type"))
	}
	if q.Get("client_id") != "client-id" {
		t.Fatalf("unexpected client_id: %q", q.Get("client_id"))
	}
	if q.Get("code_challenge_method") != "S256" {
		t.Fatalf("unexpected code_challenge_method: %q", q.Get("code_challenge_method"))
	}
	if q.Get("code_challenge") == "" {
		t.Fatalf("missing code_challenge")
	}
}

func TestExchangeCodeRequiresStateForPKCE(t *testing.T) {
	svc, err := New(Config{
		ClientID:    "client-id",
		RedirectURI: "https://example.com/api/spotify/callback",
	})
	if err != nil {
		t.Fatalf("new spotify service: %v", err)
	}

	if _, err := svc.ExchangeCode(context.Background(), "code123", ""); err == nil {
		t.Fatalf("expected missing-state error")
	}
}

func TestPlaylistMetadataEndpoint(t *testing.T) {
	u, err := url.Parse(playlistMetadataEndpoint("playlist123"))
	if err != nil {
		t.Fatalf("parse endpoint: %v", err)
	}

	if u.Path != "/v1/playlists/playlist123" {
		t.Fatalf("unexpected path: %q", u.Path)
	}
	if got := u.Query().Get("fields"); got == "" {
		t.Fatalf("expected fields query parameter")
	}
}

func TestPlaylistItemsEndpoint(t *testing.T) {
	u, err := url.Parse(playlistItemsEndpoint("playlist123", 100, "US"))
	if err != nil {
		t.Fatalf("parse endpoint: %v", err)
	}

	if u.Path != "/v1/playlists/playlist123/items" {
		t.Fatalf("unexpected path: %q", u.Path)
	}
	if got := u.Query().Get("limit"); got != strconv.Itoa(playlistItemsPageLimit) {
		t.Fatalf("unexpected limit: %q", got)
	}
	if got := u.Query().Get("offset"); got != "100" {
		t.Fatalf("unexpected offset: %q", got)
	}
	if got := u.Query().Get("market"); got != "US" {
		t.Fatalf("unexpected market: %q", got)
	}
	if got := u.Query().Get("fields"); got != "items(item(name,album(name),artists(name)))" {
		t.Fatalf("unexpected fields: %q", got)
	}
	if got := u.Query().Get("additional_types"); got != "track" {
		t.Fatalf("unexpected additional_types: %q", got)
	}
}

func TestEffectiveMarketConfiguredWins(t *testing.T) {
	svc, err := New(Config{
		ClientID:    "client-id",
		RedirectURI: "https://example.com/api/spotify/callback",
		Market:      "US",
	})
	if err != nil {
		t.Fatalf("new spotify service: %v", err)
	}
	got, err := svc.effectiveMarket("IS")
	if err != nil {
		t.Fatalf("effective market: %v", err)
	}
	if got != "US" {
		t.Fatalf("unexpected market: %q", got)
	}
}

func TestEffectiveMarketFallsBackToUserCountry(t *testing.T) {
	svc, err := New(Config{
		ClientID:    "client-id",
		RedirectURI: "https://example.com/api/spotify/callback",
	})
	if err != nil {
		t.Fatalf("new spotify service: %v", err)
	}
	got, err := svc.effectiveMarket("is")
	if err != nil {
		t.Fatalf("effective market: %v", err)
	}
	if got != "IS" {
		t.Fatalf("unexpected market: %q", got)
	}
}

func TestEffectiveMarketFailsWithoutConfiguredMarketOrCountry(t *testing.T) {
	svc, err := New(Config{
		ClientID:    "client-id",
		RedirectURI: "https://example.com/api/spotify/callback",
	})
	if err != nil {
		t.Fatalf("new spotify service: %v", err)
	}
	if _, err := svc.effectiveMarket(""); err == nil {
		t.Fatalf("expected market resolution error")
	}
}

func TestAuthURLAlwaysIncludesRequiredScopes(t *testing.T) {
	svc, err := New(Config{
		ClientID:    "client-id",
		RedirectURI: "https://example.com/api/spotify/callback",
		Scopes:      []string{"playlist-read-private"},
	})
	if err != nil {
		t.Fatalf("new spotify service: %v", err)
	}

	authURL := svc.AuthURL("state123")
	u, err := url.Parse(authURL)
	if err != nil {
		t.Fatalf("parse auth url: %v", err)
	}

	scopeSet := map[string]struct{}{}
	for _, scope := range strings.Fields(u.Query().Get("scope")) {
		scopeSet[scope] = struct{}{}
	}

	for _, required := range requiredScopes {
		if _, ok := scopeSet[required]; !ok {
			t.Fatalf("missing required scope %q in %q", required, u.Query().Get("scope"))
		}
	}
}
