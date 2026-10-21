package httpapi

import (
	"bytes"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"reflect"
	"regexp"
	"strings"
	"time"

	"github.com/aqutw/travel-golang-api/internal/catalog"
	"github.com/aqutw/travel-golang-api/internal/workspace"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/go-playground/validator/v10"
)

type FieldViolation struct {
	Field string `json:"field"`
	Rule  string `json:"rule"`
}
type WishlistInput struct {
	Title       string   `json:"title" binding:"required,notblank,max=100"`
	Description string   `json:"description" binding:"max=1000"`
	ProductIDs  []string `json:"product_ids" binding:"required,max=100,unique,dive,required,max=64"`
}
type WishlistRequest struct {
	WishlistInput
	Version int `json:"version,omitempty" binding:"min=0"`
}
type TripItem struct {
	ProductID string `json:"product_id" binding:"required,max=64"`
	Day       int    `json:"day" binding:"required,min=1,max=30"`
	Notes     string `json:"notes" binding:"max=1000"`
}
type TripInput struct {
	Title     string     `json:"title" binding:"required,notblank,max=100"`
	StartDate string     `json:"start_date" binding:"required,datetime=2006-01-02"`
	EndDate   string     `json:"end_date" binding:"required,datetime=2006-01-02"`
	Adults    int        `json:"adults" binding:"required,min=1,max=16"`
	Children  int        `json:"children" binding:"min=0,max=16"`
	Items     []TripItem `json:"items" binding:"required,max=100,dive"`
	Notes     string     `json:"notes" binding:"max=2000"`
}
type TripRequest struct {
	TripInput
	Version int `json:"version,omitempty" binding:"min=0"`
}
type ReviewInput struct {
	Author     string `json:"author" binding:"required,notblank,max=80"`
	Rating     int    `json:"rating" binding:"required,min=1,max=5"`
	Content    string `json:"content" binding:"required,notblank,max=2000"`
	TravelDate string `json:"travel_date" binding:"required,datetime=2006-01-02"`
}
type ReviewRequest struct {
	ReviewInput
	Version int `json:"version,omitempty" binding:"min=0"`
}
type QuoteRequest struct {
	StartDate string `json:"start_date" binding:"required,datetime=2006-01-02"`
	EndDate   string `json:"end_date" binding:"required,datetime=2006-01-02"`
	Adults    int    `json:"adults" binding:"required,min=1,max=16"`
	Children  int    `json:"children" binding:"min=0,max=16"`
	Quantity  int    `json:"quantity" binding:"required,min=1,max=8"`
}

