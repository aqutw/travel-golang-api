// specgen derives response schemas from the actual Go response types.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"

	"github.com/aqutw/travel-golang-api/internal/catalog"
	"github.com/aqutw/travel-golang-api/internal/httpapi"
)

type object = map[string]any

var schemas = object{}

func schema(t reflect.Type) object {
	if t == reflect.TypeOf(json.RawMessage{}) {
		return object{}
	}
	if t.Kind() == reflect.Pointer {
		return schema(t.Elem())
	}
	switch t.Kind() {
	case reflect.String:
		return object{"type": "string"}
	case reflect.Int, reflect.Int64:
		return object{"type": "integer"}
	case reflect.Float64:
		return object{"type": "number"}
	case reflect.Bool:
		return object{"type": "boolean"}
	case reflect.Slice:
		return object{"type": "array", "items": schema(t.Elem())}
	case reflect.Map:
		return object{"type": "object", "additionalProperties": schema(t.Elem())}
	case reflect.Interface:
		return object{}
	case reflect.Struct:
		name := t.Name()
		if name != "" {
			if _, ok := schemas[name]; ok {
				return object{"$ref": "#/components/schemas/" + name}
			}
			schemas[name] = object{}
		}
		properties := object{}
		required := []string{}
		for i := 0; i < t.NumField(); i++ {
			field := t.Field(i)
			if field.Anonymous {
				schema(field.Type)
				base := schemas[field.Type.Name()].(object)
				for k, v := range base["properties"].(object) {
					properties[k] = v
				}
				required = append(required, base["required"].([]string)...)
				continue
			}
			tag := strings.Split(field.Tag.Get("json"), ",")
			key := tag[0]
			if key == "-" {
				continue
			}
			if key == "" {
				key = field.Name
			}
			properties[key] = schema(field.Type)
			if len(tag) == 1 || tag[1] != "omitempty" {
				required = append(required, key)
			}
		}
		result := object{"type": "object", "properties": properties, "required": required, "additionalProperties": false}
		if name != "" {
			schemas[name] = result
			return object{"$ref": "#/components/schemas/" + name}
		}
		return result
	default:
		panic("unsupported response type: " + t.String())
	}
}
func valueSchema(v any) object { return schema(reflect.TypeOf(v)) }
func array(v any) object       { return object{"type": "array", "items": valueSchema(v)} }

type endpoint struct {
	path, summary, example, params string
	data                           object
	list                           bool
}

