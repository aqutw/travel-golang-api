# Travel Golang API

## 專案目錄與檔案結構

```text
travel-golang-api/
├── cmd/
│   ├── server/main.go                 # HTTP server 入口
│   └── specgen/main.go                # OpenAPI 產生器
├── internal/
│   ├── catalog/
│   │   ├── model.go                   # 資料型別
│   │   ├── catalog.go                 # 搜尋、庫存與價格計算
│   │   ├── database.go                # SQLite 連線與查詢
│   │   └── database_test.go           # 資料庫測試
│   ├── httpapi/
│   │   ├── api.go                     # HTTP 路由與回應
│   │   ├── content.go                 # 頁面內容處理
│   │   ├── query.go                   # 查詢參數解析
│   │   ├── api_test.go                # API 整合測試
│   │   └── sqlite_test.go             # SQLite 與 API 連動測試
│   └── assets/
│       ├── assets.go                  # 靜態資源嵌入
│       ├── index.html                 # API Explorer
│       └── openapi.json               # OpenAPI 契約
├── data/travel.sqlite3                # 業務資料、頁面設定與圖片
├── db/schema.sql                     # SQLite 資料表定義
├── scripts/
│   ├── generate_fixtures.py           # 資料與圖片產生器
│   ├── build_database.py              # SQLite 建庫工具
│   ├── database_manifest.py           # 資料庫內容比對
│   ├── verify_api.py                  # API 回應驗證
│   └── check_content.py               # 檔案內容掃描
├── docs/
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

需要 Go 1.22 或以上版本。

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

若 Go 1.22 在 macOS 出現 `missing LC_UUID`，使用系統 linker 啟動：

```sh
go run -ldflags=-linkmode=external ./cmd/server
```

### 使用 Docker

在專案根目錄執行：

```sh
docker build -t travel-golang-api .
docker run --rm -p 8080:8080 travel-golang-api
```

指定主機連接埠與日期：

```sh
docker run --rm -p 8081:8080 \
	-e REFERENCE_DATE=2026-10-20 \
	travel-golang-api
```

掛載主機上的資料庫：

```sh
docker run --rm -p 8080:8080 \
	--mount type=bind,source="$(pwd)/data/travel.sqlite3",target=/app/data/travel.sqlite3,readonly \
	travel-golang-api