func init() {
	binding.EnableDecoderDisallowUnknownFields = true
	if v, ok := binding.Validator.Engine().(*validator.Validate); ok {
		v.RegisterTagNameFunc(func(f reflect.StructField) string { return strings.Split(f.Tag.Get("json"), ",")[0] })
		_ = v.RegisterValidation("notblank", func(fl validator.FieldLevel) bool { return strings.TrimSpace(fl.Field().String()) != "" })
	}
}
func invalidBody(c *gin.Context, fields ...FieldViolation) {
	c.Abort()
	writeJSON(c.Writer, 400, ErrorEnvelope{Error: ErrorBody{Code: "invalid_body", Message: "JSON body validation failed", Fields: fields}})
}
func bindBody(c *gin.Context, target any) bool {
	if !valid(c.Writer, newQuery(c.Request)) {
		return false
	}
	media, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || media != "application/json" {
		abort(c, 415, "unsupported_media_type", "Content-Type must be application/json")
		return false
	}
	raw, err := io.ReadAll(c.Request.Body)
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			abort(c, 413, "body_too_large", "JSON body must be at most 65536 bytes")
		} else {
			invalidBody(c, FieldViolation{"body", "json"})
		}
		return false
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	var first json.RawMessage
	if err = decoder.Decode(&first); err != nil || len(first) == 0 || first[0] != '{' {
		invalidBody(c, FieldViolation{"body", "json_object"})
		return false
	}
	var trailing any
	if err = decoder.Decode(&trailing); err != io.EOF {
		invalidBody(c, FieldViolation{"body", "single_json_object"})
		return false
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(raw))
	if err = c.ShouldBindJSON(target); err != nil {
		fields := []FieldViolation{}
		var validation validator.ValidationErrors
		if errors.As(err, &validation) {
			for _, v := range validation {
				field := v.Namespace()
				if dot := strings.IndexByte(field, '.'); dot >= 0 {
					field = field[dot+1:]
				}
				field = strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(field, "WishlistInput.", ""), "TripInput.", ""), "ReviewInput.", "")
				fields = append(fields, FieldViolation{field, v.Tag()})
			}
		} else {
			fields = append(fields, FieldViolation{"body", "known_fields_and_types"})
		}
		invalidBody(c, fields...)
		return false
	}
	return true
}
func (a *API) bodyDates(c *gin.Context, start, end string) (catalog.DateRange, bool) {
	s, _ := time.Parse("2006-01-02", start)
	e, _ := time.Parse("2006-01-02", end)
	today := a.catalog.Today.Format("2006-01-02")
	if start < today || start > a.catalog.Today.AddDate(0, 0, 365).Format("2006-01-02") {
		invalidBody(c, FieldViolation{"start_date", "booking_horizon"})
		return catalog.DateRange{}, false
	}
	d := catalog.DateRange{Start: s, End: e}
	if d.Nights() < 1 || d.Nights() > 30 {
		invalidBody(c, FieldViolation{"end_date", "range_1_to_30_days"})
		return d, false
	}
	return d, true
}
func (a *API) hasStore(c *gin.Context) bool {
	if a.store == nil {
		abort(c, 503, "writes_disabled", "configure API_WRITE_KEY to enable workspace access")
		return false
	}
	return true
}
func writeError(c *gin.Context, err error, resource string) bool {
	if err == nil {
		return true
	}
	switch {
	case errors.Is(err, sql.ErrNoRows):
		abort(c, 404, "not_found", resource+" not found")
	case errors.Is(err, workspace.ErrConflict):
		abort(c, 409, "version_conflict", "resource changed; fetch its latest version before replacing it")
	case errors.Is(err, workspace.ErrReplayConflict):
		abort(c, 409, "idempotency_conflict", err.Error())
	case errors.Is(err, workspace.ErrProduct):
		field := "product_ids"
		if strings.HasPrefix(c.FullPath(), "/api/v1/trips") {
			field = "items.product_id"
		}
		invalidBody(c, FieldViolation{field, "existing_products"})
	default:
		databaseOK(c.Writer, err)
	}
	return false
}
func documentKind(c *gin.Context) string {
	if strings.HasPrefix(c.FullPath(), "/api/v1/wishlists") {
		return "wishlist"
	}
	return "trip"
}
func (a *API) documents(c *gin.Context) {
	q := newQuery(c.Request, "page", "per_page")
	page, size := q.pagination()
	if !valid(c.Writer, q) || !a.hasStore(c) {
		return
	}
	items, err := a.store.Documents(c.Request.Context(), documentKind(c))
	if writeError(c, err, "document") {
		listReply(a, c.Writer, items, page, size, nil)
	}
}
func (a *API) document(c *gin.Context) {
	if !valid(c.Writer, newQuery(c.Request)) || !a.hasStore(c) {
		return
	}
	item, err := a.store.Document(c.Request.Context(), documentKind(c), c.Param("id"))
	if writeError(c, err, "document") {
		a.reply(c.Writer, item)
	}
}

var keyPattern = regexp.MustCompile(`^[A-Za-z0-9._-]{8,128}$`)

