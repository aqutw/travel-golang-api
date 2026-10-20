package httpapi

import (
	"database/sql"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/aqutw/travel-golang-api/internal/catalog"
)

type query struct {
	values url.Values
	err    error
	dbErr  error
}

func newQuery(r *http.Request, allowed ...string) *query {
	v, err := url.ParseQuery(r.URL.RawQuery)
	q := &query{values: v, err: err}
	if len(r.URL.RawQuery) > 4096 {
		q.invalid("query", "must be at most 4096 bytes")
	}
	for key, values := range v {
		if len(values) != 1 {
			q.invalid(key, "must occur once")
		}
		if key == "locale" || key == "currency" {
			continue
		}
		found := false
		for _, a := range allowed {
			if key == a {
				found = true
				break
			}
		}
		if !found {
			q.invalid(key, "unknown query parameter")
		}
	}
	if s := q.str("locale"); q.values.Has("locale") && s != "zh-TW" {
		q.invalid("locale", "only zh-TW is supported")
	}
	if s := q.str("currency"); q.values.Has("currency") && s != "TWD" {
		q.invalid("currency", "only TWD is supported")
	}
	return q
}
func (q *query) invalid(field, message string) {
	if q.err == nil {
		q.err = fmt.Errorf("%s: %s", field, message)
	}
}
func (q *query) str(key string) string { return strings.TrimSpace(q.values.Get(key)) }
func (q *query) integer(key string, fallback, min, max int) int {
	if !q.values.Has(key) {
		return fallback
	}
	v, err := strconv.Atoi(q.str(key))
	if err != nil || v < min || v > max {
		q.invalid(key, fmt.Sprintf("must be an integer between %d and %d", min, max))
		return fallback
	}
	return v
}
func (q *query) number(key string, fallback, min, max float64) float64 {
	if !q.values.Has(key) {
		return fallback
	}
	v, err := strconv.ParseFloat(q.str(key), 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) || v < min || v > max {
		q.invalid(key, fmt.Sprintf("must be a number between %g and %g", min, max))
		return fallback
	}
	return v
}
func (q *query) boolean(key string) bool {
	v := q.str(key)
	if !q.values.Has(key) {
		return false
	}
	if v != "true" && v != "false" {
		q.invalid(key, "must be true or false")
	}
	return v == "true"
}
func (q *query) enum(key, fallback string, values ...string) string {
	if !q.values.Has(key) {
		return fallback
	}
	v := q.str(key)
	for _, s := range values {
		if s == v {
			return v
		}
	}
	q.invalid(key, "must be one of "+strings.Join(values, ", "))
	return fallback
}
func (q *query) pagination() (int, int) {
	return q.integer("page", 1, 1, 1000000), q.integer("per_page", 12, 1, 100)
}
func (q *query) dates(today time.Time, defaultDays, maxDays int) catalog.DateRange {
	start := today.AddDate(0, 0, 7)
	if q.values.Has("start_date") {
		v, err := time.ParseInLocation("2006-01-02", q.str("start_date"), today.Location())
		if err != nil {
			q.invalid("start_date", "must use YYYY-MM-DD")
		} else {
			start = v
		}
	}
	end := start.AddDate(0, 0, defaultDays)
	if q.values.Has("end_date") {
		v, err := time.ParseInLocation("2006-01-02", q.str("end_date"), today.Location())
		if err != nil {
			q.invalid("end_date", "must use YYYY-MM-DD")
		} else {
			end = v
		}
	}
	d := catalog.DateRange{Start: start, End: end}
	if start.Before(today) || start.After(today.AddDate(0, 0, 365)) {
		q.invalid("start_date", "must be between reference_date and reference_date + 365 days")
	}
	if d.Nights() < 1 || d.Nights() > maxDays {
		q.invalid("end_date", fmt.Sprintf("must be after start_date by 1 to %d days", maxDays))
	}
	return d
}

