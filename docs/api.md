# API 參考

Base URL：`http://localhost:8080`。所有業務資料使用 `/api/v1`。公開商品與內容查詢免金鑰。收藏、行程與評價編輯使用 `X-API-Key`，其值由服務端 `API_WRITE_KEY` 設定。`HEAD` 與 `OPTIONS` 支援讀取及預檢。

完整欄位型別由 [OpenAPI](../internal/assets/openapi.json) 定義，`cmd/specgen` 從 Go 回應型別產生 schema。`/docs` 提供 Swagger UI，`/` 提供 GET 範例請求，`/openapi.json` 提供契約下載。

## 回應包裝

```json
{
	"data": [],
	"meta": {
		"mock": true,
		"locale": "zh-TW",
		"currency": "TWD",
		"reference_date": "2026-10-20",
		"pagination": {
			"page": 1,
			"per_page": 12,
			"total": 0,
			"total_pages": 0,
			"has_next": false
		}
	}
}
```

`pagination` 僅出現在有分頁的清單。`products` 與 `collections/{id}/products` 另外提供 `meta.facets`，計數基於分頁前的完整篩選結果。沒有結果或頁數超出範圍時，`data` 為空陣列。

```json
{
	"error": {
		"code": "invalid_query",
		"message": "page: must be an integer between 1 and 1000000"
	}
}
```

| HTTP | code | 語意 |
| --- | --- | --- |
| 400 | `invalid_query` | 未知或重複參數、格式錯誤、超出範圍、不支援的語系或幣別 |
| 404 | `not_found` | 路由或指定資源不存在 |
| 404 | `not_applicable` | 例如向住宿查詢艙房，或向體驗查詢跟團出發日 |
| 405 | `method_not_allowed` | 該路由不支援此 HTTP 方法 |
| 400 | `invalid_body` | JSON 欄位、巢狀項目或日期關係驗證失敗；`error.fields` 提供 field 與 rule |
| 401 | `unauthorized` | 缺少或錯誤的 X-API-Key |
| 403 | `origin_not_allowed` | origin 不在 CORS_ORIGINS 內 |
| 409 | `version_conflict`、`idempotency_conflict` | 過期版本或重複 key 搭配不同內容 |
| 413 | `body_too_large` | body 超過 64 KiB |
| 415 | `unsupported_media_type` | Content-Type 不是 application/json |
| 429 | `rate_limited` | 超過寫入頻率，Retry-After 指示等待秒數 |
| 503 | `writes_disabled` | 未設定 API_WRITE_KEY |
| 500 | `database_error` | SQLite 查詢或資料解碼失敗 |

所有業務端點可接受 `locale=zh-TW`、`currency=TWD`；其他值回傳 400。列表上的文字篩選未命中時回傳 200 空陣列。商品搜尋中的未知 `type`、`country`、`destination_id` 回傳 400。

## 路由

下表包含 29 個業務 GET 端點與健康檢查。

| GET 路徑 | 資料 | 分頁 |
| --- | --- | --- |
| `/healthz` | 服務健康狀態 | — |
| `/api/v1/config` | 語系、幣別、品類、排序與限制 | — |
| `/api/v1/navigation` | 主導覽與頁尾分組 | — |
| `/api/v1/home` | Hero、優惠、目的地、集合與商品區塊 | — |
| `/api/v1/categories` | 八種旅遊品類 | — |
| `/api/v1/cruise/regions` | 郵輪港口、目的地航區與船公司 | — |
| `/api/v1/destinations` | 目的地；支援 `q`、`country` | ✓ |
| `/api/v1/destinations/{id}` | 目的地介紹、精選商品與相關集合 | — |
| `/api/v1/search/suggestions` | 目的地及商品自動完成；支援 `q`、`type`、`limit` | — |
| `/api/v1/search/filters` | 商品篩選後的 facets | — |
| `/api/v1/products` | 商品卡片搜尋與 facets | ✓ |
| `/api/v1/products/map` | 商品地圖點位、名稱與價格 | ✓ |
| `/api/v1/products/{id}` | 共用商品資訊及品類詳情 | — |
| `/api/v1/products/{id}/rooms` | 住宿房型 | — |
| `/api/v1/products/{id}/cabins` | 郵輪艙房 | — |
| `/api/v1/products/{id}/plans` | 跟團、體驗、交通、上網、運動與鐵道方案 | — |
| `/api/v1/products/{id}/itinerary` | 郵輪與跟團的逐日行程 | — |
| `/api/v1/products/{id}/calendar` | 每日庫存、起價及狀態 | — |
| `/api/v1/products/{id}/departures` | 郵輪與跟團日期；支援 `status` | ✓ |
| `/api/v1/products/{id}/availability` | 依人數、份數與日期試算各方案 | — |
| `/api/v1/products/{id}/reviews` | 商品評價；支援 `rating=1..5` | ✓ |
| `/api/v1/products/{id}/related` | 同目的地或同品類推薦 | ✓ |
| `/api/v1/collections` | 主題集合列表 | ✓ |
| `/api/v1/collections/{id}` | 主題內容與商品 ID | — |
| `/api/v1/collections/{id}/products` | 集合內商品，支援商品搜尋參數 | ✓ |
| `/api/v1/campaigns` | 優惠活動列表 | ✓ |
| `/api/v1/campaigns/{id}` | 折扣碼、門檻與條件 | — |
| `/api/v1/articles` | 文章列表；支援 `destination_id` | ✓ |
| `/api/v1/articles/{id}` | 文章段落及相關商品 ID | — |
| `/api/v1/faqs` | 常見問題；支援 `category`、`q` | ✓ |