func (a *API) mutate(c *gin.Context, request any, command func(*sql.Tx) (any, string, error)) {
	if !a.hasStore(c) {
		return
	}
	key := ""
	if c.Request.Method == "POST" {
		values := c.Request.Header.Values("Idempotency-Key")
		if len(values) != 1 || !keyPattern.MatchString(values[0]) {
			abort(c, 400, "invalid_idempotency_key", "Idempotency-Key must occur once and contain 8 to 128 letters, digits, dots, underscores or hyphens")
			return
		}
		key = values[0]
	}
	body, _ := json.Marshal(request)
	hash := sha256.Sum256(body)
	result, err := a.store.Apply(c.Request.Context(), c.Request.Method+" "+c.Request.URL.Path, key, hex.EncodeToString(hash[:]), command)
	if !writeError(c, err, "resource") {
		return
	}
	if result.Location != "" {
		c.Header("Location", result.Location)
	}
	if result.Replayed {
		c.Header("Idempotency-Replayed", "true")
	}
	writeJSON(c.Writer, result.Status, Envelope{Data: result.Data, Meta: a.meta()})
}
func checkVersion(c *gin.Context, version int) bool {
	if c.Request.Method == "PUT" && version < 1 {
		invalidBody(c, FieldViolation{"version", "required"})
		return false
	}
	if c.Request.Method == "POST" && version != 0 {
		invalidBody(c, FieldViolation{"version", "omit_on_create"})
		return false
	}
	return true
}
func (a *API) saveDocument(c *gin.Context) {
	kind := documentKind(c)
	var content any
	var ids []string
	var version int
	if kind == "wishlist" {
		var input WishlistRequest
		if !bindBody(c, &input) {
			return
		}
		content = input.WishlistInput
		ids = input.ProductIDs
		version = input.Version
	} else {
		var input TripRequest
		if !bindBody(c, &input) {
			return
		}
		dates, ok := a.bodyDates(c, input.StartDate, input.EndDate)
		if !ok {
			return
		}
		seen := map[string]bool{}
		ids = []string{}
		for _, item := range input.Items {
			key := fmtInt(item.Day) + ":" + item.ProductID
			if item.Day > dates.Nights() {
				invalidBody(c, FieldViolation{"items.day", "within_trip_dates"})
				return
			}
			if seen[key] {
				invalidBody(c, FieldViolation{"items", "unique_product_per_day"})
				return
			}
			seen[key] = true
			ids = append(ids, item.ProductID)
		}
		content = input.TripInput
		version = input.Version
	}
	if !checkVersion(c, version) {
		return
	}
	a.mutate(c, struct {
		Content any `json:"content"`
		Version int `json:"version"`
	}{content, version}, func(tx *sql.Tx) (any, string, error) {
		d, err := workspace.SaveDocument(c.Request.Context(), tx, kind, c.Param("id"), content, ids, version)
		return d, "/api/v1/" + kind + "s/" + d.ID, err
	})
}
func (a *API) saveReview(c *gin.Context) {
	var input ReviewRequest
	if !bindBody(c, &input) || !checkVersion(c, input.Version) {
		return
	}
	if input.TravelDate < "0001-01-01" || input.TravelDate > a.catalog.Today.Format("2006-01-02") {
		invalidBody(c, FieldViolation{"travel_date", "not_in_future"})
		return
	}
	if _, ok := a.getProduct(c); !ok {
		return
	}
	r := catalog.Review{ProductID: c.Param("id"), Author: input.Author, Rating: input.Rating, Content: input.Content, TravelDate: input.TravelDate}
	a.mutate(c, input, func(tx *sql.Tx) (any, string, error) {
		record, err := workspace.SaveReview(c.Request.Context(), tx, r, c.Param("review_id"), input.Version)
		return record, "/api/v1/products/" + r.ProductID + "/reviews/" + record.ID, err
	})
}
func (a *API) review(c *gin.Context) {
	if !valid(c.Writer, newQuery(c.Request)) || !a.hasStore(c) {
		return
	}
	r, err := a.store.Review(c.Request.Context(), c.Param("id"), c.Param("review_id"))
	if writeError(c, err, "review") {
		a.reply(c.Writer, r)
	}
}
func (a *API) quote(c *gin.Context) {
	var input QuoteRequest
	if !bindBody(c, &input) {
		return
	}
	p, ok := a.getProduct(c)
	if !ok {
		return
	}
	d, ok := a.bodyDates(c, input.StartDate, input.EndDate)
	if !ok {
		return
	}
	if p.Type != "stay" && d.Nights() != p.DurationDays {
		invalidBody(c, FieldViolation{"end_date", "fixed_product_duration"})
		return
	}
	if p.Price.Unit == "person" && input.Quantity != 1 {
		invalidBody(c, FieldViolation{"quantity", "one_for_person_pricing"})
		return
	}
	a.reply(c.Writer, a.catalog.Availability(p, d, input.Adults, input.Children, input.Quantity))
}
