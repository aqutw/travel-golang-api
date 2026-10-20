package httpapi

import (
	"database/sql"
	"encoding/json"
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aqutw/travel-golang-api/internal/catalog"
)

func TestSQLiteChangesReachAPIWithoutRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "travel.sqlite3")
	seed, err := os.ReadFile("../../data/travel.sqlite3")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, seed, 0600); err != nil {
		t.Fatal(err)
	}
	c, err := catalog.New(path, time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	s := httptest.NewServer(New(c))
	t.Cleanup(s.Close)
	writer, err := sql.Open("sqlite", path+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { writer.Close() })
	get[catalog.Product](t, s, "/api/v1/products/stay-001") // Warm the existing connection pool.
	for _, statement := range []string{
		"UPDATE products SET title='SQLite 即時更新旅宿',tags='[]' WHERE id='stay-001'",
		"UPDATE options SET amount=1234 WHERE id='stay-001-opt-1'",
		"UPDATE reviews SET rating=1 WHERE id='stay-001-review-1'",
		`UPDATE settings SET value=json_set(value,'$.primary[0].label','資料庫導覽') WHERE key='navigation'`,
		`UPDATE media SET content=CAST('<svg xmlns="http://www.w3.org/2000/svg"><title>database image</title></svg>' AS BLOB) WHERE url='/images/stay.svg'`,
	} {
		if _, err = writer.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	p := get[catalog.Product](t, s, "/api/v1/products/stay-001").Data
	if p.Title != "SQLite 即時更新旅宿" || p.Price.Amount != 1234 || p.Stay.Rooms[0].Price.Amount != 1234 || p.Rating.Average != 3.8 {
		t.Fatalf("detail is not based on SQLite: %+v", p)
	}
	items := get[[]catalog.Card](t, s, "/api/v1/products?type=stay&min_price=1200&max_price=1300&q=sqlite").Data
	if len(items) != 1 || items[0].ID != p.ID || items[0].Title != p.Title {
		t.Fatal("SQL search did not use updated fields")
	}
	home := get[Home](t, s, "/api/v1/home").Data
	if home.Sections[0].Products[0].Title != p.Title || home.Navigation.Primary[0].Label != "資料庫導覽" {
		t.Fatal("home did not read updated database")
	}
	nav := get[Navigation](t, s, "/api/v1/navigation").Data
	if nav.Primary[0].Label != "資料庫導覽" {
		t.Fatal("navigation was cached")
	}
	quote := get[catalog.Availability](t, s, "/api/v1/products/stay-001/availability").Data
	if quote.Options[0].UnitPrice.Amount != 1234 {
		t.Fatal("quote did not read the option price")
	}
	res, err := s.Client().Get(s.URL + "/images/stay.svg")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if string(body) != `<svg xmlns="http://www.w3.org/2000/svg"><title>database image</title></svg>` {
		t.Fatal("image did not come from database")
	}
	if _, err = writer.Exec("INSERT INTO collection_products VALUES ('family','missing-product',999)"); err == nil {
		t.Fatal("foreign key must reject broken product references")
	}
	if err = c.Close(); err != nil {
		t.Fatal(err)
	}
	res, err = s.Client().Get(s.URL + "/api/v1/products/stay-001")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var failure ErrorEnvelope
	json.NewDecoder(res.Body).Decode(&failure)
	if res.StatusCode != 500 || failure.Error.Code != "database_error" {
		t.Fatal("database failures must not be reported as missing products")
	}
}
