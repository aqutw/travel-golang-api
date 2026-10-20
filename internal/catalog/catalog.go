package catalog

import (
	"context"
	"hash/fnv"
	"sort"
	"strings"
	"time"
)

type Search struct {
	Query, Type, DestinationID, Country, Tag, Amenity, Sort string
	DeparturePort, DepartureCity, CruiseLine, PropertyType  string
	MinPrice, MaxPrice, MinDays, MaxDays                    int
	MinRating                                               float64
	FreeCancellation                                        bool
	Bounds                                                  *[4]float64 // west, south, east, north
	Dates                                                   *DateRange
	Adults, Children, Quantity                              int
}

func (c *Catalog) Search(ctx context.Context, s Search) ([]Card, error) {
	clauses := []string{"c.price_amount >= ?", "c.price_amount <= ?", "c.rating_average >= ?", "c.duration_days >= ?", "c.duration_days <= ?"}
	args := []any{s.MinPrice, s.MaxPrice, s.MinRating, s.MinDays, s.MaxDays}
	for _, filter := range []struct{ column, value string }{
		{"c.type", s.Type}, {"c.destination_id", s.DestinationID}, {"d.country_code", s.Country},
		{"c.departure_port", s.DeparturePort}, {"c.departure_city", s.DepartureCity}, {"c.cruise_line", s.CruiseLine}, {"c.property_type", s.PropertyType},
	} {
		if filter.value != "" {
			clauses = append(clauses, filter.column+" = ?")
			args = append(args, filter.value)
		}
	}
	if s.Query != "" {
		clauses = append(clauses, "instr(lower(c.title||' '||c.summary||' '||d.name||' '||d.country_name||' '||coalesce((SELECT group_concat(value,' ') FROM json_each(c.tags)),'')),lower(?)) > 0")
		args = append(args, s.Query)
	}
	for _, term := range []struct{ column, value string }{{"tags", s.Tag}, {"amenities", s.Amenity}} {
		if term.value != "" {
			clauses = append(clauses, "EXISTS (SELECT 1 FROM json_each(c."+term.column+") WHERE value = ?)")
			args = append(args, term.value)
		}
	}
	if s.FreeCancellation {
		clauses = append(clauses, "c.free_cancellation=1")
	}
	if b := s.Bounds; b != nil {
		clauses = append(clauses, "c.longitude BETWEEN ? AND ? AND c.latitude BETWEEN ? AND ?")
		args = append(args, b[0], b[2], b[1], b[3])
	}
	order := map[string]string{"price_asc": "c.price_amount ASC", "price_desc": "c.price_amount DESC", "rating": "c.rating_average DESC", "duration": "c.duration_days ASC"}[s.Sort]
	if order == "" {
		order = "c.popularity DESC"
	}
	items, err := queryJSON[Card](ctx, c, "SELECT c.body FROM product_cards c JOIN destinations d ON d.id=c.destination_id WHERE "+strings.Join(clauses, " AND ")+" ORDER BY "+order+",c.id", args...)
	if err != nil || s.Dates == nil {
		return items, err
	}
	filtered := []Card{}
	for _, card := range items {
		p, err := c.Product(ctx, card.ID)
		if err != nil {
			return nil, err
		}
		if c.matchesDates(p, s) {
			filtered = append(filtered, card)
		}
	}
	return filtered, nil
}

func (c *Catalog) matchesDates(p Product, s Search) bool {
	if p.Price.Unit == "person" && s.Quantity != 1 {
		return false
	}
	if p.Type == "stay" {
		return c.Availability(p, *s.Dates, s.Adults, s.Children, s.Quantity).Available
	}
	for start := s.Dates.Start; start.Before(s.Dates.End); start = start.AddDate(0, 0, 1) {
		d := DateRange{start, start.AddDate(0, 0, p.DurationDays)}
		if c.Availability(p, d, s.Adults, s.Children, s.Quantity).Available {
			return true
		}
	}
	return false
}

type Facet struct {
	Value string `json:"value"`
	Count int    `json:"count"`
}
type Facets struct {
	Types        []Facet `json:"types"`
	Destinations []Facet `json:"destinations"`
	Tags         []Facet `json:"tags"`
	Amenities    []Facet `json:"amenities"`
	MinPrice     int     `json:"min_price"`
	MaxPrice     int     `json:"max_price"`
}

