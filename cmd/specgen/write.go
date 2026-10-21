package main

import (
	"reflect"
	"strconv"
	"strings"

	"github.com/aqutw/travel-golang-api/internal/catalog"
	"github.com/aqutw/travel-golang-api/internal/httpapi"
	"github.com/aqutw/travel-golang-api/internal/workspace"
)

// requestSchema derives validation constraints from the same binding tags Gin uses.
func requestSchema(t reflect.Type) object {
	if t.Kind() == reflect.Struct {
		props := object{}
		required := []string{}
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if f.Anonymous {
				base := requestSchema(f.Type)
				for key, value := range base["properties"].(object) {
					props[key] = value
				}
				required = append(required, base["required"].([]string)...)
				continue
			}
			key := strings.Split(f.Tag.Get("json"), ",")[0]
			if key == "-" {
				continue
			}
			shape := requestSchema(f.Type)
			for _, rule := range strings.Split(f.Tag.Get("binding"), ",") {
				if rule == "dive" {
					break
				}
				parts := strings.SplitN(rule, "=", 2)
				switch parts[0] {
				case "required":
					required = append(required, key)
				case "notblank":
					shape["pattern"] = `\S`
				case "datetime":
					shape["format"] = "date"
				case "unique":
					shape["uniqueItems"] = true
				case "min", "max":
					if len(parts) == 2 {
						n, _ := strconv.Atoi(parts[1])
						name := map[reflect.Kind][2]string{reflect.String: {"minLength", "maxLength"}, reflect.Slice: {"minItems", "maxItems"}, reflect.Int: {"minimum", "maximum"}}[f.Type.Kind()]
						index := 0
						if parts[0] == "max" {
							index = 1
						}
						if name[index] != "" {
							shape[name[index]] = n
						}
					}
				}
			}
			// Product identifiers in a string array use required,max=64 after dive.
			if f.Type.Kind() == reflect.Slice && f.Type.Elem().Kind() == reflect.String {
				shape["items"] = object{"type": "string", "minLength": 1, "maxLength": 64}
			}
			props[key] = shape
		}
		return object{"type": "object", "additionalProperties": false, "properties": props, "required": required}
	}
	if t.Kind() == reflect.Slice {
		return object{"type": "array", "items": requestSchema(t.Elem())}
	}
	return schema(t)
}
func addWritePaths(paths object) {
	for _, value := range []any{httpapi.WishlistInput{}, httpapi.WishlistRequest{}, httpapi.TripInput{}, httpapi.TripRequest{}, httpapi.ReviewRequest{}, httpapi.QuoteRequest{}} {
		t := reflect.TypeOf(value)
		schemas[t.Name()] = requestSchema(t)
	}
	valueSchema(workspace.Document{})
	dp := schemas["Document"].(object)["properties"].(object)
	dp["content"] = object{"oneOf": []object{{"$ref": "#/components/schemas/WishlistInput"}, {"$ref": "#/components/schemas/TripInput"}}}
	dp["kind"] = enum("wishlist", "trip")
	dp["version"] = object{"type": "integer", "minimum": 1}
	for _, key := range []string{"created_at", "updated_at"} {
		dp[key] = object{"type": "string", "format": "date-time"}
	}
	review := valueSchema(workspace.ReviewRecord{})
	response := func(data object) object {
		return object{"type": "object", "additionalProperties": false, "required": []string{"data", "meta"}, "properties": object{"data": data, "meta": valueSchema(httpapi.Meta{})}}
	}
	operation := func(method, path, summary string, data, body object, protected bool) {
		params := []object{}
		for _, key := range []string{"id", "review_id"} {
			if strings.Contains(path, "{"+key+"}") {
				params = append(params, object{"name": key, "in": "path", "required": true, "schema": object{"type": "string"}})
			}
		}
		if method == "post" && protected {
			params = append(params, object{"name": "Idempotency-Key", "in": "header", "required": true, "schema": object{"type": "string", "pattern": `^[A-Za-z0-9._-]{8,128}$`}, "description": "相同路徑、key 與 body 會重播既有結果；不同 body 回傳 409。"})
		}
		responses := object{}
		status := "200"
		if method == "post" && protected {
			status = "201"
		}
		responses[status] = object{"description": "成功", "content": object{"application/json": object{"schema": response(data)}}, "headers": object{"X-Request-ID": object{"schema": object{"type": "string"}}}}
		if method == "post" && protected {
			responses[status].(object)["headers"].(object)["Location"] = object{"schema": object{"type": "string"}}
			responses[status].(object)["headers"].(object)["Idempotency-Replayed"] = object{"schema": object{"type": "string", "enum": []string{"true"}}}
		}
		for _, code := range []string{"400", "401", "403", "404", "409", "413", "415", "429", "500", "503"} {
			responses[code] = object{"description": map[string]string{"400": "JSON 欄位、日期或關聯商品無效。", "401": "缺少或錯誤的 X-API-Key。", "403": "Origin 不允許。", "404": "資源不存在。", "409": "版本或 Idempotency-Key 衝突。", "413": "JSON body 超過 64 KiB。", "415": "需要 application/json。", "429": "寫入頻率超限，參考 Retry-After。", "500": "資料庫操作失敗。", "503": "尚未設定 API_WRITE_KEY。"}[code], "content": object{"application/json": object{"schema": valueSchema(httpapi.ErrorEnvelope{})}}}
		}
		op := object{"summary": summary, "operationId": method + "_" + strings.NewReplacer("/", "_", "{", "", "}", "").Replace(strings.TrimPrefix(path, "/")), "parameters": params, "responses": responses}
		if protected {
			op["security"] = []object{{"WorkspaceKey": []string{}}}
		}
		if body != nil {
			if method == "post" && protected {
				body = object{"allOf": []object{body, {"properties": object{"version": object{"type": "integer", "maximum": 0}}}}}
			}
			if method == "put" {
				body = object{"allOf": []object{body, {"required": []string{"version"}, "properties": object{"version": object{"type": "integer", "minimum": 1}}}}}
			}
			op["requestBody"] = object{"required": true, "content": object{"application/json": object{"schema": body}}}
		}
		if method == "get" && !strings.Contains(path, "{id}") {
			op["x-smoke-path"] = path
			params = append(params, object{"$ref": "#/components/parameters/page"}, object{"$ref": "#/components/parameters/per_page"})
			op["parameters"] = params
		}
		if existing, ok := paths[path]; ok {
			existing.(object)[method] = op
		} else {
			paths[path] = object{method: op}
		}
	}
	for _, resource := range []struct{ path, name, request string }{{"wishlists", "收藏清單", "WishlistRequest"}, {"trips", "旅遊行程", "TripRequest"}} {
		path := "/api/v1/" + resource.path
		operation("get", path, "查詢"+resource.name, object{"type": "array", "items": valueSchema(workspace.Document{})}, nil, true)
		operation("get", path+"/{id}", "取得"+resource.name, valueSchema(workspace.Document{}), nil, true)
		operation("post", path, "建立"+resource.name, valueSchema(workspace.Document{}), object{"$ref": "#/components/schemas/" + resource.request}, true)
		operation("put", path+"/{id}", "完整取代"+resource.name, valueSchema(workspace.Document{}), object{"$ref": "#/components/schemas/" + resource.request}, true)
	}
	operation("post", "/api/v1/products/{id}/reviews", "建立商品評價", review, object{"$ref": "#/components/schemas/ReviewRequest"}, true)
	operation("put", "/api/v1/products/{id}/reviews/{review_id}", "完整取代 API 建立的評價", review, object{"$ref": "#/components/schemas/ReviewRequest"}, true)
	operation("get", "/api/v1/products/{id}/reviews/{review_id}", "取得可編輯評價與版本", review, nil, true)
	operation("post", "/api/v1/products/{id}/quotes", "以 JSON 查詢方案容量與報價", valueSchema(catalog.Availability{}), object{"$ref": "#/components/schemas/QuoteRequest"}, false)
}
