// Package httpapi exposes the travel API through Gin.
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
	"github.com/aqutw/travel-golang-api/internal/workspace"
	"github.com/gin-gonic/gin"
)

type API struct {
	catalog *catalog.Catalog
	store   *workspace.Store
}
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
	Code    string           `json:"code"`
	Message string           `json:"message"`
	Fields  []FieldViolation `json:"fields,omitempty"`
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
	writeJSON(w, status, ErrorEnvelope{ErrorBody{Code: code, Message: message}})
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

func New(c *catalog.Catalog, settings ...Options) *gin.Engine {
	opts := defaultOptions()
	if len(settings) > 0 {
		opts = settings[0].withDefaults()
	}
	a := &API{catalog: c, store: opts.Store}
	router := gin.New()
	router.HandleMethodNotAllowed = true
	router.RedirectTrailingSlash = false
	router.RedirectFixedPath = false
	_ = router.SetTrustedProxies(nil)
	router.Use(requestID(), accessLog(opts.Logger), recovery(opts.Logger), transport(opts), bodyLimit())
	read := func(path string, handler gin.HandlerFunc) { router.GET(path, handler); router.HEAD(path, handler) }
	read("/api/v1/config", a.config)
	read("/api/v1/navigation", a.navigation)
	read("/api/v1/home", a.home)
	read("/api/v1/categories", a.categories)
	read("/api/v1/cruise/regions", a.cruiseRegions)
	read("/api/v1/destinations", a.destinations)
	read("/api/v1/destinations/:id", a.destination)
	read("/api/v1/search/suggestions", a.suggestions)
	read("/api/v1/search/filters", a.filters)
	read("/api/v1/products", a.products)
	read("/api/v1/products/map", a.productMap)
	read("/api/v1/products/:id", a.product)
	read("/api/v1/products/:id/rooms", a.options)
	read("/api/v1/products/:id/cabins", a.options)
	read("/api/v1/products/:id/plans", a.options)
	read("/api/v1/products/:id/itinerary", a.itinerary)
	read("/api/v1/products/:id/calendar", a.calendar)
	read("/api/v1/products/:id/departures", a.departures)
	read("/api/v1/products/:id/availability", a.availability)
	read("/api/v1/products/:id/reviews", a.reviews)
	read("/api/v1/products/:id/related", a.related)
	read("/api/v1/collections", a.collections)
	read("/api/v1/collections/:id", a.collection)
	read("/api/v1/collections/:id/products", a.collectionProducts)
	read("/api/v1/campaigns", a.campaigns)
	read("/api/v1/campaigns/:id", a.campaign)
	read("/api/v1/articles", a.articles)
	read("/api/v1/articles/:id", a.article)
	read("/api/v1/faqs", a.faqs)
	read("/healthz", func(ctx *gin.Context) {
		if valid(ctx.Writer, newQuery(ctx.Request)) && databaseOK(ctx.Writer, c.Ping(ctx.Request.Context())) {
			a.reply(ctx.Writer, map[string]string{"status": "ok"})
		}
	})
	read("/openapi.json", func(ctx *gin.Context) {
		b, _ := assets.Files.ReadFile("openapi.json")
		ctx.Data(200, "application/json", b)
	})
	read("/images/*path", func(ctx *gin.Context) {
		mime, content, err := c.Media(ctx.Request.Context(), ctx.Request.URL.Path)
		if resourceOK(ctx.Writer, err, "image") {
			ctx.Header("Content-Type", mime)
			http.ServeContent(ctx.Writer, ctx.Request, ctx.Request.URL.Path, time.Time{}, bytes.NewReader(content))
		}
	})
	explorer := func(ctx *gin.Context) {
		b, _ := assets.Files.ReadFile("index.html")
		ctx.Data(200, "text/html; charset=utf-8", b)
	}
	read("/", explorer)
	read("/docs", func(ctx *gin.Context) {
		b, _ := assets.Files.ReadFile("docs.html")
		ctx.Data(200, "text/html; charset=utf-8", b)
	})
	limited := newLimiter(opts)
	router.POST("/api/v1/products/:id/quotes", limited, a.quote)
	protected := router.Group("/api/v1", authorize(opts.APIKey), limited)
	for _, resource := range []string{"wishlists", "trips"} {
		protected.GET("/"+resource, a.documents)
		protected.HEAD("/"+resource, a.documents)
		protected.GET("/"+resource+"/:id", a.document)
		protected.HEAD("/"+resource+"/:id", a.document)
		protected.POST("/"+resource, a.saveDocument)
		protected.PUT("/"+resource+"/:id", a.saveDocument)
	}
	protected.POST("/products/:id/reviews", a.saveReview)
	protected.PUT("/products/:id/reviews/:review_id", a.saveReview)
	protected.GET("/products/:id/reviews/:review_id", a.review)
	protected.HEAD("/products/:id/reviews/:review_id", a.review)
	router.NoMethod(func(ctx *gin.Context) {
		if allow := ctx.Writer.Header().Get("Allow"); allow != "" {
			ctx.Header("Allow", allow+", OPTIONS")
		}
		abort(ctx, 405, "method_not_allowed", "method is not supported for this route")
	})
	router.NoRoute(func(ctx *gin.Context) { abort(ctx, 404, "not_found", "route not found") })
	return router
}