靜態資源：`/docs`、`/openapi.json`、`/images/{type}.svg`。所有分頁清單支援 `page`（預設 1）及 `per_page`（預設 12，上限 100）。

## 商品搜尋參數

以下參數適用於 `products`、`products/map`、`search/filters` 及 `collections/{id}/products`。`search/filters` 接受分頁參數，但 facets 始終根據全部匹配商品計算。

| 參數 | 語意 |
| --- | --- |
| `q` | 商品名稱、摘要、目的地、國家、標籤的部分比對；英文不分大小寫 |
| `type` | `stay`、`cruise`、`tour`、`activity`、`transfer`、`esim`、`sport`、`rail` |
| `destination_id` | 例如 `taipei`、`tokyo`、`okinawa` |
| `country` | `TW`、`JP`、`KR`、`TH`、`SG` |
| `tag` | 單一標籤精確比對，例如 `親子友善` |
| `amenity` | 單一設施精確比對，例如 `wifi`、`parking` |
| `property_type` | `hotel`、`hot_spring`、`camping`、`entire_house` |
| `departure_port` | 郵輪出發港精確比對，例如 `基隆港` |
| `cruise_line` | 郵輪公司名稱精確比對，由 `cruise/regions` 取得 |
| `departure_city` | 跟團出發地精確比對，目前假資料為 `台北` |
| `min_price`、`max_price` | 卡片起價區間，整數 TWD 元，0..10000000 |
| `min_rating` | 0..5，支援小數 |
| `min_days`、`max_days` | 商品天數範圍，1..30 |
| `free_cancellation` | `true` 僅顯示免費取消；只接受 `true` 或 `false` |
| `sort` | `popular`、`price_asc`、`price_desc`、`rating`、`duration`；預設 `popular` |
| `bbox` | `west,south,east,north`，例如 `121,24,122,26`；不支援跨國際換日線 |
| `start_date`、`end_date` | 須成對提供。住宿是入住／退房；其他商品是出發日期窗，結束日不包含 |
| `adults`、`children`、`quantity` | 須搭配日期。預設 2、0、1。成人 1..16、兒童 0..16、份數 1..8 |

同分排序以商品 ID 升冪，避免分頁重複。非住宿搜尋會檢查日期窗內是否至少一天有符合容量及庫存的方案。搜尋 `quantity > 1` 時不納入按人計價商品；這類商品透過人數表示需求。

搜尋 `type` 參數不改變建議項目的 `type`：自動完成的建議種類仍為 `destination` 或 `product`。該參數僅限制商品建議所屬品類，目的地建議保留。

## 日期、庫存及報價

- 日期均使用台北時區。`start_date` 可介於 `reference_date` 至其後 365 天。
- 結束日不包含。`calendar` 與 `departures` 預設回傳 30 天，最多 90 天；搜尋日期窗與住宿晚數最多 30 天。
- `availability` 預設從 `reference_date + 7 天` 開始。住宿預設 1 晚；其他品類使用商品固定 `duration_days`。
- 非住宿的 `availability.end_date` 若明確提供，必須等於開始日加商品固定天數。非住宿庫存綁定開始日。
- 同一方案及日期的庫存由固定雜湊產生，每次重啟仍相同。庫存可能為零，供前端展示售罄狀態。
- 住宿跨晚查詢採各晚庫存最小值；同一房型需要每晚都有足夠房數。
- `calendar.remaining` 是當日各方案剩餘量的最大值，`price` 是尚有庫存方案中的最低單價。日曆不帶人數，實際可用方案以 `availability` 為準。
- `departures.status` 支援 `available`、`guaranteed`、`forming`、`sold_out`；成團狀態僅由虛構規則生成。
- 兒童與成人同價，沒有年齡級距。按人計價的剩餘量代表席位，其餘品類代表房、車或方案份數。

| 計價單位 | `multiplier` | `quantity` |
| --- | --- | --- |
| `night` | 晚數 × 房數 | 房數 |
| `person` | 成人＋兒童 | 必須為 1 |
| `vehicle` | 車輛數 | 車數 |
| `package` | 方案份數 | 份數 |

`total_amount = unit_price.amount × multiplier`。每個方案回傳容量、剩餘量、可用狀態及試算金額。`available=false` 的方案仍提供公式試算，前端應依狀態停用選擇。價格均假設含稅，不模擬匯率、優惠核銷、動態加價或金流。

## 商品詳情變體

商品卡片共用欄位，完整詳情依 `type` 包含一個對應結構：