func param(name, description string, s object) object {
	return object{"name": name, "in": "query", "description": description, "schema": s}
}
func integer(min, max, def int) object {
	return object{"type": "integer", "minimum": min, "maximum": max, "default": def}
}
func enum(values ...string) object { return object{"type": "string", "enum": values} }
func main() {
	common := []object{param("locale", "僅提供繁體中文。", enum("zh-TW")), param("currency", "價格以整數新台幣元表示，不進行匯率換算。", enum("TWD"))}
	params := object{
		"page":              param("page", "從 1 開始。超出資料頁數回傳空陣列。", integer(1, 1000000, 1)),
		"per_page":          param("per_page", "每頁筆數。", integer(1, 100, 12)),
		"q":                 param("q", "不分英文大小寫的部分文字搜尋。", object{"type": "string"}),
		"type":              param("type", "商品品類。", enum("stay", "cruise", "tour", "activity", "transfer", "esim", "sport", "rail")),
		"destination_id":    param("destination_id", "目的地 ID，例如 tokyo。", object{"type": "string"}),
		"country":           param("country", "國家代碼。", enum("TW", "JP", "KR", "TH", "SG")),
		"tag":               param("tag", "單一標籤精確比對，例如 親子友善、露營、包棟。", object{"type": "string"}),
		"amenity":           param("amenity", "設施代碼，例如 wifi、breakfast、parking、hot_spring。", object{"type": "string"}),
		"sort":              param("sort", "同分以商品 ID 升冪排序。", object{"type": "string", "enum": []string{"popular", "price_asc", "price_desc", "rating", "duration"}, "default": "popular"}),
		"min_price":         param("min_price", "最低卡片起價（TWD 元）。", integer(0, 10000000, 0)),
		"max_price":         param("max_price", "最高卡片起價（TWD 元）。", integer(0, 10000000, 10000000)),
		"min_rating":        param("min_rating", "最低評分。", object{"type": "number", "minimum": 0, "maximum": 5, "default": 0}),
		"min_days":          param("min_days", "最少商品行程天數。", integer(1, 30, 1)),
		"max_days":          param("max_days", "最多商品行程天數。", integer(1, 30, 30)),
		"free_cancellation": param("free_cancellation", "true 僅顯示可免費取消的商品。", object{"type": "boolean", "default": false}),
		"bbox":              param("bbox", "west,south,east,north，經緯度十進位。不支援跨越國際換日線。", object{"type": "string", "example": "121,24,122,26"}),
		"start_date":        param("start_date", "YYYY-MM-DD，台北時間；預設 reference_date+7，允許 reference_date 至 +365 天。", object{"type": "string", "format": "date"}),
		"end_date":          param("end_date", "不含此日。搜尋必須同時提供起訖日期（最多 30 天）；calendar/departures 最多 90 天。", object{"type": "string", "format": "date"}),
		"adults":            param("adults", "成人數，預設 2。搜尋人數條件必須搭配日期。", integer(1, 16, 2)),
		"children":          param("children", "兒童數，示範價格與成人相同，並計入容量。", integer(0, 16, 0)),
		"quantity":          param("quantity", "房、艙、車、上網方案份數。按人計價需為 1，以 adults+children 計價。", integer(1, 8, 1)),
		"departure_port":    param("departure_port", "郵輪出發港精確比對，例如 基隆港。", object{"type": "string"}),
		"departure_city":    param("departure_city", "跟團出發城市精確比對，例如 台北。", object{"type": "string"}),
		"cruise_line":       param("cruise_line", "郵輪品牌精確比對，值由 cruise/regions 取得。", object{"type": "string"}),
		"property_type":     param("property_type", "住宿類型精確比對。", enum("hotel", "hot_spring", "camping", "entire_house")),
		"limit":             param("limit", "自動完成回傳筆數。", integer(1, 20, 8)),
		"status":            param("status", "每日出發狀態。guaranteed 與 forming 僅用於跟團。", enum("available", "guaranteed", "forming", "sold_out")),
		"rating":            param("rating", "評價星數精確比對。不提供此參數時顯示全部評價。", object{"type": "integer", "minimum": 1, "maximum": 5}),
		"category":          param("category", "FAQ 類別，例如 general、stay、cruise、tour、activity。", object{"type": "string"}),
	}
	search := "q type destination_id country tag amenity sort min_price max_price min_rating min_days max_days free_cancellation bbox start_date end_date adults children quantity page per_page departure_port departure_city cruise_line property_type"
	valueSchema(httpapi.Meta{})
	valueSchema(httpapi.ErrorEnvelope{})
	config := object{"type": "object", "required": []string{"name", "api_version", "locales", "currencies", "timezone", "product_types", "sort_options", "price_units", "limits", "notice"}, "properties": object{
		"name": object{"type": "string"}, "api_version": enum("v1"), "locales": object{"type": "array", "items": enum("zh-TW")}, "currencies": object{"type": "array", "items": enum("TWD")}, "timezone": enum("Asia/Taipei"), "product_types": array(catalog.Category{}), "sort_options": array(httpapi.Link{}), "price_units": object{"type": "object", "additionalProperties": object{"type": "string"}}, "limits": object{"type": "object", "additionalProperties": object{"type": "integer"}}, "notice": object{"type": "string"}}}
	endpoints := []endpoint{
		{"/healthz", "健康檢查", "/healthz", "", object{"type": "object", "required": []string{"status"}, "properties": object{"status": enum("ok")}}, false},
		{"/api/v1/config", "前端設定與選項", "/api/v1/config", "", config, false},
		{"/api/v1/navigation", "主導覽與頁尾", "/api/v1/navigation", "", valueSchema(httpapi.Navigation{}), false},
		{"/api/v1/home", "首頁區塊聚合", "/api/v1/home", "", valueSchema(httpapi.Home{}), false},
		{"/api/v1/categories", "旅遊品類捷徑", "/api/v1/categories", "", array(catalog.Category{}), false},
		{"/api/v1/cruise/regions", "郵輪港口、航區與品牌", "/api/v1/cruise/regions", "", valueSchema(httpapi.CruiseRegions{}), false},
		{"/api/v1/destinations", "目的地列表", "/api/v1/destinations", "q country page per_page", array(catalog.Destination{}), true},
		{"/api/v1/destinations/{id}", "目的地探索頁", "/api/v1/destinations/tokyo", "", valueSchema(httpapi.DestinationPage{}), false},
		{"/api/v1/search/suggestions", "目的地與商品自動完成", "/api/v1/search/suggestions?q=東京", "q type limit", array(httpapi.Suggestion{}), false},
		{"/api/v1/search/filters", "目前查詢結果的篩選計數", "/api/v1/search/filters?type=stay", search, valueSchema(catalog.Facets{}), false},
		{"/api/v1/products", "商品搜尋結果", "/api/v1/products?type=stay&sort=price_asc", search, array(catalog.Card{}), true},
		{"/api/v1/products/map", "商品地圖點位", "/api/v1/products/map?bbox=121,24,122,26", search, array(httpapi.MapPoint{}), true},
		{"/api/v1/products/{id}", "商品完整詳情", "/api/v1/products/cruise-001", "", valueSchema(catalog.Product{}), false},
		{"/api/v1/products/{id}/rooms", "住宿房型", "/api/v1/products/stay-001/rooms", "", array(catalog.Option{}), false},
		{"/api/v1/products/{id}/cabins", "郵輪艙房", "/api/v1/products/cruise-001/cabins", "", array(catalog.Option{}), false},
		{"/api/v1/products/{id}/plans", "跟團、體驗與交通方案", "/api/v1/products/activity-001/plans", "", array(catalog.Option{}), false},
		{"/api/v1/products/{id}/itinerary", "郵輪與跟團每日行程", "/api/v1/products/tour-001/itinerary", "", array(catalog.Day{}), false},
		{"/api/v1/products/{id}/calendar", "每日起價與庫存日曆", "/api/v1/products/stay-001/calendar", "start_date end_date", array(catalog.CalendarDay{}), false},
		{"/api/v1/products/{id}/departures", "郵輪與跟團出發日期", "/api/v1/products/tour-001/departures", "start_date end_date page per_page status", array(catalog.CalendarDay{}), true},
		{"/api/v1/products/{id}/availability", "方案容量、庫存與報價", "/api/v1/products/stay-001/availability", "start_date end_date adults children quantity", valueSchema(catalog.Availability{}), false},
		{"/api/v1/products/{id}/reviews", "商品旅客評價", "/api/v1/products/stay-001/reviews", "page per_page rating", array(catalog.Review{}), true},
		{"/api/v1/products/{id}/related", "相關商品推薦", "/api/v1/products/stay-001/related", "page per_page", array(catalog.Card{}), true},
		{"/api/v1/collections", "主題集合列表", "/api/v1/collections", "page per_page", array(catalog.Collection{}), true},
		{"/api/v1/collections/{id}", "主題集合內容", "/api/v1/collections/camping", "", valueSchema(catalog.Collection{}), false},
		{"/api/v1/collections/{id}/products", "集合內的商品搜尋", "/api/v1/collections/camping/products", search, array(catalog.Card{}), true},
		{"/api/v1/campaigns", "優惠活動列表", "/api/v1/campaigns", "page per_page", array(catalog.Campaign{}), true},
		{"/api/v1/campaigns/{id}", "優惠條件與折扣碼", "/api/v1/campaigns/escape-stay", "", valueSchema(catalog.Campaign{}), false},
		{"/api/v1/articles", "旅遊內容列表", "/api/v1/articles", "destination_id page per_page", array(catalog.Article{}), true},
		{"/api/v1/articles/{id}", "旅遊文章詳情", "/api/v1/articles/guide-taipei", "", valueSchema(catalog.Article{}), false},
		{"/api/v1/faqs", "常見問題搜尋", "/api/v1/faqs", "q category page per_page", array(catalog.FAQ{}), true},
	}
	// Add semantic constraints to schemas derived from Go field shapes.
	for name, fields := range map[string]map[string]object{
		"Price": {"currency": enum("TWD"), "unit": enum("night", "person", "vehicle", "package"), "amount": {"type": "integer", "minimum": 0}, "original_amount": {"type": "integer", "minimum": 0}},
		"Card":  {"type": enum("stay", "cruise", "tour", "activity", "transfer", "esim", "sport", "rail"), "confirmation": enum("instant", "request")},
		"Meta":  {"locale": enum("zh-TW"), "currency": enum("TWD"), "reference_date": {"type": "string", "format": "date"}, "mock": {"type": "boolean", "enum": []bool{true}}},
	} {
		props := schemas[name].(object)["properties"].(object)
		for field, s := range fields {
			props[field] = s
		}
	}
	schemas["Product"].(object)["properties"].(object)["type"] = enum("stay", "cruise", "tour", "activity", "transfer", "esim", "sport", "rail")
	variants := []object{}
	for _, v := range []struct {
		name  string
		types []string
	}{{"stay", []string{"stay"}}, {"cruise", []string{"cruise"}}, {"tour", []string{"tour"}}, {"activity", []string{"activity", "transfer", "esim", "sport", "rail"}}} {
		variants = append(variants, object{"required": []string{v.name}, "properties": object{"type": object{"enum": v.types}}})
	}
	schemas["Product"].(object)["oneOf"] = variants
	paths := object{}
	for _, e := range endpoints {
		ps := append([]object{}, common...)
		if strings.Contains(e.path, "{id}") {
			ps = append(ps, object{"name": "id", "in": "path", "required": true, "schema": object{"type": "string"}})
		}
		for _, key := range strings.Fields(e.params) {
			ps = append(ps, object{"$ref": "#/components/parameters/" + key})
		}
		meta := valueSchema(httpapi.Meta{})
		if e.list {
			meta = object{"allOf": []object{meta, {"required": []string{"pagination"}}}}
		}
		response := object{"type": "object", "required": []string{"data", "meta"}, "additionalProperties": false, "properties": object{"data": e.data, "meta": meta}}
		responses := object{"200": object{"description": "成功；所有內容為 mock 資料。", "content": object{"application/json": object{"schema": response}}}}
		for _, status := range []string{"400", "404", "405", "500"} {
			responses[status] = object{"description": map[string]string{"400": "查詢參數無效。未知參數、重複參數與不支援語系回傳 400。", "404": "資源不存在或不適用該商品品類。", "405": "該路由不支援此 HTTP 方法。", "500": "資料庫讀取失敗。"}[status], "content": object{"application/json": object{"schema": valueSchema(httpapi.ErrorEnvelope{})}}}
		}
		paths[e.path] = object{"get": object{"summary": e.summary, "operationId": strings.NewReplacer("/", "_", "{", "", "}", "").Replace(strings.TrimPrefix(e.path, "/")), "parameters": ps, "responses": responses, "x-smoke-path": e.example}}
	}
	addWritePaths(paths)
	doc := object{"openapi": "3.0.3", "info": object{"title": "Travel Gin API", "version": "2.0.0", "description": "供旅遊前端開發使用的示範 API。所有文字、圖片、價格與庫存為虛構。資料以服務啟動時的 reference_date 為基準。"}, "servers": []object{{"url": "/"}}, "paths": paths, "components": object{"schemas": schemas, "parameters": params, "securitySchemes": object{"WorkspaceKey": object{"type": "apiKey", "in": "header", "name": "X-API-Key", "description": "單一 workspace 的受信任服務端金鑰；以 API_WRITE_KEY 設定。"}}}}
	b, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		panic(err)
	}
	if err = os.WriteFile("internal/assets/openapi.json", append(b, '\n'), 0644); err != nil {
		panic(err)
	}
	fmt.Printf("Generated OpenAPI: %d paths, %d schemas\n", len(paths), len(schemas))
}