var searchKeys = []string{"q", "type", "destination_id", "country", "tag", "amenity", "sort", "min_price", "max_price", "min_rating", "min_days", "max_days", "free_cancellation", "bbox", "start_date", "end_date", "adults", "children", "quantity", "page", "per_page", "departure_port", "departure_city", "cruise_line", "property_type"}

func (a *API) searchQuery(r *http.Request) (*query, catalog.Search, int, int) {
	q := newQuery(r, searchKeys...)
	s := catalog.Search{Query: q.str("q"), Type: q.enum("type", "", "stay", "cruise", "tour", "activity", "transfer", "esim", "sport", "rail"), DestinationID: q.str("destination_id"), Country: q.enum("country", "", "TW", "JP", "KR", "TH", "SG"), Tag: q.str("tag"), Amenity: q.str("amenity"),
		DeparturePort: q.str("departure_port"), DepartureCity: q.str("departure_city"), CruiseLine: q.str("cruise_line"), PropertyType: q.enum("property_type", "", "hotel", "hot_spring", "camping", "entire_house"),
		Sort:     q.enum("sort", "popular", "popular", "price_asc", "price_desc", "rating", "duration"),
		MinPrice: q.integer("min_price", 0, 0, 10000000), MaxPrice: q.integer("max_price", 10000000, 0, 10000000),
		MinDays: q.integer("min_days", 1, 1, 30), MaxDays: q.integer("max_days", 30, 1, 30), MinRating: q.number("min_rating", 0, 0, 5),
		FreeCancellation: q.boolean("free_cancellation"), Adults: q.integer("adults", 2, 1, 16), Children: q.integer("children", 0, 0, 16), Quantity: q.integer("quantity", 1, 1, 8)}
	if s.Type != "" {
		exists, err := a.catalog.HasCategory(r.Context(), s.Type)
		q.dbErr = err
		if !exists {
			q.invalid("type", "unknown product type")
		}
	}
	if s.DestinationID != "" {
		if _, err := a.catalog.Destination(r.Context(), s.DestinationID); errors.Is(err, sql.ErrNoRows) {
			q.invalid("destination_id", "unknown destination")
		} else if err != nil {
			q.dbErr = err
		}
	}
	if s.Country != "" {
		found, err := a.catalog.HasCountry(r.Context(), s.Country)
		if err != nil {
			q.dbErr = err
		}
		if !found {
			q.invalid("country", "unknown country code")
		}
	}
	if s.MinPrice > s.MaxPrice {
		q.invalid("min_price", "must not exceed max_price")
	}
	if s.MinDays > s.MaxDays {
		q.invalid("min_days", "must not exceed max_days")
	}
	if q.values.Has("bbox") {
		parts := strings.Split(q.str("bbox"), ",")
		b := [4]float64{}
		if len(parts) != 4 {
			q.invalid("bbox", "must contain west,south,east,north")
		} else {
			for i, p := range parts {
				v, e := strconv.ParseFloat(p, 64)
				limit := 180.0
				if i%2 == 1 {
					limit = 90
				}
				if e != nil || math.IsNaN(v) || math.IsInf(v, 0) || math.Abs(v) > limit {
					q.invalid("bbox", "invalid coordinate")
				}
				b[i] = v
			}
			if b[0] > b[2] || b[1] > b[3] {
				q.invalid("bbox", "coordinates must be ordered west,south,east,north; antimeridian crossing is unsupported")
			}
			s.Bounds = &b
		}
	}
	if q.values.Has("start_date") || q.values.Has("end_date") {
		if !q.values.Has("start_date") || !q.values.Has("end_date") {
			q.invalid("dates", "provide both start_date and end_date")
		}
		d := q.dates(a.catalog.Today, 1, 30)
		s.Dates = &d
	} else if q.values.Has("adults") || q.values.Has("children") || q.values.Has("quantity") {
		q.invalid("dates", "traveler filters require start_date and end_date")
	}
	page, size := q.pagination()
	return q, s, page, size
}
