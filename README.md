# Travel Golang API

## 專案目錄與檔案結構

```text
travel-golang-api/
├── cmd/
│   ├── examplegen/main.go             # HTTP request/response 範例產生器
│   ├── server/main.go                 # HTTP server 入口
│   └── specgen/
│       ├── main.go                    # 回應 schema 產生器
│       └── write.go                   # binding tags 與 POST/PUT schema
├── internal/
│   ├── workspace/
│   │   ├── store.go                   # 寫入交易、版本與冪等重試
│   │   ├── store_test.go              # schema migration 測試
│   │   └── migration.sql              # SQLite v1 → v2 migration
│   ├── catalog/
│   │   ├── model.go                   # 資料型別
│   │   ├── catalog.go                 # 搜尋、庫存與價格計算
│   │   ├── database.go                # SQLite 連線與查詢
│   │   └── database_test.go           # 資料庫測試
│   ├── httpapi/
│   │   ├── api.go                     # Gin 路由與回應
│   │   ├── middleware.go              # 認證、限流、CORS、日誌與 recovery
│   │   ├── write.go                   # JSON binding、validation 與寫入
│   │   ├── write_test.go              # 寫入與 middleware 測試
│   │   ├── content.go                 # 頁面內容處理
│   │   ├── query.go                   # 查詢參數解析
│   │   ├── api_test.go                # API 整合測試
│   │   └── sqlite_test.go             # SQLite 與 API 連動測試
│   └── assets/
│       ├── assets.go                  # 靜態資源嵌入
│       ├── index.html                 # API Explorer
│       ├── docs.html                  # Swagger UI
│       └── openapi.json               # OpenAPI 契約
├── data/travel.sqlite3                # 業務資料、頁面設定與圖片
├── db/schema.sql                     # SQLite 資料表定義
├── scripts/
│   ├── generate_fixtures.py           # 資料與圖片產生器
│   ├── build_database.py              # SQLite 建庫工具
│   ├── database_manifest.py           # 資料庫內容比對
│   ├── verify_api.py                  # API 回應驗證
│   ├── check_content.py               # 檔案內容掃描
│   ├── verify_get_compat.py            # GET 遷移前後比對
│   ├── verify_writes.py                # POST/PUT 與私有 GET 驗證
│   └── verify_examples.py              # README 表格、連結與範例契約驗證
├── docs/
│   ├── examples/*.http                # 每個 API 的 request/response 範例
│   ├── api.md
│   ├── architecture.md
│   ├── information-architecture.md
│   └── sqlite.md
├── .github/workflows/ci.yml
├── .dockerignore
├── .gitignore
├── Dockerfile
├── Makefile
├── go.mod
├── go.sum
├── requirements-dev.txt
├── LICENSE
└── README.md
```

## 啟動方式

### 使用 Go

需要 Go 1.25 或以上版本。

```sh
git clone https://github.com/aqutw/travel-golang-api.git
cd travel-golang-api
go run ./cmd/server
```

預設服務位址為 `http://localhost:8080`。

預設讀取專案內的 `data/travel.sqlite3`。指定其他資料庫檔案：

```sh
DB_PATH=/absolute/path/travel.sqlite3 go run ./cmd/server
```

指定監聽位址與日期：

```sh
ADDR=127.0.0.1:8081 REFERENCE_DATE=2026-10-20 go run ./cmd/server
```

啟用收藏、行程及評價寫入，並限制前端 origin：

```sh
export API_WRITE_KEY="$(openssl rand -hex 32)"
CORS_ORIGINS=http://localhost:5173,http://localhost:3000 go run ./cmd/server
```

`API_WRITE_KEY` 至少 32 bytes。受保護的請求以 `X-API-Key` 傳送相同值；未設定時回傳 `503 writes_disabled`。未設定 `CORS_ORIGINS` 時允許所有 origin。

資料庫與所在目錄需要寫入權限。首次啟用寫入時會在同一個資料庫檔案升級 schema。

Swagger UI：`http://localhost:8080/docs`。API Explorer：`http://localhost:8080/`。

