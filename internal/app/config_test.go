package app

import "testing"

func TestResolveAddrDefaultPort(t *testing.T) {
	t.Setenv("DUMPIFY_ADDR", "")
	t.Setenv("PORT", "")

	addr, err := resolveAddr()
	if err != nil {
		t.Fatalf("resolve addr: %v", err)
	}
	if addr != ":8080" {
		t.Fatalf("addr=%q want %q", addr, ":8080")
	}
}

func TestResolveAddrFromPort(t *testing.T) {
	t.Setenv("DUMPIFY_ADDR", "")
	t.Setenv("PORT", "9090")

	addr, err := resolveAddr()
	if err != nil {
		t.Fatalf("resolve addr: %v", err)
	}
	if addr != ":9090" {
		t.Fatalf("addr=%q want %q", addr, ":9090")
	}
}

func TestResolveAddrOverride(t *testing.T) {
	t.Setenv("DUMPIFY_ADDR", "127.0.0.1:7777")
	t.Setenv("PORT", "9090")

	addr, err := resolveAddr()
	if err != nil {
		t.Fatalf("resolve addr: %v", err)
	}
	if addr != "127.0.0.1:7777" {
		t.Fatalf("addr=%q want %q", addr, "127.0.0.1:7777")
	}
}

func TestResolveAddrInvalidPort(t *testing.T) {
	t.Setenv("DUMPIFY_ADDR", "")
	t.Setenv("PORT", "not-a-number")

	if _, err := resolveAddr(); err == nil {
		t.Fatalf("expected error for invalid port")
	}
}
