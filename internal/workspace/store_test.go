package workspace

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
)

func TestMigrationAndReopenPreserveData(t *testing.T) {
	path := filepath.Join(t.TempDir(), "travel.sqlite3")
	raw, err := os.ReadFile("../../data/travel.sqlite3")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	// Recreate the previous schema using the same catalog and views.
	_, err = db.Exec("DROP TABLE document_products; DROP TABLE documents; DROP TABLE managed_reviews; DROP TABLE idempotency_keys; PRAGMA user_version=1;")
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	for i := 0; i < 2; i++ {
		store, err := New(path)
		if err != nil {
			t.Fatal(err)
		}
		var version int
		if err = store.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != 2 {
			t.Fatal("migration did not reach version 2", version, err)
		}
		var products int
		store.db.QueryRow("SELECT count(*) FROM products").Scan(&products)
		if products != 96 {
			t.Fatal("migration changed catalog")
		}
		store.Close()
	}
	missing := filepath.Join(t.TempDir(), "missing.sqlite3")
	if store, err := New(missing); err == nil {
		store.Close()
		t.Fatal("missing file must not be created")
	}
	if _, err = os.Stat(missing); !os.IsNotExist(err) {
		t.Fatal("created missing DB", err)
	}
}
