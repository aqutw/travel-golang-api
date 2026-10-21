package httpapi

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aqutw/travel-golang-api/internal/catalog"
	"github.com/aqutw/travel-golang-api/internal/workspace"
	"github.com/gin-gonic/gin"
)

const testKey = "test-key-at-least-32-bytes-long-123456"

func writable(t *testing.T, custom ...func(*Options)) (*httptest.Server, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "travel.sqlite3")
	raw, err := os.ReadFile("../../data/travel.sqlite3")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	c, err := catalog.New(path, time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	store, err := workspace.New(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	opts := Options{Store: store, APIKey: testKey, RatePerMinute: 100000, Burst: 1000}
	for _, f := range custom {
		f(&opts)
	}
	s := httptest.NewServer(New(c, opts))
	t.Cleanup(s.Close)
	return s, path
}
func send(t *testing.T, s *httptest.Server, method, path, body, key string, headers ...map[string]string) (int, http.Header, []byte) {
	t.Helper()
	r, _ := http.NewRequest(method, s.URL+path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-API-Key", testKey)
	if key != "" {
		r.Header.Set("Idempotency-Key", key)
	}
	for _, set := range headers {
		for k, v := range set {
			r.Header.Set(k, v)
		}
	}
	response, err := s.Client().Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return response.StatusCode, response.Header, raw
}
func documentResponse(t *testing.T, raw []byte) workspace.Document {
	t.Helper()
	var e result[workspace.Document]
	if err := json.Unmarshal(raw, &e); err != nil {
		t.Fatal(err)
	}
	return e.Data
}
func requireStatus(t *testing.T, got, want int, body []byte) {
	t.Helper()
	if got != want {
		t.Fatalf("status=%d want=%d body=%s", got, want, body)
	}
}
func TestDocumentLifecycleIdempotencyAndPersistence(t *testing.T) {
	s, path := writable(t)
	body := `{"title":"週末收藏","description":"候選住宿","product_ids":["stay-001","stay-002"]}`
	status, headers, raw := send(t, s, "POST", "/api/v1/wishlists", body, "create-wishlist-001")
	requireStatus(t, status, 201, raw)
	created := documentResponse(t, raw)
	if created.Version != 1 || headers.Get("Location") != "/api/v1/wishlists/"+created.ID {
		t.Fatal("missing version or Location")
	}
	status, replayHeaders, replay := send(t, s, "POST", "/api/v1/wishlists", body, "create-wishlist-001")
	requireStatus(t, status, 201, replay)
	if !bytes.Equal(raw, replay) || replayHeaders.Get("Idempotency-Replayed") != "true" {
		t.Fatal("retry created a different result")
	}
	status, _, raw = send(t, s, "POST", "/api/v1/wishlists", strings.Replace(body, "週末收藏", "不同內容", 1), "create-wishlist-001")
	requireStatus(t, status, 409, raw)
	location := headers.Get("Location")
	put := `{"title":"新收藏","description":"","product_ids":["stay-003"],"version":1}`
	status, _, raw = send(t, s, "PUT", location, put, "")
	requireStatus(t, status, 200, raw)
	updated := documentResponse(t, raw)
	if updated.Version != 2 {
		t.Fatal("version was not incremented")
	}
	status, _, again := send(t, s, "PUT", location, put, "")
	requireStatus(t, status, 200, again)
	if !bytes.Equal(raw, again) {
		t.Fatal("PUT retry changed timestamps or version")
	}
	status, _, raw = send(t, s, "PUT", location, strings.Replace(put, "新收藏", "stale", 1), "")
	requireStatus(t, status, 409, raw)
	status, _, raw = send(t, s, "GET", location, "", "")
	requireStatus(t, status, 200, raw)
	// Reopen the store to prove this data survived outside the handler and pool.
	reopened, err := workspace.New(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	stored, err := reopened.Document(t.Context(), "wishlist", created.ID)
	if err != nil || stored.Version != 2 {
		t.Fatal("not persisted", err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var count int
	if err = db.QueryRow("SELECT count(*) FROM document_products WHERE document_id=?", created.ID).Scan(&count); err != nil || count != 1 {
		t.Fatal("replacement left old product references", err, count)
	}
}
func TestTripNestedBindingAndRollback(t *testing.T) {
	s, path := writable(t)
	body := `{"title":"台北兩日","start_date":"2026-11-01","end_date":"2026-11-03","adults":2,"children":0,"items":[{"product_id":"stay-001","day":1,"notes":"入住"}],"notes":"","version":0}`
	status, h, raw := send(t, s, "POST", "/api/v1/trips", body, "trip-create-0001")
	requireStatus(t, status, 201, raw)
	put := strings.Replace(strings.Replace(body, `"version":0`, `"version":1`, 1), "台北兩日", "更新行程", 1)
	status, _, raw = send(t, s, "PUT", h.Get("Location"), put, "")
	requireStatus(t, status, 200, raw)
	for _, invalid := range []string{strings.Replace(body, `"day":1`, `"day":3`, 1), strings.Replace(body, "2026-11-03", "2026-11-01", 1), strings.Replace(body, `"adults":2`, `"adults":0`, 1), strings.Replace(body, `"day":1`, `"day":0`, 1), strings.Replace(body, "stay-001", "missing", 1)} {
		status, _, raw = send(t, s, "POST", "/api/v1/trips", invalid, "trip-invalid-0001")
		requireStatus(t, status, 400, raw)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var n int
	db.QueryRow("SELECT count(*) FROM documents WHERE kind='trip'").Scan(&n)
	if n != 1 {
		t.Fatal("failed writes leaked documents")
	}
	db.QueryRow("SELECT count(*) FROM idempotency_keys WHERE key='trip-invalid-0001'").Scan(&n)
	if n != 0 {
		t.Fatal("failed writes consumed idempotency key")
	}
}
func TestReviewsChangeRatingAndProtectSeedReviews(t *testing.T) {
	s, _ := writable(t)
	body := `{"author":"測試旅人","rating":1,"content":"交通便利","travel_date":"2026-10-01"}`
	status, h, raw := send(t, s, "POST", "/api/v1/products/stay-001/reviews", body, "review-create-0001")
	requireStatus(t, status, 201, raw)
	product := get[catalog.Product](t, s, "/api/v1/products/stay-001").Data
	if product.Rating.Count != 6 || product.Rating.Average != 4 {
		t.Fatalf("ratings not recalculated: %+v", product.Rating)
	}
	put := strings.Replace(body, `"rating":1`, `"rating":5`, 1)
	put = strings.TrimSuffix(put, "}") + `,"version":1}`
	status, _, raw = send(t, s, "PUT", h.Get("Location"), put, "")
	requireStatus(t, status, 200, raw)
	status, _, raw = send(t, s, "GET", h.Get("Location"), "", "")
	requireStatus(t, status, 200, raw)
	product = get[catalog.Product](t, s, "/api/v1/products/stay-001").Data
	if product.Rating.Average != 4.7 {
		t.Fatal("review update not in rating")
	}
	status, _, raw = send(t, s, "PUT", "/api/v1/products/stay-001/reviews/stay-001-review-1", put, "")
	requireStatus(t, status, 404, raw)
	status, _, raw = send(t, s, "POST", "/api/v1/products/missing/reviews", body, "review-missing-001")
	requireStatus(t, status, 404, raw)
}
func TestQuoteMatchesGET(t *testing.T) {
	s, _ := writable(t)
	body := `{"start_date":"2026-11-01","end_date":"2026-11-03","adults":2,"children":0,"quantity":1}`
	status, _, raw := send(t, s, "POST", "/api/v1/products/stay-001/quotes", body, "", map[string]string{"X-API-Key": ""})
	requireStatus(t, status, 200, raw)
	var quote result[catalog.Availability]
	json.Unmarshal(raw, &quote)
	expected := get[catalog.Availability](t, s, "/api/v1/products/stay-001/availability?start_date=2026-11-01&end_date=2026-11-03&adults=2&children=0&quantity=1")
	a, _ := json.Marshal(quote)
	b, _ := json.Marshal(expected)
	if !bytes.Equal(a, b) {
		t.Fatal("quote differs from GET availability")
	}
}
func TestStrictBodyValidationAndAuthentication(t *testing.T) {
	s, _ := writable(t)
	for _, body := range []string{`{}`, `null`, `[]`, `{"title":"   ","product_ids":[]}`, `{"title":"a","product_ids":["stay-001","stay-001"]}`, `{"title":"a","product_ids":[],"extra":true}`, `{"title":"a","product_ids":[]} {}`, `{"title":1,"product_ids":[]}`, `{"title":"a","product_ids":[],"version":1}`} {
		status, _, raw := send(t, s, "POST", "/api/v1/wishlists", body, "invalid-body-0001")
		requireStatus(t, status, 400, raw)
		var err ErrorEnvelope
		json.Unmarshal(raw, &err)
		if err.Error.Code != "invalid_body" || len(err.Error.Fields) == 0 {
			t.Fatalf("missing field errors %s", raw)
		}
	}
	status, _, raw := send(t, s, "POST", "/api/v1/wishlists", `{"title":"a","product_ids":[]}`, "")
	requireStatus(t, status, 400, raw)
	status, _, raw = send(t, s, "POST", "/api/v1/wishlists", `{}`, "valid-key-0001", map[string]string{"Content-Type": "text/plain"})
	requireStatus(t, status, 415, raw)
	for _, path := range []string{"/api/v1/wishlists", "/api/v1/trips", "/api/v1/products/stay-001/reviews/review-missing"} {
		status, _, raw = send(t, s, "GET", path, "", "", map[string]string{"X-API-Key": "wrong"})
		requireStatus(t, status, 401, raw)
	}
	status, _, raw = send(t, s, "POST", "/api/v1/wishlists", `{}`, "valid-key-0001", map[string]string{"X-API-Key": ""})
	requireStatus(t, status, 401, raw)
	disabled := server(t)
	status, _, raw = send(t, disabled, "POST", "/api/v1/wishlists", `{}`, "valid-key-0001")
	requireStatus(t, status, 503, raw)
}
func TestBodyLimitAndChunkedBody(t *testing.T) {
	s, _ := writable(t)
	body := `{"title":"` + strings.Repeat("a", int(MaxBodyBytes)) + `","product_ids":[]}`
	status, _, raw := send(t, s, "POST", "/api/v1/wishlists", body, "oversized-0001")
	requireStatus(t, status, 413, raw)
	req, _ := http.NewRequest("POST", s.URL+"/api/v1/wishlists", io.NopCloser(strings.NewReader(body)))
	req.ContentLength = -1
	req.Header.Set("X-API-Key", testKey)
	req.Header.Set("Content-Type", "application/json")
	res, err := s.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != 413 {
		t.Fatal("chunked limit bypassed", res.StatusCode)
	}
}
func TestMiddlewareCORSIDsRecoveryAndLogs(t *testing.T) {
	var logs bytes.Buffer
	s, _ := writable(t, func(o *Options) {
		o.AllowedOrigins = []string{"http://localhost:5173"}
		o.Logger = slog.New(slog.NewJSONHandler(&logs, nil))
	})
	status, h, raw := send(t, s, "GET", "/healthz?token=do-not-log", "", "", map[string]string{"Origin": "http://localhost:5173", "X-Request-ID": "request-test-1234"})
	requireStatus(t, status, 400, raw)
	if h.Get("X-Request-ID") != "request-test-1234" || h.Get("Vary") != "Origin" {
		t.Fatal("missing request tracking / CORS vary")
	}
	if strings.Contains(logs.String(), "do-not-log") || strings.Contains(logs.String(), testKey) {
		t.Fatal("logs leaked input")
	}
	status, h, raw = send(t, s, "OPTIONS", "/api/v1/trips", "", "", map[string]string{"Origin": "http://localhost:5173", "X-API-Key": ""})
	requireStatus(t, status, 204, raw)
	if !strings.Contains(h.Get("Access-Control-Allow-Methods"), "PUT") {
		t.Fatal("PUT preflight not allowed")
	}
	status, _, raw = send(t, s, "GET", "/healthz", "", "", map[string]string{"Origin": "https://blocked.example"})
	requireStatus(t, status, 403, raw)
	// Exercise the same installed recovery middleware, including the error envelope.
	router := gin.New()
	router.Use(requestID(), recovery(slog.New(slog.NewTextHandler(io.Discard, nil))))
	router.GET("/panic", func(c *gin.Context) { panic("secret panic body") })
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest("GET", "/panic", nil))
	if rec.Code != 500 || strings.Contains(rec.Body.String(), "secret") {
		t.Fatal("recovery failed", rec.Body.String())
	}
}
func TestRateLimitUsesPeerIPAndReturnsRetryAfter(t *testing.T) {
	s, _ := writable(t, func(o *Options) { o.Burst = 1; o.RatePerMinute = 1 })
	body := `{"start_date":"2026-11-01","end_date":"2026-11-03","adults":2,"children":0,"quantity":1}`
	status, _, raw := send(t, s, "POST", "/api/v1/products/stay-001/quotes", body, "")
	requireStatus(t, status, 200, raw)
	status, h, raw := send(t, s, "POST", "/api/v1/products/stay-001/quotes", body, "", map[string]string{"X-Forwarded-For": "192.0.2.1"})
	requireStatus(t, status, 429, raw)
	if h.Get("Retry-After") == "" {
		t.Fatal("missing Retry-After")
	}
	get[catalog.Product](t, s, "/api/v1/products/stay-001")
}
func TestConcurrentRetriesCreateOneRecord(t *testing.T) {
	s, _ := writable(t)
	var wg sync.WaitGroup
	responses := make(chan string, 12)
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			status, _, raw := send(t, s, "POST", "/api/v1/wishlists", `{"title":"one","product_ids":["stay-001"]}`, "same-request-001")
			responses <- fmt.Sprintf("%d %s", status, raw)
		}()
	}
	wg.Wait()
	close(responses)
	first := ""
	for response := range responses {
		if first == "" {
			first = response
		}
		if response != first || !strings.HasPrefix(response, "201 ") {
			t.Fatal("concurrent replay differed", response)
		}
	}
	status, _, raw := send(t, s, "GET", "/api/v1/wishlists", "", "")
	requireStatus(t, status, 200, raw)
	var result result[[]workspace.Document]
	json.Unmarshal(raw, &result)
	if len(result.Data) != 1 {
		t.Fatal("duplicate records")
	}
}

func TestGinRoutesMatchOpenAPI(t *testing.T) {
	c, err := catalog.New("../../data/travel.sqlite3", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	engine := New(c)
	b, err := os.ReadFile("../assets/openapi.json")
	if err != nil {
		t.Fatal(err)
	}
	var spec struct {
		Paths map[string]map[string]json.RawMessage `json:"paths"`
	}
	if err = json.Unmarshal(b, &spec); err != nil {
		t.Fatal(err)
	}
	actual := map[string]bool{}
	for _, route := range engine.Routes() {
		if route.Method == "HEAD" {
			continue
		}
		path := strings.ReplaceAll(strings.ReplaceAll(route.Path, ":review_id", "{review_id}"), ":id", "{id}")
		if path == "/" || path == "/docs" || path == "/openapi.json" || strings.HasPrefix(path, "/images/") {
			continue
		}
		actual[strings.ToLower(route.Method)+" "+path] = true
	}
	expected := 0
	for path, item := range spec.Paths {
		for method := range item {
			key := method + " " + path
			if !actual[key] {
				t.Errorf("documented operation has no Gin route: %s", key)
			}
			delete(actual, key)
			expected++
		}
	}
	if expected != 42 || len(actual) != 0 {
		t.Fatalf("route/schema drift: count=%d undocumented=%v", expected, actual)
	}
}