func (a *API) getProduct(ctx *gin.Context) (catalog.Product, bool) {
	w, r := ctx.Writer, ctx.Request
	p, err := a.catalog.Product(r.Context(), ctx.Param("id"))
	return p, resourceOK(w, err, "product")
}

func (a *API) products(ctx *gin.Context) {
	w, r := ctx.Writer, ctx.Request
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
func (a *API) product(ctx *gin.Context) {
	w, r := ctx.Writer, ctx.Request
	if !valid(w, newQuery(r)) {
		return
	}
	if p, ok := a.getProduct(ctx); ok {
		a.reply(w, p)
	}
}
func (a *API) filters(ctx *gin.Context) {
	w, r := ctx.Writer, ctx.Request
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

func (a *API) productMap(ctx *gin.Context) {
	w, r := ctx.Writer, ctx.Request
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

func (a *API) options(ctx *gin.Context) {
	w, r := ctx.Writer, ctx.Request
	if !valid(w, newQuery(r)) {
		return
	}
	p, ok := a.getProduct(ctx)
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
func (a *API) itinerary(ctx *gin.Context) {
	w, r := ctx.Writer, ctx.Request
	if !valid(w, newQuery(r)) {
		return
	}
	p, ok := a.getProduct(ctx)
	if !ok {
		return
	}
	if p.Type != "cruise" && p.Type != "tour" {
		failure(w, 404, "not_applicable", "itinerary applies to cruises and tours")
		return
	}
	a.reply(w, p.Itinerary())
}
func (a *API) calendar(ctx *gin.Context) {
	w, r := ctx.Writer, ctx.Request
	q := newQuery(r, "start_date", "end_date")
	d := q.dates(a.catalog.Today, 30, 90)
	if !valid(w, q) {
		return
	}
	if p, ok := a.getProduct(ctx); ok {
		a.reply(w, a.catalog.Calendar(p, d))
	}
}
func (a *API) departures(ctx *gin.Context) {
	w, r := ctx.Writer, ctx.Request
	q := newQuery(r, "start_date", "end_date", "page", "per_page", "status")
	d := q.dates(a.catalog.Today, 30, 90)
	page, size := q.pagination()
	status := q.enum("status", "", "available", "guaranteed", "forming", "sold_out")
	if !valid(w, q) {
		return
	}
	p, ok := a.getProduct(ctx)
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
func (a *API) availability(ctx *gin.Context) {
	w, r := ctx.Writer, ctx.Request
	p, ok := a.getProduct(ctx)
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
func (a *API) reviews(ctx *gin.Context) {
	w, r := ctx.Writer, ctx.Request
	q := newQuery(r, "page", "per_page", "rating")
	page, size := q.pagination()
	rating := q.integer("rating", 0, 1, 5)
	if !valid(w, q) {
		return
	}
	p, ok := a.getProduct(ctx)
	if !ok {
		return
	}
	items, err := a.catalog.Reviews(r.Context(), p.ID, rating)
	if !databaseOK(w, err) {
		return
	}
	listReply(a, w, items, page, size, nil)
}
func (a *API) related(ctx *gin.Context) {
	w, r := ctx.Writer, ctx.Request
	q := newQuery(r, "page", "per_page")
	page, size := q.pagination()
	if !valid(w, q) {
		return
	}
	p, ok := a.getProduct(ctx)
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
