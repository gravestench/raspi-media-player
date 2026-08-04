package database

import (
	"context"
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
		if count != 10 {
			db.Close()
			t.Fatalf("migration count: got %d want 10", count)
		}
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestFailedMigrationRollsBackAndDatabaseRecovers(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "recovery.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	err = applyMigration(context.Background(), db, "broken.sql", `CREATE TABLE should_rollback (id INTEGER); INSERT INTO missing_table VALUES (1);`)
	if err == nil {
		t.Fatal("broken migration unexpectedly succeeded")
	}
	var tableCount, migrationCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'should_rollback'`).Scan(&tableCount); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE name = 'broken.sql'`).Scan(&migrationCount); err != nil {
		t.Fatal(err)
	}
	if tableCount != 0 || migrationCount != 0 {
		t.Fatalf("failed migration leaked state: table=%d record=%d", tableCount, migrationCount)
	}
	if err := applyMigration(context.Background(), db, "recovered.sql", `CREATE TABLE recovered (id INTEGER);`); err != nil {
		t.Fatalf("recovery migration failed: %v", err)
	}
}
