package httpapi

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aqutw/travel-golang-api/internal/assets"
	"github.com/aqutw/travel-golang-api/internal/catalog"
)

func server(t *testing.T) *httptest.Server {
	t.Helper()
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	c, err := catalog.New("../../data/travel.sqlite3", now)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	s := httptest.NewServer(New(c))
	t.Cleanup(s.Close)
	return s
}

type result[T any] struct {
	Data T    `json:"data"`
	Meta Meta `json:"meta"`
}

func get[T any](t *testing.T, s *httptest.Server, path string) result[T] {
	t.Helper()
	res, err := s.Client().Get(s.URL + path)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	if res.StatusCode != 200 {
		t.Fatalf("GET %s = %d: %s", path, res.StatusCode, body)
	}
	var out result[T]
	if err = json.Unmarshal(body, &out); err != nil {
		t.Fatal(err)
	}
	if !out.Meta.Mock || out.Meta.ReferenceDate != "2026-10-20" {
		t.Fatalf("invalid metadata: %+v", out.Meta)
	}
	return out
}

func TestEveryDocumentedOperation(t *testing.T) {
	s := server(t)
	b, _ := assets.Files.ReadFile("openapi.json")
	var spec struct {
		Paths map[string]struct {
			Get struct {
				Example string `json:"x-smoke-path"`
			} `json:"get"`
		} `json:"paths"`
	}
	if err := json.Unmarshal(b, &spec); err != nil {
		t.Fatal(err)
	}
	if len(spec.Paths) != 30 {
		t.Fatalf("expected 30 documented operations, got %d", len(spec.Paths))
	}
	for route, op := range spec.Paths {
		t.Run(route, func(t *testing.T) {
			if op.Get.Example == "" {
				t.Fatal("missing smoke example")
			}
			get[json.RawMessage](t, s, op.Get.Example)
		})
	}
}

func TestSearchAndPagination(t *testing.T) {
	s := server(t)
	all := get[[]catalog.Card](t, s, "/api/v1/products?per_page=100")
	if len(all.Data) != 96 || all.Meta.Pagination.Total != 96 {
		t.Fatal("catalog size mismatch")
	}
	seen := map[string]bool{}
	for page := 1; page <= 14; page++ {
		v := get[[]catalog.Card](t, s, fmt.Sprintf("/api/v1/products?per_page=7&page=%d", page))
		for _, p := range v.Data {
			if seen[p.ID] {
				t.Fatal("duplicate across pages:", p.ID)
			}
			seen[p.ID] = true
		}
		if v.Meta.Pagination.HasNext != (page < 14) {
			t.Fatal("bad has_next")
		}
	}
	if len(seen) != 96 {
		t.Fatal("pagination dropped products")
	}
	for _, direction := range []string{"price_asc", "price_desc"} {
		items := get[[]catalog.Card](t, s, "/api/v1/products?per_page=100&sort="+direction).Data
		for i := 1; i < len(items); i++ {
			if direction == "price_asc" && items[i-1].Price.Amount > items[i].Price.Amount || direction == "price_desc" && items[i-1].Price.Amount < items[i].Price.Amount {
				t.Fatal("incorrect price order")
			}
		}
	}
	filtered := get[[]catalog.Card](t, s, "/api/v1/products?type=stay&country=JP&min_price=2500&max_price=3200&free_cancellation=true&sort=rating")
	if len(filtered.Data) == 0 {
		t.Fatal("expected filtered examples")
	}
	for _, p := range filtered.Data {
		if p.Type != "stay" || p.Price.Amount < 2500 || p.Price.Amount > 3200 || !p.FreeCancellation {
			t.Fatal("filter failed")
		}
		d := get[catalog.Destination](t, s, "/api/v1/destinations/"+p.DestinationID)
		if d.Data.CountryCode != "JP" {
			t.Fatal("country filter failed")
		}
	}
	count := 0
	for _, f := range filtered.Meta.Facets.Types {
		count += f.Count
	}
	if count != filtered.Meta.Pagination.Total {
		t.Fatal("facets must describe the full matched set")
	}
	empty := get[[]catalog.Card](t, s, "/api/v1/products?q=not-found")
	if len(empty.Data) != 0 || empty.Meta.Pagination.TotalPages != 0 || empty.Data == nil {
		t.Fatal("empty results must use []")
	}
	beyond := get[[]catalog.Card](t, s, "/api/v1/products?page=999999")
	if len(beyond.Data) != 0 || beyond.Meta.Pagination.HasNext {
		t.Fatal("bad out-of-range page")
	}
	ports := get[[]catalog.Card](t, s, "/api/v1/products?type=cruise&departure_port="+url.QueryEscape("基隆港"))
	if len(ports.Data) != 4 {
		t.Fatal("departure port filter failed")
	}
	camping := get[[]catalog.Card](t, s, "/api/v1/products?property_type=camping")
	if len(camping.Data) != 3 {
		t.Fatal("camping filter failed")
	}
}

