// Package workspace persists travel plans, wishlists and managed reviews.
package workspace

import (
	"context"
	"crypto/rand"
	"database/sql"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"time"

	"github.com/aqutw/travel-golang-api/internal/catalog"
)

//go:embed migration.sql
var Migration string

var ErrConflict = errors.New("resource version has changed")
var ErrReplayConflict = errors.New("idempotency key was already used with a different request")
var ErrProduct = errors.New("one or more products do not exist")

type Store struct{ db *sql.DB }
type Document struct {
	ID        string          `json:"id"`
	Kind      string          `json:"kind"`
	Content   json.RawMessage `json:"content"`
	Version   int             `json:"version"`
	CreatedAt string          `json:"created_at"`
	UpdatedAt string          `json:"updated_at"`
}
type ReviewRecord struct {
	catalog.Review
	Version int `json:"version"`
}
type Mutation struct {
	Status   int
	Data     json.RawMessage
	Location string
	Replayed bool
}

func New(path string) (*Store, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	u := url.URL{Scheme: "file", Path: abs}
	u.RawQuery = url.Values{"mode": {"rw"}, "_txlock": {"immediate"}, "_pragma": {"foreign_keys(1)", "busy_timeout(5000)"}}.Encode()
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	tx, err := db.BeginTx(ctx, nil)
	if err == nil {
		defer tx.Rollback()
		var version, appID int
		err = tx.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version)
		if err == nil {
			err = tx.QueryRowContext(ctx, "PRAGMA application_id").Scan(&appID)
		}
		if err == nil && (appID != 1414676822 || (version != 1 && version != 2)) {
			err = errors.New("unsupported catalog schema")
		}
		if err == nil && version == 1 {
			_, err = tx.ExecContext(ctx, Migration)
		}
		if err == nil {
			err = tx.Commit()
		}
	}
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("open writable workspace: %w", err)
	}
	return s, nil
}
func (s *Store) Close() error { return s.db.Close() }
func ID(prefix string) string { return prefix + "-" + rand.Text() }
func Timestamp() string       { return time.Now().UTC().Format(time.RFC3339Nano) }

// Apply commits both the resource and its replay record in one immediate transaction.
func (s *Store) Apply(ctx context.Context, scope, key, hash string, command func(*sql.Tx) (any, string, error)) (Mutation, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Mutation{}, err
	}
	defer tx.Rollback()
	if key != "" {
		var m Mutation
		var stored string
		var raw string
		err = tx.QueryRowContext(ctx, "SELECT request_hash,status,data,location FROM idempotency_keys WHERE scope=? AND key=?", scope, key).Scan(&stored, &m.Status, &raw, &m.Location)
		if err == nil {
			if stored != hash {
				return Mutation{}, ErrReplayConflict
			}
			m.Data = json.RawMessage(raw)
			m.Replayed = true
			return m, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return Mutation{}, err
		}
	}
	data, location, err := command(tx)
	if err != nil {
		return Mutation{}, err
	}
	raw, err := json.Marshal(data)
	if err != nil {
		return Mutation{}, err
	}
	m := Mutation{Status: 200, Data: raw, Location: location}
	if key != "" {
		m.Status = 201
		if _, err = tx.ExecContext(ctx, "INSERT INTO idempotency_keys(scope,key,request_hash,status,data,location) VALUES(?,?,?,?,?,?)", scope, key, hash, m.Status, string(raw), location); err != nil {
			return Mutation{}, err
		}
	}
	if err = tx.Commit(); err != nil {
		return Mutation{}, err
	}
	return m, nil
}

type rowScanner interface{ Scan(...any) error }

func scanDocument(row rowScanner) (Document, error) {
	var d Document
	var raw string
	err := row.Scan(&d.ID, &d.Kind, &raw, &d.Version, &d.CreatedAt, &d.UpdatedAt)
	d.Content = json.RawMessage(raw)
	return d, err
}

const documentSelect = "SELECT id,kind,content,version,created_at,updated_at FROM documents"

