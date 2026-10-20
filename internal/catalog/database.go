package catalog

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// Catalog reads the same SQLite file on every request; it does not cache entities.
type Catalog struct {
	db    *sql.DB
	Today time.Time
}

func New(path string, now time.Time) (*Catalog, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	u := url.URL{Scheme: "file", Path: abs}
	q := url.Values{"mode": {"ro"}, "_pragma": {"foreign_keys(1)", "busy_timeout(5000)"}}
	u.RawQuery = q.Encode()
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return nil, fmt.Errorf("open catalog: %w", err)
	}
	db.SetMaxOpenConns(4)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var version, appID int
	if err = db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err == nil {
		err = db.QueryRowContext(ctx, "PRAGMA application_id").Scan(&appID)
	}
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("read catalog %s: %w", abs, err)
	}
	if version != 1 || appID != 1414676822 {
		db.Close()
		return nil, fmt.Errorf("unsupported catalog schema: application_id=%d user_version=%d", appID, version)
	}
	zone := time.FixedZone("Asia/Taipei", 8*60*60)
	n := now.In(zone)
	return &Catalog{db: db, Today: time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, zone)}, nil
}

func (c *Catalog) Close() error                   { return c.db.Close() }
func (c *Catalog) Ping(ctx context.Context) error { return c.db.PingContext(ctx) }

func queryJSON[T any](ctx context.Context, c *Catalog, query string, args ...any) ([]T, error) {
	rows, err := c.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []T{}
	for rows.Next() {
		var raw string
		var item T
		if err = rows.Scan(&raw); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(raw), &item); err != nil {
			return nil, fmt.Errorf("decode catalog row: %w", err)
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
func oneJSON[T any](ctx context.Context, c *Catalog, query string, args ...any) (T, error) {
	var item T
	var raw string
	if err := c.db.QueryRowContext(ctx, query, args...).Scan(&raw); err != nil {
		return item, err
	}
	err := json.Unmarshal([]byte(raw), &item)
	return item, err
}

func (c *Catalog) Product(ctx context.Context, id string) (Product, error) {
	return oneJSON[Product](ctx, c, "SELECT body FROM product_details WHERE id=?", id)
}
func (c *Catalog) Destination(ctx context.Context, id string) (Destination, error) {
	return oneJSON[Destination](ctx, c, "SELECT body FROM destinations_json WHERE id=?", id)
}
func (c *Catalog) Collection(ctx context.Context, id string) (Collection, error) {
	return oneJSON[Collection](ctx, c, "SELECT body FROM collections_json WHERE id=?", id)
}
func (c *Catalog) Campaign(ctx context.Context, id string) (Campaign, error) {
	return oneJSON[Campaign](ctx, c, "SELECT body FROM campaigns_json WHERE id=?", id)
}
func (c *Catalog) Article(ctx context.Context, id string) (Article, error) {
	return oneJSON[Article](ctx, c, "SELECT body FROM articles_json WHERE id=?", id)
}
func (c *Catalog) Categories(ctx context.Context) ([]Category, error) {
	return queryJSON[Category](ctx, c, "SELECT body FROM categories_json ORDER BY position,id")
}
func (c *Catalog) Destinations(ctx context.Context) ([]Destination, error) {
	return queryJSON[Destination](ctx, c, "SELECT body FROM destinations_json ORDER BY position,id")
}
func (c *Catalog) Cards(ctx context.Context) ([]Card, error) {
	return queryJSON[Card](ctx, c, "SELECT body FROM product_cards ORDER BY position,id")
}
func (c *Catalog) Collections(ctx context.Context) ([]Collection, error) {
	return queryJSON[Collection](ctx, c, "SELECT body FROM collections_json ORDER BY position,id")
}
func (c *Catalog) Campaigns(ctx context.Context) ([]Campaign, error) {
	return queryJSON[Campaign](ctx, c, "SELECT body FROM campaigns_json ORDER BY position,id")
}
func (c *Catalog) Articles(ctx context.Context) ([]Article, error) {
	return queryJSON[Article](ctx, c, "SELECT body FROM articles_json ORDER BY position,id")
}
func (c *Catalog) FAQs(ctx context.Context) ([]FAQ, error) {
	return queryJSON[FAQ](ctx, c, "SELECT body FROM faqs_json ORDER BY position,id")
}
func (c *Catalog) Reviews(ctx context.Context, id string, rating int) ([]Review, error) {
	return queryJSON[Review](ctx, c, "SELECT body FROM reviews_json WHERE product_id=? AND (?=0 OR rating=?) ORDER BY position,id", id, rating, rating)
}
func (c *Catalog) Setting(ctx context.Context, key string, dst any) error {
	var raw string
	if err := c.db.QueryRowContext(ctx, "SELECT value FROM settings WHERE key=?", key).Scan(&raw); err != nil {
		return err
	}
	return json.Unmarshal([]byte(raw), dst)
}
func (c *Catalog) Media(ctx context.Context, path string) (string, []byte, error) {
	var mime string
	var content []byte
	err := c.db.QueryRowContext(ctx, "SELECT mime_type,content FROM media WHERE url=?", path).Scan(&mime, &content)
	return mime, content, err
}
func (c *Catalog) HasCategory(ctx context.Context, id string) (bool, error) {
	var exists bool
	err := c.db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM categories WHERE id=?)", id).Scan(&exists)
	return exists, err
}
func (c *Catalog) HasCountry(ctx context.Context, code string) (bool, error) {
	var exists bool
	err := c.db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM destinations WHERE country_code=?)", code).Scan(&exists)
	return exists, err
}
