// Package httpapi exposes read-only page data over HTTP.
package httpapi

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/aqutw/travel-golang-api/internal/assets"
	"github.com/aqutw/travel-golang-api/internal/catalog"
)

type API struct{ catalog *catalog.Catalog }
type Pagination struct {
	Page       int  `json:"page"`
	PerPage    int  `json:"per_page"`
	Total      int  `json:"total"`
	TotalPages int  `json:"total_pages"`
	HasNext    bool `json:"has_next"`
}
type Meta struct {
	Mock          bool            `json:"mock"`
	Locale        string          `json:"locale"`
	Currency      string          `json:"currency"`
	ReferenceDate string          `json:"reference_date"`
	Pagination    *Pagination     `json:"pagination,omitempty"`
	Facets        *catalog.Facets `json:"facets,omitempty"`
}
type Envelope struct {
	Data any  `json:"data"`
	Meta Meta `json:"meta"`
}
type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
type ErrorEnvelope struct {
	Error ErrorBody `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
func failure(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, ErrorEnvelope{ErrorBody{code, message}})
}
func valid(w http.ResponseWriter, q *query) bool {
	if !databaseOK(w, q.dbErr) {
		return false
	}
	if q.err != nil {
		failure(w, 400, "invalid_query", q.err.Error())
		return false
	}
	return true
}

func databaseOK(w http.ResponseWriter, err error) bool {
	if err == nil {
		return true
	}
	log.Printf("catalog query: %v", err)
	failure(w, 500, "database_error", "unable to read catalog")
	return false
}
func resourceOK(w http.ResponseWriter, err error, name string) bool {
	if errors.Is(err, sql.ErrNoRows) {
		failure(w, 404, "not_found", name+" not found")
		return false
	}
	return databaseOK(w, err)
}
func (a *API) meta() Meta {
	return Meta{Mock: true, Locale: "zh-TW", Currency: "TWD", ReferenceDate: a.catalog.Today.Format("2006-01-02")}
}
func (a *API) reply(w http.ResponseWriter, data any) { writeJSON(w, 200, Envelope{data, a.meta()}) }
func pageOf[T any](items []T, page, size int) ([]T, *Pagination) {
	total := len(items)
	start := (page - 1) * size
	if start > total {
		start = total
	}
	end := start + size
	if end > total {
		end = total
	}
	return items[start:end], &Pagination{page, size, total, (total + size - 1) / size, end < total}
}
func listReply[T any](a *API, w http.ResponseWriter, items []T, page, size int, facets *catalog.Facets) {
	data, p := pageOf(items, page, size)
	m := a.meta()
	m.Pagination = p
	m.Facets = facets
	writeJSON(w, 200, Envelope{data, m})
}

func New(c *catalog.Catalog) http.Handler {
	a := &API{catalog: c}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/config", a.config)
	mux.HandleFunc("/api/v1/navigation", a.navigation)
	mux.HandleFunc("/api/v1/home", a.home)
	mux.HandleFunc("/api/v1/categories", a.categories)
	mux.HandleFunc("/api/v1/cruise/regions", a.cruiseRegions)
	mux.HandleFunc("/api/v1/destinations", a.destinations)
	mux.HandleFunc("/api/v1/destinations/{id}", a.destination)
	mux.HandleFunc("/api/v1/search/suggestions", a.suggestions)
	mux.HandleFunc("/api/v1/search/filters", a.filters)
	mux.HandleFunc("/api/v1/products", a.products)
	mux.HandleFunc("/api/v1/products/map", a.productMap)
	mux.HandleFunc("/api/v1/products/{id}", a.product)
	mux.HandleFunc("/api/v1/products/{id}/rooms", a.options)
	mux.HandleFunc("/api/v1/products/{id}/cabins", a.options)
	mux.HandleFunc("/api/v1/products/{id}/plans", a.options)
	mux.HandleFunc("/api/v1/products/{id}/itinerary", a.itinerary)
	mux.HandleFunc("/api/v1/products/{id}/calendar", a.calendar)
	mux.HandleFunc("/api/v1/products/{id}/departures", a.departures)
	mux.HandleFunc("/api/v1/products/{id}/availability", a.availability)
	mux.HandleFunc("/api/v1/products/{id}/reviews", a.reviews)
	mux.HandleFunc("/api/v1/products/{id}/related", a.related)
	mux.HandleFunc("/api/v1/collections", a.collections)
	mux.HandleFunc("/api/v1/collections/{id}", a.collection)
	mux.HandleFunc("/api/v1/collections/{id}/products", a.collectionProducts)
	mux.HandleFunc("/api/v1/campaigns", a.campaigns)
	mux.HandleFunc("/api/v1/campaigns/{id}", a.campaign)
	mux.HandleFunc("/api/v1/articles", a.articles)
	mux.HandleFunc("/api/v1/articles/{id}", a.article)
	mux.HandleFunc("/api/v1/faqs", a.faqs)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if valid(w, newQuery(r)) && databaseOK(w, c.Ping(r.Context())) {
			a.reply(w, map[string]string{"status": "ok"})
		}
	})
	mux.HandleFunc("/openapi.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		b, _ := assets.Files.ReadFile("openapi.json")
		_, _ = w.Write(b)
	})
	mux.HandleFunc("/images/", func(w http.ResponseWriter, r *http.Request) {
		mime, content, err := c.Media(r.Context(), r.URL.Path)
		if !resourceOK(w, err, "image") {
			return
		}
		w.Header().Set("Content-Type", mime)
		http.ServeContent(w, r, r.URL.Path, time.Time{}, bytes.NewReader(content))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" && r.URL.Path != "/docs" {
			failure(w, 404, "not_found", "route not found")
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		b, _ := assets.Files.ReadFile("index.html")
		_, _ = w.Write(b)
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-store")
		if r.Method == http.MethodOptions {
			w.WriteHeader(204)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD, OPTIONS")
			failure(w, 405, "method_not_allowed", "only GET, HEAD and OPTIONS are supported")
			return
		}
		mux.ServeHTTP(w, r)
	})
}

func (a *API) getProduct(w http.ResponseWriter, r *http.Request) (catalog.Product, bool) {
	p, err := a.catalog.Product(r.Context(), r.PathValue("id"))
	return p, resourceOK(w, err, "product")
}

func (a *API) products(w http.ResponseWriter, r *http.Request) {
	q, s, page, size := a.searchQuery(r)
	if !valid(w, q) {
		return
	}
	items, err := a.catalog.Search(r.Context(), s)
	if !databaseOK(w, err) {
		return
	}
	f := catalog.CountFacets(items)
	listReply(a, w, items, page, size, &f)
}
func (a *API) product(w http.ResponseWriter, r *http.Request) {
	if !valid(w, newQuery(r)) {
		return
	}
	if p, ok := a.getProduct(w, r); ok {
		a.reply(w, p)
	}
}
func (a *API) filters(w http.ResponseWriter, r *http.Request) {
	q, s, _, _ := a.searchQuery(r)
	if !valid(w, q) {
		return
	}
	items, err := a.catalog.Search(r.Context(), s)
	if databaseOK(w, err) {
		a.reply(w, catalog.CountFacets(items))
	}
}

type MapPoint struct {
	ID        string        `json:"id"`
	Type      string        `json:"type"`
	Title     string        `json:"title"`
	Latitude  float64       `json:"latitude"`
	Longitude float64       `json:"longitude"`
	Price     catalog.Price `json:"price"`
}

func (a *API) productMap(w http.ResponseWriter, r *http.Request) {
	q, s, page, size := a.searchQuery(r)
	if !valid(w, q) {
		return
	}
	points := []MapPoint{}
	items, err := a.catalog.Search(r.Context(), s)
	if !databaseOK(w, err) {
		return
	}
	for _, p := range items {
		points = append(points, MapPoint{p.ID, p.Type, p.Title, p.Latitude, p.Longitude, p.Price})
	}
	listReply(a, w, points, page, size, nil)
}

func (a *API) options(w http.ResponseWriter, r *http.Request) {
	if !valid(w, newQuery(r)) {
		return
	}
	p, ok := a.getProduct(w, r)
	if !ok {
		return
	}
	resource := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
	if resource == "rooms" && p.Type != "stay" || resource == "cabins" && p.Type != "cruise" || resource == "plans" && (p.Type == "stay" || p.Type == "cruise") {
		failure(w, 404, "not_applicable", "resource does not apply to this product type")
		return
	}
	a.reply(w, p.Options())
}
func (a *API) itinerary(w http.ResponseWriter, r *http.Request) {
	if !valid(w, newQuery(r)) {
		return
	}
	p, ok := a.getProduct(w, r)
	if !ok {
		return
	}
	if p.Type != "cruise" && p.Type != "tour" {
		failure(w, 404, "not_applicable", "itinerary applies to cruises and tours")
		return
	}
	a.reply(w, p.Itinerary())
}
func (a *API) calendar(w http.ResponseWriter, r *http.Request) {
	q := newQuery(r, "start_date", "end_date")
	d := q.dates(a.catalog.Today, 30, 90)
	if !valid(w, q) {
		return
	}
	if p, ok := a.getProduct(w, r); ok {
		a.reply(w, a.catalog.Calendar(p, d))
	}
}
func (a *API) departures(w http.ResponseWriter, r *http.Request) {
	q := newQuery(r, "start_date", "end_date", "page", "per_page", "status")
	d := q.dates(a.catalog.Today, 30, 90)
	page, size := q.pagination()
	status := q.enum("status", "", "available", "guaranteed", "forming", "sold_out")
	if !valid(w, q) {
		return
	}
	p, ok := a.getProduct(w, r)
	if !ok {
		return
	}
	if p.Type != "tour" && p.Type != "cruise" {
		failure(w, 404, "not_applicable", "departures apply to cruises and tours")
		return
	}
	items := []catalog.CalendarDay{}
	for _, row := range a.catalog.Calendar(p, d) {
		if status == "" || status == row.Status {
			items = append(items, row)
		}
	}
	listReply(a, w, items, page, size, nil)
}
func (a *API) availability(w http.ResponseWriter, r *http.Request) {
	p, ok := a.getProduct(w, r)
	if !ok {
		return
	}
	q := newQuery(r, "start_date", "end_date", "adults", "children", "quantity")
	d := q.dates(a.catalog.Today, p.DurationDays, 30)
	adults := q.integer("adults", 2, 1, 16)
	children := q.integer("children", 0, 0, 16)
	quantity := q.integer("quantity", 1, 1, 8)
	if p.Type != "stay" && d.Nights() != p.DurationDays {
		q.invalid("end_date", fmt.Sprintf("this product has a fixed duration of %d days", p.DurationDays))
	}
	if p.Price.Unit == "person" && quantity != 1 {
		q.invalid("quantity", "person-priced products require quantity=1; set adults and children instead")
	}
	if !valid(w, q) {
		return
	}
	a.reply(w, a.catalog.Availability(p, d, adults, children, quantity))
}
func (a *API) reviews(w http.ResponseWriter, r *http.Request) {
	q := newQuery(r, "page", "per_page", "rating")
	page, size := q.pagination()
	rating := q.integer("rating", 0, 1, 5)
	if !valid(w, q) {
		return
	}
	p, ok := a.getProduct(w, r)
	if !ok {
		return
	}
	items, err := a.catalog.Reviews(r.Context(), p.ID, rating)
	if !databaseOK(w, err) {
		return
	}
	listReply(a, w, items, page, size, nil)
}
func (a *API) related(w http.ResponseWriter, r *http.Request) {
	q := newQuery(r, "page", "per_page")
	page, size := q.pagination()
	if !valid(w, q) {
		return
	}
	p, ok := a.getProduct(w, r)
	if !ok {
		return
	}
	items := []catalog.Card{}
	cards, err := a.catalog.Cards(r.Context())
	if !databaseOK(w, err) {
		return
	}
	for _, v := range cards {
		if v.ID != p.ID && (v.DestinationID == p.DestinationID || v.Type == p.Type) {
			items = append(items, v)
		}
	}
	listReply(a, w, items, page, size, nil)
}
