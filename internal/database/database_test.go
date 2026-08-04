package database

import (
	"path/filepath"
	"testing"
)

func TestOpenAppliesMigrationsIdempotently(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.sqlite")
	for i := 0; i < 2; i++ {
		db, err := Open(path)
		if err != nil {
			t.Fatal(err)
		}
		var count int
		if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&count); err != nil {
			db.Close()
			t.Fatal(err)
		}
		if count != 3 {
			db.Close()
			t.Fatalf("migration count: got %d want 3", count)
		}
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
	}
}
