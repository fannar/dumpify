package db

import (
	"path/filepath"
	"testing"
	"time"

	"dumpify/internal/domain"
)

func TestUpsertAccountAndExports(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "db.json")
	store, err := Open(storePath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}

	token := domain.AuthToken{AccessToken: "a1", ExpiresAt: time.Now().UTC().Add(time.Hour)}
	acc, err := store.UpsertAccount("spotify", domain.User{ID: "u1", DisplayName: "User"}, token)
	if err != nil {
		t.Fatalf("upsert account: %v", err)
	}

	acc2, err := store.UpsertAccount("spotify", domain.User{ID: "u1", DisplayName: "User 2"}, token)
	if err != nil {
		t.Fatalf("upsert account second: %v", err)
	}
	if acc2.ID != acc.ID {
		t.Fatalf("expected same account id, got %d and %d", acc.ID, acc2.ID)
	}

	rec, err := store.CreateExport(acc.ID, "spotify", "json", "/tmp/a.json")
	if err != nil {
		t.Fatalf("create export: %v", err)
	}
	if rec.ID < 1 {
		t.Fatalf("expected export id > 0")
	}

	exports := store.ListExportsForAccount(acc.ID, 10)
	if len(exports) != 1 {
		t.Fatalf("expected 1 export, got %d", len(exports))
	}
}
