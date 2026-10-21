package httpapi

import (
	"encoding/json"
	"github.com/aqutw/travel-golang-api/internal/catalog"
	"github.com/gin-gonic/gin"
	"sort"
	"strings"
)

type CruiseRegions struct {
	PortRegions  []string `json:"port_regions"`
	RouteRegions []Link   `json:"route_regions"`
	CruiseLines  []string `json:"cruise_lines"`
}

type Link struct {
	Label string `json:"label"`
	Href  string `json:"href"`
}
type NavigationGroup struct {
	Title string `json:"title"`
	Links []Link `json:"links"`
}
type Navigation struct {
	Primary []Link            `json:"primary"`
	Footer  []NavigationGroup `json:"footer"`
}

type HomeSection struct {
	ID           string         `json:"id"`
	Title        string         `json:"title"`
	CollectionID string         `json:"collection_id"`
	MoreURL      string         `json:"more_url"`
	Products     []catalog.Card `json:"products"`
}
type Home struct {
	Hero struct {
		Title    string        `json:"title"`
		Subtitle string        `json:"subtitle"`
		Image    catalog.Image `json:"image"`
	} `json:"hero"`
	Navigation   Navigation            `json:"navigation"`
	Categories   []catalog.Category    `json:"categories"`
	Campaigns    []catalog.Campaign    `json:"campaigns"`
	Destinations []catalog.Destination `json:"destinations"`
	Sections     []HomeSection         `json:"sections"`
	Collections  []catalog.Collection  `json:"collections"`
	Articles     []catalog.Article     `json:"articles"`
}

type DestinationPage struct {
	catalog.Destination
	ProductCount     int                  `json:"product_count"`
	FeaturedProducts []catalog.Card       `json:"featured_products"`
	Collections      []catalog.Collection `json:"collections"`
}

type Suggestion struct {
	ID       string        `json:"id"`
	Type     string        `json:"type"`
	Label    string        `json:"label"`
	Subtitle string        `json:"subtitle"`
	Href     string        `json:"href"`
	Image    catalog.Image `json:"image"`
}