| type | 詳情欄位 | 內容 |
| --- | --- | --- |
| `stay` | `stay` | 住宿類型、入住退房時間、房型 |
| `cruise` | `cruise` | 船名、品牌、出發港、航線、艙房、逐日行程 |
| `tour` | `tour` | 出發城市、交通、最低成團人數、方案、逐日行程 |
| 其餘品類 | `activity` | 集合地點、電子憑證、使用步驟、方案 |

價格卡片為起價，不代表指定日期、全部旅客的總價。商品評分由五則假評價計算，與 `reviews` 端點一致。

## 收藏、行程、評價及報價

| Method | 路徑 | 行為 | 金鑰 |
| --- | --- | --- | --- |
| GET | `/api/v1/wishlists` | 分頁查詢收藏清單 | 需要 |
| GET | `/api/v1/wishlists/{id}` | 取得收藏與版本 | 需要 |
| POST | `/api/v1/wishlists` | 建立收藏清單 | 需要 |
| PUT | `/api/v1/wishlists/{id}` | 完整取代收藏 | 需要 |
| GET | `/api/v1/trips` | 分頁查詢行程 | 需要 |
| GET | `/api/v1/trips/{id}` | 取得行程與版本 | 需要 |
| POST | `/api/v1/trips` | 建立旅遊行程 | 需要 |
| PUT | `/api/v1/trips/{id}` | 完整取代行程 | 需要 |
| POST | `/api/v1/products/{id}/reviews` | 建立評價，更新商品平均評分 | 需要 |
| GET | `/api/v1/products/{id}/reviews/{review_id}` | 取得可編輯評價與版本 | 需要 |
| PUT | `/api/v1/products/{id}/reviews/{review_id}` | 完整取代 API 建立的評價 | 需要 |
| POST | `/api/v1/products/{id}/quotes` | 結構化日期、人數及數量報價 | 公開 |

收藏 body 包含 `title`（非空白、最多 100 字）、`description`（最多 1000 字）、`product_ids`（必填陣列，最多 100 筆，商品不可重複）。

行程 body 包含 `title`、`start_date`、`end_date`、`adults`（1–16）、`children`（0–16，預設 0）、`items`（必填陣列，最多 100 筆）、`notes`（最多 2000 字）。每個 item 包含 `product_id`、`day`（1–30）、`notes`（最多 1000 字）。起訖日採結束日不包含，範圍 1–30 天，開始日位於 reference_date 至 +365 天。item 的 day 必須落在行程範圍，同一天的商品不可重複。

評價 body 包含 `author`（非空白、最多 80 字）、`rating`（1–5）、`content`（非空白、最多 2000 字）、`travel_date`（YYYY-MM-DD，不晚於 reference_date）。種子評價可透過公開清單讀取；新增的評價提供帶有 `version` 的個別編輯端點。

報價 body 必填 `start_date`、`end_date`、`adults`、`quantity`（1–8）；`children` 預設 0。按人計價時 quantity 為 1。住宿以外的商品必須符合固定行程天數。結果與 GET availability 相同，庫存仍依方案 ID 與日期推導。

所有 POST／PUT 使用 `application/json`。Gin binding tags 驗證型別、範圍與巢狀陣列，拒絕未知欄位、多個 JSON 文件及超過 64 KiB 的 body。時間關係、商品存在性與版本規則由邊界與交易層處理。

建立收藏、行程及評價時，必須提供 8–128 字元的 `Idempotency-Key`（英數字、點、底線或連字號）。相同方法、路徑、key 與正規化 body 重播原本的 201、data 及 Location，並附上 `Idempotency-Replayed: true`；相同 key 搭配不同內容回傳 409。記錄保存在 SQLite，服務重新啟動後仍有效。

PUT 使用完整 body 與目前 `version`。更新成功後版本加一，過期版本修改不同內容回傳 409。與目前內容完全相同的重試回傳目前資料，不改版本及 updated_at。未找到 ID 回傳 404。

`X-API-Key` 代表單一 workspace 的受信任服務端存取權，持有該金鑰的客戶端共用收藏、行程及評價編輯權。服務沒有會員帳號或各會員的存取範圍。

## Middleware

- 每個請求回傳 `X-Request-ID`；輸入限 8–64 個英數字、底線或連字號，其他值會重新產生。
- access log 包含 request ID、method、路由樣式、HTTP status 與耗時，省略 query、body 及金鑰。
- panic recovery 回傳結構化 500，不回傳 panic 內容。
- JSON body 上限 64 KiB，包含無 Content-Length 的串流請求。
- CORS 預設允許所有 origin；`CORS_ORIGINS` 可指定逗號分隔的完整 origin。預檢支援 X-API-Key 與 Idempotency-Key。
- POST／PUT 每個 peer IP 預設每分鐘 60 次、burst 20。超限回傳 429 與 Retry-After。最多保存 1024 個 bucket，閒置超過 10 分鐘可回收。
- 停用 trusted proxies，限流不採用客戶端提供的 X-Forwarded-For。限流計數屬於單一服務程序。
- 回應附上 `Cache-Control: no-store` 與 `X-Content-Type-Options: nosniff`。