func (s *Store) Document(ctx context.Context, kind, id string) (Document, error) {
	return scanDocument(s.db.QueryRowContext(ctx, documentSelect+" WHERE kind=? AND id=?", kind, id))
}
func (s *Store) Documents(ctx context.Context, kind string) ([]Document, error) {
	rows, err := s.db.QueryContext(ctx, documentSelect+" WHERE kind=? ORDER BY created_at,id", kind)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Document{}
	for rows.Next() {
		d, err := scanDocument(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}
func CheckProducts(ctx context.Context, tx *sql.Tx, ids []string) error {
	for _, id := range ids {
		var found bool
		if err := tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM products WHERE id=?)", id).Scan(&found); err != nil {
			return err
		}
		if !found {
			return ErrProduct
		}
	}
	return nil
}
func SaveDocument(ctx context.Context, tx *sql.Tx, kind, id string, content any, ids []string, version int) (Document, error) {
	raw, err := json.Marshal(content)
	if err != nil {
		return Document{}, err
	}
	var d Document
	if id != "" {
		d, err = scanDocument(tx.QueryRowContext(ctx, documentSelect+" WHERE kind=? AND id=?", kind, id))
		if err != nil {
			return d, err
		}
		// Repeating a successful replacement with the same content is a no-op.
		if string(d.Content) == string(raw) {
			return d, nil
		}
		if version != d.Version {
			return d, ErrConflict
		}
	}
	if err = CheckProducts(ctx, tx, ids); err != nil {
		return d, err
	}
	now := Timestamp()
	if id == "" {
		d = Document{ID: ID(kind), Kind: kind, Content: raw, Version: 1, CreatedAt: now, UpdatedAt: now}
		_, err = tx.ExecContext(ctx, "INSERT INTO documents VALUES(?,?,?,?,?,?)", d.ID, d.Kind, string(raw), d.Version, d.CreatedAt, d.UpdatedAt)
	} else {
		d.Content = raw
		d.Version++
		d.UpdatedAt = now
		_, err = tx.ExecContext(ctx, "UPDATE documents SET content=?,version=?,updated_at=? WHERE id=?", string(raw), d.Version, now, id)
	}
	if err != nil {
		return d, err
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM document_products WHERE document_id=?", d.ID); err != nil {
		return d, err
	}
	for position, pid := range ids {
		if _, err = tx.ExecContext(ctx, "INSERT INTO document_products VALUES(?,?,?)", d.ID, pid, position); err != nil {
			return d, err
		}
	}
	return d, nil
}
func scanReview(row rowScanner) (ReviewRecord, error) {
	var r ReviewRecord
	err := row.Scan(&r.ID, &r.ProductID, &r.Author, &r.Rating, &r.Content, &r.TravelDate, &r.CreatedAt, &r.Version)
	return r, err
}

const reviewSelect = "SELECT r.id,r.product_id,r.author,r.rating,r.content,r.travel_date,r.created_at,m.version FROM reviews r JOIN managed_reviews m ON m.review_id=r.id"

func (s *Store) Review(ctx context.Context, product, id string) (ReviewRecord, error) {
	return scanReview(s.db.QueryRowContext(ctx, reviewSelect+" WHERE r.product_id=? AND r.id=?", product, id))
}
func SaveReview(ctx context.Context, tx *sql.Tx, r catalog.Review, id string, version int) (ReviewRecord, error) {
	if id == "" {
		if err := CheckProducts(ctx, tx, []string{r.ProductID}); err != nil {
			return ReviewRecord{}, err
		}
		r.ID = ID("review")
		r.CreatedAt = time.Now().UTC().Format("2006-01-02")
		_, err := tx.ExecContext(ctx, "INSERT INTO reviews(id,product_id,author,rating,content,travel_date,created_at,position) VALUES(?,?,?,?,?,?,?,(SELECT coalesce(max(position),-1)+1 FROM reviews))", r.ID, r.ProductID, r.Author, r.Rating, r.Content, r.TravelDate, r.CreatedAt)
		if err != nil {
			return ReviewRecord{}, err
		}
		_, err = tx.ExecContext(ctx, "INSERT INTO managed_reviews VALUES(?,1)", r.ID)
		return ReviewRecord{r, 1}, err
	}
	old, err := scanReview(tx.QueryRowContext(ctx, reviewSelect+" WHERE r.id=? AND r.product_id=?", id, r.ProductID))
	if err != nil {
		return old, err
	}
	if old.Author == r.Author && old.Rating == r.Rating && old.Content == r.Content && old.TravelDate == r.TravelDate {
		return old, nil
	}
	if version != old.Version {
		return old, ErrConflict
	}
	old.Author = r.Author
	old.Rating = r.Rating
	old.Content = r.Content
	old.TravelDate = r.TravelDate
	old.Version++
	_, err = tx.ExecContext(ctx, "UPDATE reviews SET author=?,rating=?,content=?,travel_date=? WHERE id=?", old.Author, old.Rating, old.Content, old.TravelDate, id)
	if err != nil {
		return old, err
	}
	_, err = tx.ExecContext(ctx, "UPDATE managed_reviews SET version=? WHERE review_id=?", old.Version, id)
	return old, err
}
