package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadEnvFilesOrderAndPrecedence(t *testing.T) {
	t.Setenv("FROM_ENV", "keep")

	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	localPath := filepath.Join(dir, ".env.local")

	if err := os.WriteFile(envPath, []byte("\nA=1\nB=from-env\nURL=https://example.com/#keep\nCOMMENTED=value # trailing comment\nQUOTED=\"quoted value\"\nSINGLE='single value'\n"), 0o600); err != nil {
		t.Fatalf("write .env: %v", err)
	}
	if err := os.WriteFile(localPath, []byte("\nB=from-local\nQUOTED=\"override\"\nFROM_ENV=should-not-win\nNEW_LOCAL=42\n"), 0o600); err != nil {
		t.Fatalf("write .env.local: %v", err)
	}

	if err := LoadEnvFiles(envPath, localPath); err != nil {
		t.Fatalf("load env files: %v", err)
	}

	assertEnvEqual(t, "A", "1")
	assertEnvEqual(t, "B", "from-local")
	assertEnvEqual(t, "QUOTED", "override")
	assertEnvEqual(t, "SINGLE", "single value")
	assertEnvEqual(t, "COMMENTED", "value")
	assertEnvEqual(t, "URL", "https://example.com/#keep")
	assertEnvEqual(t, "NEW_LOCAL", "42")
	assertEnvEqual(t, "FROM_ENV", "keep")
}

func TestLoadEnvFilesInvalidLine(t *testing.T) {
	dir := t.TempDir()
	badPath := filepath.Join(dir, ".env.bad")
	if err := os.WriteFile(badPath, []byte("NOT_A_VALID_LINE\n"), 0o600); err != nil {
		t.Fatalf("write bad env file: %v", err)
	}

	if err := LoadEnvFiles(badPath); err == nil {
		t.Fatalf("expected parse error")
	}
}

func TestDefaultEnvFiles(t *testing.T) {
	files := defaultEnvFiles("")
	if len(files) != 2 || files[0] != ".env" || files[1] != ".env.local" {
		t.Fatalf("unexpected default files: %#v", files)
	}

	files = defaultEnvFiles("one.env, two.env three.env")
	if len(files) != 3 {
		t.Fatalf("unexpected custom files len: %d", len(files))
	}
}

func assertEnvEqual(t *testing.T, key, want string) {
	t.Helper()
	got := os.Getenv(key)
	if got != want {
		t.Fatalf("%s: got %q want %q", key, got, want)
	}
}