func (a *API) cruiseRegions(ctx *gin.Context) {
	w, r := ctx.Writer, ctx.Request
	if !valid(w, newQuery(r)) {
		return
	}
	cards, err := a.catalog.Cards(r.Context())
	if !databaseOK(w, err) {
		return
	}
	destinations, err := a.catalog.Destinations(r.Context())
	if !databaseOK(w, err) {
		return
	}
	ports, lines, ids := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, card := range cards {
		if card.Type == "cruise" {
			p, err := a.catalog.Product(r.Context(), card.ID)
			if !databaseOK(w, err) {
				return
			}
			ports[p.Cruise.DeparturePort] = true
			lines[p.Cruise.CruiseLine] = true
			ids[p.DestinationID] = true
		}
	}
	result := CruiseRegions{PortRegions: []string{}, RouteRegions: []Link{}, CruiseLines: []string{}}
	for port := range ports {
		result.PortRegions = append(result.PortRegions, port)
	}
	sort.Strings(result.PortRegions)
	for line := range lines {
		result.CruiseLines = append(result.CruiseLines, line)
	}
	sort.Strings(result.CruiseLines)
	for _, d := range destinations {
		if ids[d.ID] {
			result.RouteRegions = append(result.RouteRegions, Link{d.Name, d.ID})
		}
	}
	a.reply(w, result)
}
func (a *API) config(ctx *gin.Context) {
	w, r := ctx.Writer, ctx.Request
	if !valid(w, newQuery(r)) {
		return
	}
	var config map[string]any
	if !databaseOK(w, a.catalog.Setting(r.Context(), "config", &config)) {
		return
	}
	categories, err := a.catalog.Categories(r.Context())
	if !databaseOK(w, err) {
		return
	}
	config["product_types"] = categories
	a.reply(w, config)
}
func (a *API) navigation(ctx *gin.Context) {
	w, r := ctx.Writer, ctx.Request
	if !valid(w, newQuery(r)) {
		return
	}
	var data Navigation
	if databaseOK(w, a.catalog.Setting(r.Context(), "navigation", &data)) {
		a.reply(w, data)
	}
}
func (a *API) categories(ctx *gin.Context) {
	w, r := ctx.Writer, ctx.Request
	if !valid(w, newQuery(r)) {
		return
	}
	items, err := a.catalog.Categories(r.Context())
	if databaseOK(w, err) {
		a.reply(w, items)
	}
}
func selectCards(cards []catalog.Card, ids []string) []catalog.Card {
	result := []catalog.Card{}
	for _, id := range ids {
		for _, card := range cards {
			if card.ID == id {
				result = append(result, card)
				break
			}
		}
	}
	return result
}
func take[T any](items []T, n int) []T {
	if len(items) < n {
		return items
	}
	return items[:n]
}
func (a *API) home(ctx *gin.Context) {
	w, r := ctx.Writer, ctx.Request
	if !valid(w, newQuery(r)) {
		return
	}
	var layout struct {
		Hero            json.RawMessage `json:"hero"`
		SectionIDs      []string        `json:"section_collection_ids"`
		DestinationIDs  []string        `json:"destination_ids"`
		ArticleIDs      []string        `json:"article_ids"`
		CardsPerSection int             `json:"cards_per_section"`
	}
	if !databaseOK(w, a.catalog.Setting(r.Context(), "home", &layout)) {
		return
	}
	h := Home{Sections: []HomeSection{}, Destinations: []catalog.Destination{}, Articles: []catalog.Article{}}
	if !databaseOK(w, json.Unmarshal(layout.Hero, &h.Hero)) {
		return
	}
	if !databaseOK(w, a.catalog.Setting(r.Context(), "navigation", &h.Navigation)) {
		return
	}
	var err error
	h.Categories, err = a.catalog.Categories(r.Context())
	if !databaseOK(w, err) {
		return
	}
	h.Campaigns, err = a.catalog.Campaigns(r.Context())
	if !databaseOK(w, err) {
		return
	}
	h.Collections, err = a.catalog.Collections(r.Context())
	if !databaseOK(w, err) {
		return
	}
	destinations, err := a.catalog.Destinations(r.Context())
	if !databaseOK(w, err) {
		return
	}
	articles, err := a.catalog.Articles(r.Context())
	if !databaseOK(w, err) {
		return
	}
	cards, err := a.catalog.Cards(r.Context())
	if !databaseOK(w, err) {
		return
	}
	for _, id := range layout.DestinationIDs {
		for _, d := range destinations {
			if d.ID == id {
				h.Destinations = append(h.Destinations, d)
			}
		}
	}
	for _, id := range layout.ArticleIDs {
		for _, article := range articles {
			if article.ID == id {
				h.Articles = append(h.Articles, article)
			}
		}
	}
	for _, id := range layout.SectionIDs {
		for _, c := range h.Collections {
			if c.ID == id {
				h.Sections = append(h.Sections, HomeSection{c.ID, c.Title, c.ID, "/collections/" + c.ID, take(selectCards(cards, c.ProductIDs), layout.CardsPerSection)})
			}
		}
	}
	a.reply(w, h)
}
func (a *API) destinations(ctx *gin.Context) {
	w, r := ctx.Writer, ctx.Request
	q := newQuery(r, "q", "country", "page", "per_page")
	country := q.enum("country", "", "TW", "JP", "KR", "TH", "SG")
	page, size := q.pagination()
	if !valid(w, q) {
		return
	}
	rows, err := a.catalog.Destinations(r.Context())
	if !databaseOK(w, err) {
		return
	}
	items := []catalog.Destination{}
	for _, d := range rows {
		if (country == "" || country == d.CountryCode) && strings.Contains(strings.ToLower(d.Name+" "+d.ID+" "+d.CountryName), strings.ToLower(q.str("q"))) {
			items = append(items, d)
		}
	}
	listReply(a, w, items, page, size, nil)
}
func (a *API) destination(ctx *gin.Context) {
	w, r := ctx.Writer, ctx.Request
	if !valid(w, newQuery(r)) {
		return
	}
	d, err := a.catalog.Destination(r.Context(), ctx.Param("id"))
	if !resourceOK(w, err, "destination") {
		return
	}
	cards, err := a.catalog.Cards(r.Context())
	if !databaseOK(w, err) {
		return
	}
	allCollections, err := a.catalog.Collections(r.Context())
	if !databaseOK(w, err) {
		return
	}
	items := []catalog.Card{}
	collections := []catalog.Collection{}
	for _, p := range cards {
		if p.DestinationID == d.ID {
			items = append(items, p)
		}
	}
	for _, c := range allCollections {
		if len(selectCards(items, c.ProductIDs)) > 0 {
			collections = append(collections, c)
		}
	}
	a.reply(w, DestinationPage{d, len(items), take(items, 6), collections})
}
func (a *API) suggestions(ctx *gin.Context) {
	w, r := ctx.Writer, ctx.Request
	q := newQuery(r, "q", "type", "limit")
	limit := q.integer("limit", 8, 1, 20)
	kind := q.enum("type", "", "stay", "cruise", "tour", "activity", "transfer", "esim", "sport", "rail")
	if !valid(w, q) {
		return
	}
	destinations, err := a.catalog.Destinations(r.Context())
	if !databaseOK(w, err) {
		return
	}
	cards, err := a.catalog.Cards(r.Context())
	if !databaseOK(w, err) {
		return
	}
	term := strings.ToLower(q.str("q"))
	items := []Suggestion{}
	for _, d := range destinations {
		if strings.Contains(strings.ToLower(d.Name+" "+d.ID+" "+d.CountryName), term) {
			items = append(items, Suggestion{d.ID, "destination", d.Name, d.CountryName, "/products?destination_id=" + d.ID, d.Image})
		}
	}
	for _, p := range cards {
		if (kind == "" || kind == p.Type) && strings.Contains(strings.ToLower(p.Title+" "+p.ID), term) {
			items = append(items, Suggestion{p.ID, "product", p.Title, p.LocationName, "/products/" + p.ID, p.Cover})
		}
	}
	a.reply(w, take(items, limit))
}
func (a *API) collections(ctx *gin.Context) {
	w, r := ctx.Writer, ctx.Request
	q := newQuery(r, "page", "per_page")
	page, size := q.pagination()
	if !valid(w, q) {
		return
	}
	items, err := a.catalog.Collections(r.Context())
	if databaseOK(w, err) {
		listReply(a, w, items, page, size, nil)
	}
}
func (a *API) collection(ctx *gin.Context) {
	w, r := ctx.Writer, ctx.Request
	if !valid(w, newQuery(r)) {
		return
	}
	item, err := a.catalog.Collection(r.Context(), ctx.Param("id"))
	if resourceOK(w, err, "collection") {
		a.reply(w, item)
	}
}
func (a *API) collectionProducts(ctx *gin.Context) {
	w, r := ctx.Writer, ctx.Request
	q, s, page, size := a.searchQuery(r)
	if !valid(w, q) {
		return
	}
	c, err := a.catalog.Collection(r.Context(), ctx.Param("id"))
	if !resourceOK(w, err, "collection") {
		return
	}
	rows, err := a.catalog.Search(r.Context(), s)
	if !databaseOK(w, err) {
		return
	}
	ids := map[string]bool{}
	for _, id := range c.ProductIDs {
		ids[id] = true
	}
	items := []catalog.Card{}
	for _, p := range rows {
		if ids[p.ID] {
			items = append(items, p)
		}
	}
	facets := catalog.CountFacets(items)
	listReply(a, w, items, page, size, &facets)
}
func (a *API) campaigns(ctx *gin.Context) {
	w, r := ctx.Writer, ctx.Request
	q := newQuery(r, "page", "per_page")
	page, size := q.pagination()
	if !valid(w, q) {
		return
	}
	items, err := a.catalog.Campaigns(r.Context())
	if databaseOK(w, err) {
		listReply(a, w, items, page, size, nil)
	}
}
func (a *API) campaign(ctx *gin.Context) {
	w, r := ctx.Writer, ctx.Request
	if !valid(w, newQuery(r)) {
		return
	}
	item, err := a.catalog.Campaign(r.Context(), ctx.Param("id"))
	if resourceOK(w, err, "campaign") {
		a.reply(w, item)
	}
}
func (a *API) articles(ctx *gin.Context) {
	w, r := ctx.Writer, ctx.Request
	q := newQuery(r, "destination_id", "page", "per_page")
	page, size := q.pagination()
	if !valid(w, q) {
		return
	}
	rows, err := a.catalog.Articles(r.Context())
	if !databaseOK(w, err) {
		return
	}
	items := []catalog.Article{}
	for _, v := range rows {
		if q.str("destination_id") == "" || q.str("destination_id") == v.DestinationID {
			items = append(items, v)
		}
	}
	listReply(a, w, items, page, size, nil)
}
func (a *API) article(ctx *gin.Context) {
	w, r := ctx.Writer, ctx.Request
	if !valid(w, newQuery(r)) {
		return
	}
	item, err := a.catalog.Article(r.Context(), ctx.Param("id"))
	if resourceOK(w, err, "article") {
		a.reply(w, item)
	}
}
func (a *API) faqs(ctx *gin.Context) {
	w, r := ctx.Writer, ctx.Request
	q := newQuery(r, "category", "q", "page", "per_page")
	page, size := q.pagination()
	if !valid(w, q) {
		return
	}
	rows, err := a.catalog.FAQs(r.Context())
	if !databaseOK(w, err) {
		return
	}
	items := []catalog.FAQ{}
	for _, v := range rows {
		if (q.str("category") == "" || q.str("category") == v.Category) && strings.Contains(strings.ToLower(v.Question+v.Answer), strings.ToLower(q.str("q"))) {
			items = append(items, v)
		}
	}
	listReply(a, w, items, page, size, nil)
}
