// Package catalog defines travel catalog response types.
package catalog

type Image struct {
	URL    string `json:"url"`
	Alt    string `json:"alt"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

type Price struct {
	Amount         int    `json:"amount"`
	OriginalAmount int    `json:"original_amount"`
	Currency       string `json:"currency"`
	Unit           string `json:"unit"`
	TaxIncluded    bool   `json:"tax_included"`
}

type Destination struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	CountryCode string  `json:"country_code"`
	CountryName string  `json:"country_name"`
	Description string  `json:"description"`
	Latitude    float64 `json:"latitude"`
	Longitude   float64 `json:"longitude"`
	Image       Image   `json:"image"`
}

type Category struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Image       Image  `json:"image"`
}

// Option describes a room, cabin, or bookable plan. Inventory is calculated per date.
type Option struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Capacity    int      `json:"capacity"`
	Description string   `json:"description"`
	Amenities   []string `json:"amenities"`
	Image       Image    `json:"image"`
	Price       Price    `json:"price"`
}

type Day struct {
	Day           int      `json:"day"`
	Title         string   `json:"title"`
	Description   string   `json:"description"`
	Meals         []string `json:"meals"`
	Accommodation string   `json:"accommodation"`
}

type StayDetails struct {
	PropertyType string   `json:"property_type"`
	CheckInTime  string   `json:"check_in_time"`
	CheckOutTime string   `json:"check_out_time"`
	Rooms        []Option `json:"rooms"`
}

type CruiseDetails struct {
	ShipName      string   `json:"ship_name"`
	CruiseLine    string   `json:"cruise_line"`
	DeparturePort string   `json:"departure_port"`
	Route         string   `json:"route"`
	Cabins        []Option `json:"cabins"`
	Itinerary     []Day    `json:"itinerary"`
}

type TourDetails struct {
	DepartureCity    string   `json:"departure_city"`
	Transport        string   `json:"transport"`
	MinimumTravelers int      `json:"minimum_travelers"`
	Plans            []Option `json:"plans"`
	Itinerary        []Day    `json:"itinerary"`
}

type ActivityDetails struct {
	MeetingPoint      string   `json:"meeting_point"`
	VoucherType       string   `json:"voucher_type"`
	UsageInstructions []string `json:"usage_instructions"`
	Plans             []Option `json:"plans"`
}

type Rating struct {
	Average float64 `json:"average"`
	Count   int     `json:"count"`
}

type Card struct {
	ID               string   `json:"id"`
	Type             string   `json:"type"`
	Title            string   `json:"title"`
	Summary          string   `json:"summary"`
	DestinationID    string   `json:"destination_id"`
	LocationName     string   `json:"location_name"`
	Latitude         float64  `json:"latitude"`
	Longitude        float64  `json:"longitude"`
	Cover            Image    `json:"cover"`
	Price            Price    `json:"price"`
	Rating           Rating   `json:"rating"`
	Tags             []string `json:"tags"`
	Amenities        []string `json:"amenities"`
	DurationDays     int      `json:"duration_days"`
	Popularity       int      `json:"popularity"`
	FreeCancellation bool     `json:"free_cancellation"`
	Confirmation     string   `json:"confirmation"`
	DetailURL        string   `json:"detail_url"`
}

type Product struct {
	Card
	Images             []Image          `json:"images"`
	Description        string           `json:"description"`
	Address            string           `json:"address"`
	Highlights         []string         `json:"highlights"`
	Included           []string         `json:"included"`
	Excluded           []string         `json:"excluded"`
	CancellationPolicy string           `json:"cancellation_policy"`
	Notices            []string         `json:"notices"`
	Stay               *StayDetails     `json:"stay,omitempty"`
	Cruise             *CruiseDetails   `json:"cruise,omitempty"`
	Tour               *TourDetails     `json:"tour,omitempty"`
	Activity           *ActivityDetails `json:"activity,omitempty"`
}

func (p Product) Options() []Option {
	switch {
	case p.Stay != nil:
		return p.Stay.Rooms
	case p.Cruise != nil:
		return p.Cruise.Cabins
	case p.Tour != nil:
		return p.Tour.Plans
	default:
		return p.Activity.Plans
	}
}

func (p Product) Itinerary() []Day {
	if p.Cruise != nil {
		return p.Cruise.Itinerary
	}
	if p.Tour != nil {
		return p.Tour.Itinerary
	}
	return []Day{}
}

type Collection struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Image       Image    `json:"image"`
	ProductIDs  []string `json:"product_ids"`
}

type Campaign struct {
	ID             string   `json:"id"`
	Title          string   `json:"title"`
	Description    string   `json:"description"`
	Image          Image    `json:"image"`
	CouponCode     string   `json:"coupon_code"`
	DiscountAmount int      `json:"discount_amount"`
	MinimumSpend   int      `json:"minimum_spend"`
	CollectionID   string   `json:"collection_id"`
	Terms          []string `json:"terms"`
}

type Article struct {
	ID                string   `json:"id"`
	Title             string   `json:"title"`
	Summary           string   `json:"summary"`
	DestinationID     string   `json:"destination_id"`
	Image             Image    `json:"image"`
	Content           []string `json:"content"`
	RelatedProductIDs []string `json:"related_product_ids"`
}

type FAQ struct {
	ID       string `json:"id"`
	Category string `json:"category"`
	Question string `json:"question"`
	Answer   string `json:"answer"`
}

type Review struct {
	ID         string `json:"id"`
	ProductID  string `json:"product_id"`
	Author     string `json:"author"`
	Rating     int    `json:"rating"`
	Content    string `json:"content"`
	TravelDate string `json:"travel_date"`
	CreatedAt  string `json:"created_at"`
}
