package spotify

import (
	"context"
	"net/url"
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