執行測試與實際 HTTP 驗證：

```sh
make test race vet
python3 -m venv /tmp/travel-api-venv
/tmp/travel-api-venv/bin/pip install -r requirements-dev.txt
# 先啟動服務，並讓目前 shell 的 API_WRITE_KEY 與服務一致：
/tmp/travel-api-venv/bin/python scripts/verify_api.py
/tmp/travel-api-venv/bin/python scripts/verify_writes.py
```

`verify_writes.py` 會建立收藏、行程及評價，可搭配測試資料庫執行。

重新產生 README API 表格與範例，並驗證全部範例：

```sh
make examples
/tmp/travel-api-venv/bin/python scripts/verify_examples.py
```

產生範例時使用暫存資料庫，不修改 `data/travel.sqlite3`。

### 使用 Docker

```sh
docker build -t travel-golang-api .
export API_WRITE_KEY="$(openssl rand -hex 32)"
docker volume create travel-api-data
docker run --rm -p 8080:8080 \
  -e API_WRITE_KEY \
  --mount type=volume,source=travel-api-data,target=/app/data \
  travel-golang-api
```

指定主機連接埠與日期：

```sh
docker run --rm -p 8081:8080 \
  -e API_WRITE_KEY -e REFERENCE_DATE=2026-10-20 \
  --mount type=volume,source=travel-api-data,target=/app/data \
  travel-golang-api
```

掛載主機上的資料庫目錄：

```sh
docker run --rm -p 8080:8080 \
  --user "$(id -u):$(id -g)" -e API_WRITE_KEY \
  --mount type=bind,source="$(pwd)/data",target=/app/data \
  travel-golang-api
```

## Request / response examples

共 42 個 API：35 GET、4 POST、3 PUT。每一列對應一組 method 與 endpoint，範例包含完整 JSON 回應。

範例日期為 `REFERENCE_DATE=2026-10-20`。`{{API_WRITE_KEY}}` 請替換為服務金鑰；`{{wishlist_id}}`、`{{trip_id}}`、`{{review_id}}` 請使用對應 POST 回傳的 ID。回應中的時間為固定示例值，PUT 範例使用建立後的 `version=1`。