func CountFacets(cards []Card) Facets {
	f := Facets{Types: []Facet{}, Destinations: []Facet{}, Tags: []Facet{}, Amenities: []Facet{}}
	maps := []map[string]int{{}, {}, {}, {}}
	for i, p := range cards {
		maps[0][p.Type]++
		maps[1][p.DestinationID]++
		for _, v := range p.Tags {
			maps[2][v]++
		}
		for _, v := range p.Amenities {
			maps[3][v]++
		}
		if i == 0 || p.Price.Amount < f.MinPrice {
			f.MinPrice = p.Price.Amount
		}
		if p.Price.Amount > f.MaxPrice {
			f.MaxPrice = p.Price.Amount
		}
	}
	outputs := []*[]Facet{&f.Types, &f.Destinations, &f.Tags, &f.Amenities}
	for i, m := range maps {
		for k, v := range m {
			*outputs[i] = append(*outputs[i], Facet{k, v})
		}
		sort.Slice(*outputs[i], func(a, b int) bool { return (*outputs[i])[a].Value < (*outputs[i])[b].Value })
	}
	return f
}

type DateRange struct{ Start, End time.Time }

func (d DateRange) Nights() int { return int(d.End.Sub(d.Start).Hours() / 24) }

// Calendar rows are a deterministic synthetic inventory shared by every endpoint.
type CalendarDay struct {
	Date      string `json:"date"`
	Available bool   `json:"available"`
	Remaining int    `json:"remaining"`
	Price     Price  `json:"price"`
	Status    string `json:"status"`
}

func inventory(id string, date time.Time) int {
	h := fnv.New32a()
	_, _ = h.Write([]byte(id + date.Format("2006-01-02")))
	return int(h.Sum32() % 9)
}

func (c *Catalog) Calendar(p Product, d DateRange) []CalendarDay {
	rows := []CalendarDay{}
	for day := d.Start; day.Before(d.End); day = day.AddDate(0, 0, 1) {
		left := 0
		dayPrice := p.Price
		priced := false
		for _, opt := range p.Options() {
			n := inventory(opt.ID, day)
			if n > left {
				left = n
			}
			if n > 0 && (!priced || opt.Price.Amount < dayPrice.Amount) {
				dayPrice = opt.Price
				priced = true
			}
		}
		status := "available"
		if left == 0 {
			status = "sold_out"
		} else if p.Type == "tour" {
			if day.Day()%2 == 0 {
				status = "guaranteed"
			} else {
				status = "forming"
			}
		}
		rows = append(rows, CalendarDay{day.Format("2006-01-02"), left > 0, left, dayPrice, status})
	}
	return rows
}

type Quote struct {
	OptionID    string `json:"option_id"`
	Name        string `json:"name"`
	Capacity    int    `json:"capacity"`
	Remaining   int    `json:"remaining"`
	Available   bool   `json:"available"`
	UnitPrice   Price  `json:"unit_price"`
	Multiplier  int    `json:"multiplier"`
	TotalAmount int    `json:"total_amount"`
}

type Availability struct {
	ProductID  string  `json:"product_id"`
	StartDate  string  `json:"start_date"`
	EndDate    string  `json:"end_date"`
	Nights     int     `json:"nights"`
	Adults     int     `json:"adults"`
	Children   int     `json:"children"`
	Quantity   int     `json:"quantity"`
	Available  bool    `json:"available"`
	Currency   string  `json:"currency"`
	Options    []Quote `json:"options"`
	Disclaimer string  `json:"disclaimer"`
}

func (c *Catalog) Availability(p Product, d DateRange, adults, children, quantity int) Availability {
	a := Availability{ProductID: p.ID, StartDate: d.Start.Format("2006-01-02"), EndDate: d.End.Format("2006-01-02"), Nights: d.Nights(), Adults: adults, Children: children, Quantity: quantity, Currency: "TWD", Options: []Quote{}, Disclaimer: "模擬報價，不保留庫存，也不建立訂單。"}
	for _, opt := range p.Options() {
		left := inventory(opt.ID, d.Start)
		if p.Type == "stay" {
			for day := d.Start.AddDate(0, 0, 1); day.Before(d.End); day = day.AddDate(0, 0, 1) {
				if n := inventory(opt.ID, day); n < left {
					left = n
				}
			}
		}
		multiplier := quantity
		if p.Type == "stay" {
			multiplier *= d.Nights()
		} else if opt.Price.Unit == "person" {
			multiplier = adults + children
		}
		available := left >= quantity && opt.Capacity*quantity >= adults+children
		if opt.Price.Unit == "person" {
			available = left >= adults+children && opt.Capacity >= adults+children
		}
		a.Options = append(a.Options, Quote{opt.ID, opt.Name, opt.Capacity, left, available, opt.Price, multiplier, opt.Price.Amount * multiplier})
		a.Available = a.Available || available
	}
	return a
}
