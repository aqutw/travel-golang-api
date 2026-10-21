// examplegen records every documented API using an isolated SQLite copy.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/aqutw/travel-golang-api/internal/catalog"
	"github.com/aqutw/travel-golang-api/internal/httpapi"
	"github.com/aqutw/travel-golang-api/internal/workspace"
	"github.com/gin-gonic/gin"
)

const referenceDate = "2026-10-20"
const exampleKey = "documentation-example-key-not-for-deployment-0001"

type operation struct {
	Summary   string            `json:"summary"`
	SmokePath string            `json:"x-smoke-path"`
	Security  []json.RawMessage `json:"security"`
}
type example struct {
	Method, Route, Summary, RequestFile, ResponseFile string
	Request, Response                                 []byte
}
type recorder struct {
	handler  http.Handler
	paths    map[string]map[string]operation
	ids      map[string]string
	examples map[string]example
	errors   map[string]example
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	specBytes, err := os.ReadFile("internal/assets/openapi.json")
	if err != nil {
		return err
	}
	var spec struct {
		Paths map[string]map[string]operation `json:"paths"`
	}
	if err = json.Unmarshal(specBytes, &spec); err != nil {
		return err
	}
	temporary, err := os.MkdirTemp("", "travel-examples-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(temporary)
	seed, err := os.ReadFile("data/travel.sqlite3")
	if err != nil {
		return err
	}
	dbPath := filepath.Join(temporary, "travel.sqlite3")
	if err = os.WriteFile(dbPath, seed, 0600); err != nil {
		return err
	}
	today, _ := time.Parse("2006-01-02", referenceDate)
	catalogDB, err := catalog.New(dbPath, today)
	if err != nil {
		return err
	}
	defer catalogDB.Close()
	store, err := workspace.New(dbPath)
	if err != nil {
		return err
	}
	defer store.Close()
	gin.SetMode(gin.ReleaseMode)
	r := recorder{handler: httpapi.New(catalogDB, httpapi.Options{Store: store, APIKey: exampleKey, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), Burst: 100, RatePerMinute: 10000}), paths: spec.Paths, ids: map[string]string{}, examples: map[string]example{}, errors: map[string]example{}}
	routes := make([]string, 0, len(spec.Paths))
	for route := range spec.Paths {
		routes = append(routes, route)
	}
	sort.Strings(routes)
	// Read public data before creating examples, so public responses match the seed.
	for _, route := range routes {
		op, ok := spec.Paths[route]["get"]
		if !ok || len(op.Security) > 0 {
			continue
		}
		path := op.SmokePath
		switch route {
		case "/api/v1/products":
			path = "/api/v1/products?type=stay&destination_id=taipei&per_page=1"
		case "/api/v1/products/{id}/availability":
			path = "/api/v1/products/stay-001/availability?start_date=2026-11-01&end_date=2026-11-03&adults=2&quantity=1"
		default:
			if opAllowsPagination(specBytes, route) && !strings.Contains(path, "per_page=") {
				separator := "?"
				if strings.Contains(path, "?") {
					separator = "&"
				}
				path += separator + "per_page=2"
			}
		}
		if _, err = r.capture("GET", route, path, nil, 200, ""); err != nil {
			return err
		}
	}
	wishlist := httpapi.WishlistInput{Title: "週末收藏", Description: "候選住宿", ProductIDs: []string{"stay-001"}}
	trip := httpapi.TripInput{Title: "台北兩日", StartDate: "2026-11-01", EndDate: "2026-11-03", Adults: 2, Items: []httpapi.TripItem{{ProductID: "stay-001", Day: 1, Notes: "入住"}}, Notes: "安排早餐"}
	review := httpapi.ReviewInput{Author: "測試旅人", Rating: 5, Content: "交通便利，房間舒適。", TravelDate: "2026-10-01"}
	for _, entry := range []struct {
		route, path, variable string
		body                  any
	}{
		{"/api/v1/wishlists", "/api/v1/wishlists", "wishlist_id", wishlist},
		{"/api/v1/trips", "/api/v1/trips", "trip_id", trip},
		{"/api/v1/products/{id}/reviews", "/api/v1/products/stay-001/reviews", "review_id", review},
	} {
		if _, err = r.capture("POST", entry.route, entry.path, entry.body, 201, entry.variable); err != nil {
			return err
		}
	}
	for _, entry := range []struct{ route, path string }{
		{"/api/v1/wishlists", "/api/v1/wishlists?per_page=2"},
		{"/api/v1/wishlists/{id}", "/api/v1/wishlists/{{wishlist_id}}"},
		{"/api/v1/trips", "/api/v1/trips?per_page=2"},
		{"/api/v1/trips/{id}", "/api/v1/trips/{{trip_id}}"},
		{"/api/v1/products/{id}/reviews/{review_id}", "/api/v1/products/stay-001/reviews/{{review_id}}"},
	} {
		if _, err = r.capture("GET", entry.route, entry.path, nil, 200, ""); err != nil {
			return err
		}
	}
	wishlist.Title = "親子住宿"
	wishlist.Description = "更新候選"
	wishlist.ProductIDs = []string{"stay-002"}
	trip.Notes = "更新行程：安排早餐與散步"
	review.Rating = 4
	review.Content = "交通便利，房間舒適；早餐選擇可以更多。"
	for _, entry := range []struct {
		route, path string
		body        any
	}{
		{"/api/v1/wishlists/{id}", "/api/v1/wishlists/{{wishlist_id}}", httpapi.WishlistRequest{WishlistInput: wishlist, Version: 1}},
		{"/api/v1/trips/{id}", "/api/v1/trips/{{trip_id}}", httpapi.TripRequest{TripInput: trip, Version: 1}},
		{"/api/v1/products/{id}/reviews/{review_id}", "/api/v1/products/stay-001/reviews/{{review_id}}", httpapi.ReviewRequest{ReviewInput: review, Version: 1}},
		{"/api/v1/products/{id}/quotes", "/api/v1/products/stay-001/quotes", httpapi.QuoteRequest{StartDate: "2026-11-01", EndDate: "2026-11-03", Adults: 2, Quantity: 1}},
	} {
		if _, err = r.capture(map[bool]string{true: "POST", false: "PUT"}[strings.HasSuffix(entry.route, "/quotes")], entry.route, entry.path, entry.body, 200, ""); err != nil {
			return err
		}
	}
	if _, err = r.capture("GET", "/api/v1/products", "/api/v1/products?page=0", nil, 400, ""); err != nil {
		return err
	}
	trip.Items[0].Day = 0
	if _, err = r.capture("POST", "/api/v1/trips", "/api/v1/trips", trip, 400, ""); err != nil {
		return err
	}
	for route, methods := range spec.Paths {
		for method := range methods {
			if _, ok := r.examples[strings.ToUpper(method)+" "+route]; !ok {
				return fmt.Errorf("missing example for %s %s", method, route)
			}
		}
	}
	return r.write(routes)
}