```

## Request / response examples

以下回應以 `REFERENCE_DATE=2026-10-20` 為例。

### 健康檢查

Request：

```sh
curl -i 'http://localhost:8080/healthz'
```

Response：`HTTP 200`

```json
{
  "data": {
    "status": "ok"
  },
  "meta": {
    "mock": true,
    "locale": "zh-TW",
    "currency": "TWD",
    "reference_date": "2026-10-20"
  }
}
```

### 搜尋住宿商品

Request：

```sh
curl -i 'http://localhost:8080/api/v1/products?type=stay&destination_id=taipei&per_page=1'
```

Response：`HTTP 200`

```json
{
  "data": [
    {
      "id": "stay-001",
      "type": "stay",
      "title": "台北｜木光旅宿",
      "summary": "以台北為起點，享受從容的住宿時光。所有內容皆為測試假資料。",
      "destination_id": "taipei",
      "location_name": "台灣・台北",
      "latitude": 25.033,
      "longitude": 121.565,
      "cover": {
        "url": "/images/stay.svg",
        "alt": "台北｜木光旅宿示意圖",
        "width": 1200,
        "height": 800
      },
      "price": {
        "amount": 2400,
        "original_amount": 2880,
        "currency": "TWD",
        "unit": "night",
        "tax_included": true
      },
      "rating": {
        "average": 4.6,
        "count": 5
      },
      "tags": [
        "親子友善",
        "熱門推薦",
        "飯店"
      ],
      "amenities": [
        "wifi",
        "breakfast",
        "parking"
      ],
      "duration_days": 1,
      "popularity": 1000,
      "free_cancellation": true,
      "confirmation": "instant",
      "detail_url": "/api/v1/products/stay-001"
    }
  ],
  "meta": {
    "mock": true,
    "locale": "zh-TW",
    "currency": "TWD",
    "reference_date": "2026-10-20",
    "pagination": {
      "page": 1,
      "per_page": 1,
      "total": 1,
      "total_pages": 1,
      "has_next": false
    },
    "facets": {
      "types": [
        {
          "value": "stay",
          "count": 1
        }
      ],
      "destinations": [
        {
          "value": "taipei",
          "count": 1
        }
      ],
      "tags": [
        {
          "value": "熱門推薦",
          "count": 1
        },
        {
          "value": "親子友善",
          "count": 1
        },
        {
          "value": "飯店",
          "count": 1
        }
      ],
      "amenities": [
        {
          "value": "breakfast",
          "count": 1
        },
        {
          "value": "parking",
          "count": 1
        },
        {
          "value": "wifi",
          "count": 1
        }
      ],
      "min_price": 2400,
      "max_price": 2400
    }
  }
}
```

### 查詢郵輪艙房

Request：

```sh
curl -i 'http://localhost:8080/api/v1/products/cruise-001/cabins'
```

Response：`HTTP 200`

```json
{
  "data": [
    {
      "id": "cruise-001-opt-1",
      "name": "內艙雙人房",
      "capacity": 2,
      "description": "內艙雙人房，提供測試用價格與容量。",
      "amenities": [
        "chinese_service",
        "guide"
      ],
      "image": {
        "url": "/images/cruise.svg",
        "alt": "內艙雙人房示意圖",
        "width": 1200,
        "height": 800
      },
      "price": {
        "amount": 12900,
        "original_amount": 15480,
        "currency": "TWD",
        "unit": "person",
        "tax_included": true
      }
    },
    {
      "id": "cruise-001-opt-2",
      "name": "陽台四人房",
      "capacity": 4,
      "description": "陽台四人房，提供測試用價格與容量。",
      "amenities": [
        "chinese_service",
        "guide"
      ],
      "image": {
        "url": "/images/cruise.svg",
        "alt": "陽台四人房示意圖",
        "width": 1200,
        "height": 800
      },
      "price": {
        "amount": 15400,
        "original_amount": 18480,
        "currency": "TWD",
        "unit": "person",
        "tax_included": true
      }
    }
  ],
  "meta": {
    "mock": true,
    "locale": "zh-TW",
    "currency": "TWD",
    "reference_date": "2026-10-20"
  }
}
```

### 查詢住宿庫存與報價

Request：

```sh
curl -i 'http://localhost:8080/api/v1/products/stay-001/availability?start_date=2026-11-01&end_date=2026-11-03&adults=2&quantity=1'
```

Response：`HTTP 200`

```json
{
  "data": {
    "product_id": "stay-001",
    "start_date": "2026-11-01",
    "end_date": "2026-11-03",
    "nights": 2,
    "adults": 2,
    "children": 0,
    "quantity": 1,
    "available": true,
    "currency": "TWD",
    "options": [
      {
        "option_id": "stay-001-opt-1",
        "name": "經典雙人房",
        "capacity": 2,
        "remaining": 0,
        "available": false,
        "unit_price": {
          "amount": 2400,
          "original_amount": 2880,
          "currency": "TWD",
          "unit": "night",
          "tax_included": true
        },
        "multiplier": 2,
        "total_amount": 4800
      },
      {
        "option_id": "stay-001-opt-2",
        "name": "景觀四人房",
        "capacity": 4,
        "remaining": 1,
        "available": true,
        "unit_price": {
          "amount": 3000,
          "original_amount": 3600,
          "currency": "TWD",
          "unit": "night",
          "tax_included": true
        },
        "multiplier": 2,
        "total_amount": 6000
      }
    ],
    "disclaimer": "模擬報價，不保留庫存，也不建立訂單。"
  },
  "meta": {
    "mock": true,
    "locale": "zh-TW",
    "currency": "TWD",
    "reference_date": "2026-10-20"
  }
}
```

### 無效查詢參數

Request：

```sh
curl -i 'http://localhost:8080/api/v1/products?page=0'
```

Response：`HTTP 400`

```json
{
  "error": {
    "code": "invalid_query",
    "message": "page: must be an integer between 1 and 1000000"
  }
}
```