func TestReferencesAndVariants(t *testing.T) {
	s := server(t)
	cards := get[[]catalog.Card](t, s, "/api/v1/products?per_page=100").Data
	imagePaths := map[string]bool{}
	for _, card := range cards {
		p := get[catalog.Product](t, s, card.DetailURL).Data
		if p.ID != card.ID || p.Price != card.Price {
			t.Fatal("detail and card differ")
		}
		get[catalog.Destination](t, s, "/api/v1/destinations/"+p.DestinationID)
		variants := 0
		for _, ok := range []bool{p.Stay != nil, p.Cruise != nil, p.Tour != nil, p.Activity != nil} {
			if ok {
				variants++
			}
		}
		if variants != 1 {
			t.Fatal("expected exactly one detail variant")
		}
		resource := "plans"
		if p.Type == "stay" {
			resource = "rooms"
		}
		if p.Type == "cruise" {
			resource = "cabins"
		}
		opts := get[[]catalog.Option](t, s, card.DetailURL+"/"+resource).Data
		if len(opts) != 2 || opts[0].Price.Amount != p.Price.Amount {
			t.Fatal("invalid plan data")
		}
		reviews := get[[]catalog.Review](t, s, card.DetailURL+"/reviews").Data
		sum := 0
		for _, v := range reviews {
			sum += v.Rating
		}
		if p.Rating.Count != len(reviews) || p.Rating.Average != math.Round(float64(sum)/float64(len(reviews))*10)/10 {
			t.Fatal("review summary differs from actual reviews")
		}
		for _, img := range p.Images {
			imagePaths[img.URL] = true
		}
	}
	for path := range imagePaths {
		res, err := s.Client().Get(s.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(res.Body)
		res.Body.Close()
		if res.StatusCode != 200 || !strings.Contains(string(b), "<svg") {
			t.Fatal("broken image:", path)
		}
	}
	collections := get[[]catalog.Collection](t, s, "/api/v1/collections").Data
	for _, collection := range collections {
		items := get[[]catalog.Card](t, s, "/api/v1/collections/"+collection.ID+"/products?per_page=100")
		if len(items.Data) != len(collection.ProductIDs) {
			t.Fatal("collection has missing references")
		}
	}
}

func TestAvailabilityAcrossPagesAndDates(t *testing.T) {
	s := server(t)
	for _, id := range []string{"stay-001", "cruise-001", "tour-001", "activity-001", "transfer-001", "esim-001", "sport-001", "rail-001"} {
		t.Run(id, func(t *testing.T) {
			p := get[catalog.Product](t, s, "/api/v1/products/"+id).Data
			calendar := get[[]catalog.CalendarDay](t, s, "/api/v1/products/"+id+"/calendar").Data
			if len(calendar) != 30 {
				t.Fatal("calendar must have 30 days by default")
			}
			for _, day := range calendar {
				q := get[catalog.Availability](t, s, "/api/v1/products/"+id+"/availability?adults=1&start_date="+day.Date).Data
				if day.Available != q.Available {
					t.Fatal("calendar and availability differ")
				}
				min := 0
				for _, o := range q.Options {
					if o.Available && (min == 0 || o.UnitPrice.Amount < min) {
						min = o.UnitPrice.Amount
					}
					if o.TotalAmount != o.UnitPrice.Amount*o.Multiplier {
						t.Fatal("quote calculation mismatch")
					}
				}
				if day.Available && day.Price.Amount != min {
					t.Fatal("calendar must show cheapest available plan")
				}
				items := get[[]catalog.Card](t, s, "/api/v1/products?type="+p.Type+"&start_date="+day.Date+"&end_date="+q.EndDate+"&adults=1&per_page=100").Data
				found := false
				for _, v := range items {
					if v.ID == id {
						found = true
					}
				}
				if p.Type == "stay" && found != q.Available {
					t.Fatal("search and availability differ for stay")
				}
				if p.Type != "stay" && q.Available && !found {
					t.Fatal("available departure missing from search window")
				}
			}
		})
	}
	trip := get[catalog.Availability](t, s, "/api/v1/products/stay-001/availability?start_date=2026-11-01&end_date=2026-11-04&adults=4&quantity=2").Data
	for i, opt := range trip.Options {
		min := 100
		for _, date := range []string{"2026-11-01", "2026-11-02", "2026-11-03"} {
			q := get[catalog.Availability](t, s, "/api/v1/products/stay-001/availability?start_date="+date).Data
			if q.Options[i].Remaining < min {
				min = q.Options[i].Remaining
			}
		}
		if opt.Remaining != min || opt.Multiplier != 6 || opt.TotalAmount != opt.UnitPrice.Amount*6 {
			t.Fatal("multi-night quote or minimum inventory is wrong")
		}
	}
	full := get[catalog.Availability](t, s, "/api/v1/products/stay-001/availability?adults=16&children=16")
	if full.Data.Available {
		t.Fatal("over-capacity request must be unavailable")
	}
}

func TestInvalidQueriesAndMissingResources(t *testing.T) {
	s := server(t)
	for _, query := range []string{"page=0", "page=1.5", "page=9999999999999999999999", "page=", "per_page=101", "page=1&page=2", "surprise=1", "type=hotel", "country=ZZ", "destination_id=missing", "sort=random", "property_type=boat", "min_price=-1", "min_price=5000&max_price=1", "min_days=10&max_days=2", "min_rating=NaN", "min_rating=Inf", "free_cancellation=1", "free_cancellation=", "bbox=1,2,3", "bbox=122,24,121,26", "bbox=181,0,182,1", "bbox=NaN,0,1,2", "start_date=2026-11-01", "adults=2", "start_date=2026-02-30&end_date=2026-03-01", "start_date=2026-10-19&end_date=2026-10-20", "start_date=2026-11-02&end_date=2026-11-01", "start_date=2026-11-01&end_date=2027-01-01", "start_date=2028-01-01&end_date=2028-01-02", "locale=en", "currency=USD"} {
		t.Run(query, func(t *testing.T) {
			res, err := s.Client().Get(s.URL + "/api/v1/products?" + query)
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()
			var body ErrorEnvelope
			json.NewDecoder(res.Body).Decode(&body)
			if res.StatusCode != 400 || body.Error.Code != "invalid_query" {
				t.Fatalf("expected structured 400, got %d %+v", res.StatusCode, body)
			}
		})
	}
	for path, expected := range map[string]int{"/api/v1/products/missing": 404, "/api/v1/products/stay-001/cabins": 404, "/api/v1/products/stay-001/plans": 404, "/api/v1/products/activity-001/itinerary": 404, "/api/v1/products/stay-001/departures": 404, "/api/v1/destinations/missing": 404, "/api/v1/collections/missing": 404, "/api/v1/articles/missing": 404, "/api/v1/campaigns/missing": 404, "/unknown": 404, "/api/v1/products/cruise-001/availability?quantity=2": 400, "/api/v1/products/tour-001/availability?start_date=2026-11-01&end_date=2026-11-02": 400, "/api/v1/products/stay-001/calendar?start_date=2026-11-01&end_date=2027-03-01": 400} {
		res, err := s.Client().Get(s.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != expected {
			t.Fatalf("%s: expected %d, got %d", path, expected, res.StatusCode)
		}
	}
}

func TestReadOnlyCORSAndConcurrentRequests(t *testing.T) {
	s := server(t)
	for method, code := range map[string]int{"POST": 405, "PUT": 405, "PATCH": 405, "DELETE": 405, "HEAD": 200, "OPTIONS": 204} {
		req, _ := http.NewRequest(method, s.URL+"/api/v1/products", nil)
		req.Header.Set("Origin", "http://localhost:5173")
		res, err := s.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(res.Body)
		res.Body.Close()
		if res.StatusCode != code || res.Header.Get("Access-Control-Allow-Origin") != "*" {
			t.Fatalf("%s: bad method/CORS behavior", method)
		}
		if (method == "HEAD" || method == "OPTIONS") && len(b) != 0 {
			t.Fatal("HEAD and OPTIONS must have no body")
		}
	}
	var wg sync.WaitGroup
	errors := make(chan error, 48)
	for i := 0; i < 24; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for _, path := range []string{"/api/v1/products?sort=price_desc", "/api/v1/home"} {
				res, err := s.Client().Get(s.URL + path)
				if err != nil {
					errors <- err
					continue
				}
				_, err = io.Copy(io.Discard, res.Body)
				res.Body.Close()
				if err != nil {
					errors <- err
				} else if res.StatusCode != 200 {
					errors <- fmt.Errorf("concurrent %s: HTTP %d", path, res.StatusCode)
				}
			}
		}()
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		t.Error(err)
	}
	items := get[[]catalog.Card](t, s, "/api/v1/products").Data
	if items[0].ID != "stay-001" {
		t.Fatal("concurrent sorting mutated catalog order")
	}
}