func opAllowsPagination(raw []byte, route string) bool {
	// Read the authoritative parameter references, including shared pagination parameters.
	var spec struct {
		Paths map[string]map[string]struct {
			Parameters []struct {
				Ref string `json:"$ref"`
			} `json:"parameters"`
		} `json:"paths"`
	}
	_ = json.Unmarshal(raw, &spec)
	for _, p := range spec.Paths[route]["get"].Parameters {
		if p.Ref == "#/components/parameters/per_page" {
			return true
		}
	}
	return false
}
func filename(method, route string) string {
	return strings.ToLower(method) + "-" + strings.Trim(strings.NewReplacer("/", "-", "{", "", "}", "").Replace(route), "-")
}
func (r *recorder) capture(method, route, path string, body any, status int, variable string) (example, error) {
	op, ok := r.paths[route][strings.ToLower(method)]
	if !ok {
		return example{}, fmt.Errorf("undocumented operation %s %s", method, route)
	}
	target, err := url.Parse(path)
	if err != nil {
		return example{}, err
	}
	if target.RawQuery != "" {
		path = target.Path + "?" + target.Query().Encode()
	}
	displayPath := path
	for id, name := range r.ids {
		path = strings.ReplaceAll(path, "{{"+name+"}}", id)
	}
	var payload []byte
	if body != nil {
		var err error
		payload, err = json.MarshalIndent(body, "", "  ")
		if err != nil {
			return example{}, err
		}
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(payload))
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Request-ID", "example-request-0001")
	if len(op.Security) > 0 {
		req.Header.Set("X-API-Key", exampleKey)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if method == "POST" && len(op.Security) > 0 {
		req.Header.Set("Idempotency-Key", filename(method, route)+"-001")
	}
	response := httptest.NewRecorder()
	r.handler.ServeHTTP(response, req)
	if response.Code != status {
		return example{}, fmt.Errorf("%s %s: status %d, want %d: %s", method, path, response.Code, status, response.Body.String())
	}
	var data map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &data); err != nil {
		return example{}, err
	}
	if variable != "" {
		id, ok := data["data"].(map[string]any)["id"].(string)
		if !ok {
			return example{}, fmt.Errorf("missing generated id: %s", path)
		}
		r.ids[id] = variable
	}
	normalized := r.normalize(data)
	rendered, err := json.MarshalIndent(normalized, "", "  ")
	if err != nil {
		return example{}, err
	}
	var requestText strings.Builder
	fmt.Fprintf(&requestText, "%s %s HTTP/1.1\nHost: localhost:8080\nAccept: application/json\nX-Request-ID: example-request-0001\n", method, displayPath)
	if len(op.Security) > 0 {
		requestText.WriteString("X-API-Key: {{API_WRITE_KEY}}\n")
	}
	if key := req.Header.Get("Idempotency-Key"); key != "" {
		fmt.Fprintf(&requestText, "Idempotency-Key: %s\n", key)
	}
	if body != nil {
		requestText.WriteString("Content-Type: application/json\n")
	}
	requestText.WriteByte('\n')
	if body != nil {
		requestText.Write(payload)
		requestText.WriteByte('\n')
	}
	var responseText strings.Builder
	fmt.Fprintf(&responseText, "HTTP/1.1 %d %s\n", status, http.StatusText(status))
	for _, key := range []string{"Content-Type", "Location", "X-Request-ID", "Cache-Control", "X-Content-Type-Options"} {
		if value := response.Header().Get(key); value != "" {
			fmt.Fprintf(&responseText, "%s: %s\n", key, r.normalize(value))
		}
	}
	responseText.WriteByte('\n')
	responseText.Write(rendered)
	responseText.WriteByte('\n')
	stem := filename(method, route)
	if status >= 400 {
		stem += ".invalid"
	}
	ex := example{Method: method, Route: route, Summary: op.Summary, RequestFile: stem + ".request.http", ResponseFile: stem + ".response.http", Request: []byte(requestText.String()), Response: []byte(responseText.String())}
	if status >= 400 {
		r.errors[method+" "+route] = ex
	} else {
		r.examples[method+" "+route] = ex
	}
	return ex, nil
}
func (r *recorder) normalize(value any) any {
	switch v := value.(type) {
	case string:
		for id, name := range r.ids {
			v = strings.ReplaceAll(v, id, "{{"+name+"}}")
		}
		return v
	case []any:
		for i, item := range v {
			v[i] = r.normalize(item)
		}
		return v
	case map[string]any:
		if id, ok := v["id"].(string); ok {
			if _, managed := r.ids[id]; managed {
				if _, document := v["kind"]; document {
					v["created_at"] = referenceDate + "T00:00:00Z"
					v["updated_at"] = referenceDate + "T00:00:00Z"
					if v["version"].(float64) > 1 {
						v["updated_at"] = referenceDate + "T00:05:00Z"
					}
				} else {
					v["created_at"] = referenceDate
				}
			}
		}
		for key, item := range v {
			v[key] = r.normalize(item)
		}
		return v
	default:
		return value
	}
}
func (r *recorder) write(routes []string) error {
	directory := "docs/examples"
	if err := os.MkdirAll(directory, 0755); err != nil {
		return err
	}
	expected := map[string]bool{}
	for _, set := range []map[string]example{r.examples, r.errors} {
		for _, ex := range set {
			for name, data := range map[string][]byte{ex.RequestFile: ex.Request, ex.ResponseFile: ex.Response} {
				expected[name] = true
				if err := os.WriteFile(filepath.Join(directory, name), data, 0644); err != nil {
					return err
				}
			}
		}
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() && regexp.MustCompile(`\.(request|response)\.http$`).MatchString(entry.Name()) && !expected[entry.Name()] {
			if err = os.Remove(filepath.Join(directory, entry.Name())); err != nil {
				return err
			}
		}
	}
	readme, err := os.ReadFile("README.md")
	if err != nil {
		return err
	}
	marker := "## Request / response examples"
	prefix, _, found := strings.Cut(string(readme), marker)
	if !found {
		return fmt.Errorf("README examples heading not found")
	}
	var table strings.Builder
	counts := map[string]int{}
	for _, ex := range r.examples {
		counts[ex.Method]++
	}
	fmt.Fprintf(&table, marker+"\n\n共 %d 個 API：%d GET、%d POST、%d PUT。每一列對應一組 method 與 endpoint，範例包含完整 JSON 回應。\n\n範例日期為 `REFERENCE_DATE=2026-10-20`。`{{API_WRITE_KEY}}` 請替換為服務金鑰；`{{wishlist_id}}`、`{{trip_id}}`、`{{review_id}}` 請使用對應 POST 回傳的 ID。回應中的時間為固定示例值，PUT 範例使用建立後的 `version=1`。\n\n| Method | Endpoint | 說明 | Request 範例 | Response 範例 |\n| --- | --- | --- | --- | --- |\n", len(r.examples), counts["GET"], counts["POST"], counts["PUT"])
	for _, route := range routes {
		for _, method := range []string{"GET", "POST", "PUT"} {
			ex, ok := r.examples[method+" "+route]
			if !ok {
				continue
			}
			requestLink := fmt.Sprintf("[Request](docs/examples/%s)", ex.RequestFile)
			responseLink := fmt.Sprintf("[Response](docs/examples/%s)", ex.ResponseFile)
			if invalid, ok := r.errors[method+" "+route]; ok {
				requestLink += fmt.Sprintf(" · [400](docs/examples/%s)", invalid.RequestFile)
				responseLink += fmt.Sprintf(" · [400](docs/examples/%s)", invalid.ResponseFile)
			}
			fmt.Fprintf(&table, "| `%s` | `%s` | %s | %s | %s |\n", method, route, strings.ReplaceAll(ex.Summary, "|", `\|`), requestLink, responseLink)
		}
	}
	if err = os.WriteFile("README.md", []byte(prefix+table.String()), 0644); err != nil {
		return err
	}
	fmt.Printf("Generated %d operation examples and %d error examples (%d HTTP files).\n", len(r.examples), len(r.errors), len(expected))
	return nil
}