| Method | Endpoint | 說明 | Request 範例 | Response 範例 |
| --- | --- | --- | --- | --- |
| `GET` | `/api/v1/articles` | 旅遊內容列表 | [Request](docs/examples/get-api-v1-articles.request.http) | [Response](docs/examples/get-api-v1-articles.response.http) |
| `GET` | `/api/v1/articles/{id}` | 旅遊文章詳情 | [Request](docs/examples/get-api-v1-articles-id.request.http) | [Response](docs/examples/get-api-v1-articles-id.response.http) |
| `GET` | `/api/v1/campaigns` | 優惠活動列表 | [Request](docs/examples/get-api-v1-campaigns.request.http) | [Response](docs/examples/get-api-v1-campaigns.response.http) |
| `GET` | `/api/v1/campaigns/{id}` | 優惠條件與折扣碼 | [Request](docs/examples/get-api-v1-campaigns-id.request.http) | [Response](docs/examples/get-api-v1-campaigns-id.response.http) |
| `GET` | `/api/v1/categories` | 旅遊品類捷徑 | [Request](docs/examples/get-api-v1-categories.request.http) | [Response](docs/examples/get-api-v1-categories.response.http) |
| `GET` | `/api/v1/collections` | 主題集合列表 | [Request](docs/examples/get-api-v1-collections.request.http) | [Response](docs/examples/get-api-v1-collections.response.http) |
| `GET` | `/api/v1/collections/{id}` | 主題集合內容 | [Request](docs/examples/get-api-v1-collections-id.request.http) | [Response](docs/examples/get-api-v1-collections-id.response.http) |
| `GET` | `/api/v1/collections/{id}/products` | 集合內的商品搜尋 | [Request](docs/examples/get-api-v1-collections-id-products.request.http) | [Response](docs/examples/get-api-v1-collections-id-products.response.http) |
| `GET` | `/api/v1/config` | 前端設定與選項 | [Request](docs/examples/get-api-v1-config.request.http) | [Response](docs/examples/get-api-v1-config.response.http) |
| `GET` | `/api/v1/cruise/regions` | 郵輪港口、航區與品牌 | [Request](docs/examples/get-api-v1-cruise-regions.request.http) | [Response](docs/examples/get-api-v1-cruise-regions.response.http) |
| `GET` | `/api/v1/destinations` | 目的地列表 | [Request](docs/examples/get-api-v1-destinations.request.http) | [Response](docs/examples/get-api-v1-destinations.response.http) |
| `GET` | `/api/v1/destinations/{id}` | 目的地探索頁 | [Request](docs/examples/get-api-v1-destinations-id.request.http) | [Response](docs/examples/get-api-v1-destinations-id.response.http) |
| `GET` | `/api/v1/faqs` | 常見問題搜尋 | [Request](docs/examples/get-api-v1-faqs.request.http) | [Response](docs/examples/get-api-v1-faqs.response.http) |
| `GET` | `/api/v1/home` | 首頁區塊聚合 | [Request](docs/examples/get-api-v1-home.request.http) | [Response](docs/examples/get-api-v1-home.response.http) |
| `GET` | `/api/v1/navigation` | 主導覽與頁尾 | [Request](docs/examples/get-api-v1-navigation.request.http) | [Response](docs/examples/get-api-v1-navigation.response.http) |
| `GET` | `/api/v1/products` | 商品搜尋結果 | [Request](docs/examples/get-api-v1-products.request.http) · [400](docs/examples/get-api-v1-products.invalid.request.http) | [Response](docs/examples/get-api-v1-products.response.http) · [400](docs/examples/get-api-v1-products.invalid.response.http) |
| `GET` | `/api/v1/products/map` | 商品地圖點位 | [Request](docs/examples/get-api-v1-products-map.request.http) | [Response](docs/examples/get-api-v1-products-map.response.http) |
| `GET` | `/api/v1/products/{id}` | 商品完整詳情 | [Request](docs/examples/get-api-v1-products-id.request.http) | [Response](docs/examples/get-api-v1-products-id.response.http) |
| `GET` | `/api/v1/products/{id}/availability` | 方案容量、庫存與報價 | [Request](docs/examples/get-api-v1-products-id-availability.request.http) | [Response](docs/examples/get-api-v1-products-id-availability.response.http) |
| `GET` | `/api/v1/products/{id}/cabins` | 郵輪艙房 | [Request](docs/examples/get-api-v1-products-id-cabins.request.http) | [Response](docs/examples/get-api-v1-products-id-cabins.response.http) |
| `GET` | `/api/v1/products/{id}/calendar` | 每日起價與庫存日曆 | [Request](docs/examples/get-api-v1-products-id-calendar.request.http) | [Response](docs/examples/get-api-v1-products-id-calendar.response.http) |
| `GET` | `/api/v1/products/{id}/departures` | 郵輪與跟團出發日期 | [Request](docs/examples/get-api-v1-products-id-departures.request.http) | [Response](docs/examples/get-api-v1-products-id-departures.response.http) |
| `GET` | `/api/v1/products/{id}/itinerary` | 郵輪與跟團每日行程 | [Request](docs/examples/get-api-v1-products-id-itinerary.request.http) | [Response](docs/examples/get-api-v1-products-id-itinerary.response.http) |
| `GET` | `/api/v1/products/{id}/plans` | 跟團、體驗與交通方案 | [Request](docs/examples/get-api-v1-products-id-plans.request.http) | [Response](docs/examples/get-api-v1-products-id-plans.response.http) |
| `POST` | `/api/v1/products/{id}/quotes` | 以 JSON 查詢方案容量與報價 | [Request](docs/examples/post-api-v1-products-id-quotes.request.http) | [Response](docs/examples/post-api-v1-products-id-quotes.response.http) |
| `GET` | `/api/v1/products/{id}/related` | 相關商品推薦 | [Request](docs/examples/get-api-v1-products-id-related.request.http) | [Response](docs/examples/get-api-v1-products-id-related.response.http) |
| `GET` | `/api/v1/products/{id}/reviews` | 商品旅客評價 | [Request](docs/examples/get-api-v1-products-id-reviews.request.http) | [Response](docs/examples/get-api-v1-products-id-reviews.response.http) |
| `POST` | `/api/v1/products/{id}/reviews` | 建立商品評價 | [Request](docs/examples/post-api-v1-products-id-reviews.request.http) | [Response](docs/examples/post-api-v1-products-id-reviews.response.http) |
| `GET` | `/api/v1/products/{id}/reviews/{review_id}` | 取得可編輯評價與版本 | [Request](docs/examples/get-api-v1-products-id-reviews-review_id.request.http) | [Response](docs/examples/get-api-v1-products-id-reviews-review_id.response.http) |
| `PUT` | `/api/v1/products/{id}/reviews/{review_id}` | 完整取代 API 建立的評價 | [Request](docs/examples/put-api-v1-products-id-reviews-review_id.request.http) | [Response](docs/examples/put-api-v1-products-id-reviews-review_id.response.http) |
| `GET` | `/api/v1/products/{id}/rooms` | 住宿房型 | [Request](docs/examples/get-api-v1-products-id-rooms.request.http) | [Response](docs/examples/get-api-v1-products-id-rooms.response.http) |
| `GET` | `/api/v1/search/filters` | 目前查詢結果的篩選計數 | [Request](docs/examples/get-api-v1-search-filters.request.http) | [Response](docs/examples/get-api-v1-search-filters.response.http) |
| `GET` | `/api/v1/search/suggestions` | 目的地與商品自動完成 | [Request](docs/examples/get-api-v1-search-suggestions.request.http) | [Response](docs/examples/get-api-v1-search-suggestions.response.http) |
| `GET` | `/api/v1/trips` | 查詢旅遊行程 | [Request](docs/examples/get-api-v1-trips.request.http) | [Response](docs/examples/get-api-v1-trips.response.http) |
| `POST` | `/api/v1/trips` | 建立旅遊行程 | [Request](docs/examples/post-api-v1-trips.request.http) · [400](docs/examples/post-api-v1-trips.invalid.request.http) | [Response](docs/examples/post-api-v1-trips.response.http) · [400](docs/examples/post-api-v1-trips.invalid.response.http) |
| `GET` | `/api/v1/trips/{id}` | 取得旅遊行程 | [Request](docs/examples/get-api-v1-trips-id.request.http) | [Response](docs/examples/get-api-v1-trips-id.response.http) |
| `PUT` | `/api/v1/trips/{id}` | 完整取代旅遊行程 | [Request](docs/examples/put-api-v1-trips-id.request.http) | [Response](docs/examples/put-api-v1-trips-id.response.http) |
| `GET` | `/api/v1/wishlists` | 查詢收藏清單 | [Request](docs/examples/get-api-v1-wishlists.request.http) | [Response](docs/examples/get-api-v1-wishlists.response.http) |
| `POST` | `/api/v1/wishlists` | 建立收藏清單 | [Request](docs/examples/post-api-v1-wishlists.request.http) | [Response](docs/examples/post-api-v1-wishlists.response.http) |
| `GET` | `/api/v1/wishlists/{id}` | 取得收藏清單 | [Request](docs/examples/get-api-v1-wishlists-id.request.http) | [Response](docs/examples/get-api-v1-wishlists-id.response.http) |
| `PUT` | `/api/v1/wishlists/{id}` | 完整取代收藏清單 | [Request](docs/examples/put-api-v1-wishlists-id.request.http) | [Response](docs/examples/put-api-v1-wishlists-id.response.http) |
| `GET` | `/healthz` | 健康檢查 | [Request](docs/examples/get-healthz.request.http) | [Response](docs/examples/get-healthz.response.http) |
