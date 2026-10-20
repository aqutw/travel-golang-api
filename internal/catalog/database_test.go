package catalog

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDatabaseIsReadOnlyAndMissingFileIsNotCreated(t *testing.T) {
	c, err := New("../../data/travel.sqlite3", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if _, err = c.db.Exec("UPDATE products SET title='unexpected write'"); err == nil {
		t.Fatal("API connection must be read only")
	}
	for _, term := range []string{"' OR 1=1 --", "%", "_"} {
		items, err := c.Search(context.Background(), Search{Query: term, MaxPrice: 10000000, MinDays: 1, MaxDays: 30})
		if err != nil || len(items) != 0 {
			t.Fatalf("search must treat input as literal text: %q: %v", term, err)
		}
	}
	missing := filepath.Join(t.TempDir(), "missing.sqlite3")
	if db, err := New(missing, time.Now()); err == nil {
		db.Close()
		t.Fatal("missing database must fail")
	}
	if _, err = os.Stat(missing); !os.IsNotExist(err) {
		t.Fatal("opening missing database must not create a file")
	}
}
