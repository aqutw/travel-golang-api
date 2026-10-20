#!/usr/bin/env python3
"""Generate original, deterministic travel fixtures and SVG placeholders. No network."""
from build_database import build_database
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
COLORS = [('#e8f2ed', '#245d50', '#8fc4a9'), ('#e8edf8', '#354c84', '#a7bce9'),
          ('#fcf0df', '#ad6539', '#edc18b'), ('#f2eafa', '#695186', '#c1a6dd'),
          ('#e5f4f3', '#236b79', '#86c8ce'), ('#f9e8e7', '#994f59', '#e6a7b0'),
          ('#edf0e1', '#667444', '#bac38c'), ('#f0ebdf', '#75664d', '#cabb97')]
TYPES = [('stay', '住宿', 2400, 'night'), ('cruise', '郵輪', 12900, 'person'),
         ('tour', '跟團旅遊', 24800, 'person'), ('activity', '景點體驗', 950, 'person'),
         ('transfer', '包車接送', 1800, 'vehicle'), ('esim', 'eSIM', 290, 'package'),
         ('sport', '運動旅遊', 8900, 'person'), ('rail', '鐵道假期', 1600, 'person')]
CITIES = [('taipei', '台北', 'TW', '台灣', 25.033, 121.565),
          ('yilan', '宜蘭', 'TW', '台灣', 24.757, 121.754),
          ('hualien', '花蓮', 'TW', '台灣', 23.987, 121.601),
          ('tainan', '台南', 'TW', '台灣', 22.999, 120.227),
          ('tokyo', '東京', 'JP', '日本', 35.676, 139.650),
          ('kyoto', '京都', 'JP', '日本', 35.011, 135.768),
          ('osaka', '大阪', 'JP', '日本', 34.694, 135.502),
          ('okinawa', '沖繩', 'JP', '日本', 26.212, 127.680),
          ('hokkaido', '北海道', 'JP', '日本', 43.062, 141.354),
          ('seoul', '首爾', 'KR', '韓國', 37.566, 126.978),
          ('bangkok', '曼谷', 'TH', '泰國', 13.756, 100.502),
          ('singapore', '新加坡', 'SG', '新加坡', 1.352, 103.820)]


def image(kind, alt):
    return dict(url=f'/images/{kind}.svg', alt=alt, width=1200, height=800)


def price(amount, unit):
    return dict(amount=amount, original_amount=amount + amount // 5,
                currency='TWD', unit=unit, tax_included=True)


def generate():
    media = {}
    data = dict(destinations=[], categories=[], products=[], collections=[], campaigns=[], articles=[], faqs=[], reviews=[])
    for i, (kind, name, _, _) in enumerate(TYPES):
        bg, ink, light = COLORS[i]
        # Geometric vector art is deliberately a placeholder, never a copied travel photo.
        scene = {
            'stay': '<path d="M330 580V280h500v300Z"/><path d="M290 290 580 130 870 290Z"/>',
            'cruise': '<path d="m220 490 120 150h550l120-150Z"/><path d="M380 470V300h420v170ZM470 300v-90h210v90Z"/>',
            'tour': '<path d="m140 620 340-430 210 300 130-200 260 330Z"/>',
            'activity': '<path d="M320 650V300l280-150 280 150v350H760V400H440v250Z"/>',
            'transfer': '<path d="M220 570V390q0-90 90-90h560q90 0 90 90v180Z"/><circle cx="370" cy="590" r="65"/><circle cx="810" cy="590" r="65"/>',
            'esim': '<rect x="420" y="160" width="360" height="500" rx="48"/><path d="M530 270h140v240H530Z" fill="none" stroke="white" stroke-width="15"/>',
            'sport': '<path d="m140 650 400-500 240 350 120-170 200 320Z"/><path d="m430 290 110-140 100 145-115-40Z" fill="white"/>',
            'rail': '<rect x="350" y="180" width="500" height="440" rx="65"/><path d="M410 400h380V250H410Z" fill="white"/><path d="m420 720 90-110m270 110-90-110" stroke-width="25" stroke="currentColor"/>'
        }[kind]
        svg = f'''<svg xmlns="http://www.w3.org/2000/svg" width="1200" height="800" viewBox="0 0 1200 800" role="img" aria-labelledby="title desc">
<title id="title">{name}示意插畫</title><desc id="desc">原創幾何圖形假圖片，非實景照片</desc>
<rect width="1200" height="800" fill="{bg}"/><circle cx="920" cy="160" r="95" fill="{light}"/>
<path d="M0 650Q300 540 600 660T1200 620V800H0Z" fill="{light}"/>
<g fill="{ink}" color="{ink}">{scene}</g>
<text x="64" y="90" font-family="sans-serif" font-size="28" fill="{ink}">TRAVEL / {kind.upper()}</text>
<text x="64" y="755" font-family="sans-serif" font-size="20" fill="{ink}">ORIGINAL MOCK ILLUSTRATION</text></svg>\n'''
        media[f'/images/{kind}.svg'] = svg
        data['categories'].append(dict(id=kind, name=name, description=f'探索{name}商品與方案', image=image(kind, f'{name}示意圖')))

    for i, (slug, name, code, country, lat, lng) in enumerate(CITIES):
        data['destinations'].append(dict(id=slug, name=name, country_code=code, country_name=country,
            description=f'從{name}的街區散步到在地餐桌，安排一段留有餘裕的旅行。',
            latitude=lat, longitude=lng, image=image(TYPES[i % 8][0], f'{name}旅遊示意圖')))

    for ti, (kind, label, base, unit) in enumerate(TYPES):
        for i in range(12):
            # Ports and rail destinations stay geographically plausible.
            di = ([7, 11, 4][i % 3] if kind == 'cruise' else
                  [0, 1, 3][i % 3] if kind == 'rail' else i)
            dest = data['destinations'][di]
            pid = f'{kind}-{i+1:03}'
            amount = base + (i % 6) * (150 if base < 3000 else 1000)
            duration = 1 if kind in ('stay', 'activity', 'transfer', 'rail') else 5 if kind in ('tour', 'cruise', 'esim') else 3
            suffixes = {
                'stay': ['木光旅宿', '溪畔溫泉小屋', '山嵐豪華露營', '巷弄包棟寓所'],
                'cruise': ['海風號跳島五日', '晨星號海上假期五日', '遠洋號慢遊五日', '藍灣號親子航程五日'],
                'tour': ['溫泉與山城五日', '街區美食五日', '森林步道五日', '親子自然探索五日'],
                'activity': ['城市散步與手作體驗', '在地料理工作坊', '森林導覽一日遊', '河岸單車體驗'],
                'transfer': ['機場接送專車', '港口接送專車', '市區包車六小時', '近郊包車八小時'],
                'esim': ['五日上網卡', '五日輕量上網卡', '五日旅遊數據卡', '五日高速上網卡'],
                'sport': ['城市路跑三日套裝', '單車慢遊三日', '雪地初學三日營', '山徑健行三日'],
                'rail': ['鐵道與散步一日', '鐵道美食一日', '車站周邊一日', '鐵道文化一日']
            }
            title = f'{dest["name"]}｜{suffixes[kind][i % 4]}'
            amenities = ['wifi', 'breakfast'] if kind == 'stay' else ['chinese_service', 'guide']
            tags = ['親子友善' if i % 2 == 0 else '雙人出遊', '熱門推薦' if i % 3 == 0 else '慢遊精選']
            if kind == 'stay':
                tags.append(['飯店', '溫泉', '露營', '包棟'][i % 4])
                if i % 3 == 0: amenities.append('parking')
                if i % 4 == 1: amenities.append('hot_spring')
            product = dict(id=pid, type=kind, title=title,
                summary=f'以{dest["name"]}為起點，享受從容的{label}時光。所有內容皆為測試假資料。',
                destination_id=dest['id'], location_name=f'{dest["country_name"]}・{dest["name"]}',
                latitude=round(dest['latitude'] + (i % 4) * .003, 6),
                longitude=round(dest['longitude'] + (i % 3) * .004, 6),
                cover=image(kind, f'{title}示意圖'), price=price(amount, unit),
                rating={}, tags=tags, amenities=amenities, duration_days=duration,
                popularity=1000 - i * 23 - ti * 7, free_cancellation=i % 2 == 0,
                confirmation='request' if kind in ('cruise', 'tour') else 'instant',
                detail_url=f'/api/v1/products/{pid}', images=[image(kind, f'{title}示意圖'), image('tour', '周邊環境示意圖'), image('stay', '休憩空間示意圖')],
                description=f'這是原創的{label}示範商品。保留探索{dest["name"]}的時間，讓行程留白，讓風景成為旅程的主角。',
                address=f'{dest["name"]}示範街 {i+1} 號（虛構地址）', highlights=['在地風格體驗', '繁體中文資訊', '清楚的方案內容'],
                included=['方案所列服務', '示範稅金'], excluded=['個人購物', '未列明的加購服務'],
                cancellation_policy='出發前七日可免費取消。' if i % 2 == 0 else '本示範方案不提供免費取消。',
                notices=['所有商品、評價、優惠與庫存皆為虛構。', '插畫為示意用途，非實景照片。'])
            names = ['經典雙人房', '景觀四人房'] if kind == 'stay' else ['內艙雙人房', '陽台四人房'] if kind == 'cruise' else ['標準方案', '升級方案']
            options = [dict(id=f'{pid}-opt-{j+1}', name=n, capacity=(2 if j == 0 else 4) if kind in ('stay', 'cruise') else 8,
                description=f'{n}，提供測試用價格與容量。', amenities=amenities, image=image(kind, f'{n}示意圖'),
                price=price(amount + j * (600 if base < 3000 else 2500), unit)) for j, n in enumerate(names)]
            itinerary = [dict(day=day, title=f'{dest["name"]}・'+['啟程與相遇', '街區探索', '自然漫遊', '在地生活', '悠閒返程'][(day-1) % 5],
                description=f'第 {day} 天探索在地景色，保留彈性自由活動時間。', meals=['早餐', '晚餐'],
                accommodation='船上住宿' if kind == 'cruise' else '示範風格旅館') for day in range(1, duration+1)]
            if kind == 'stay':
                product['stay'] = dict(property_type=['hotel', 'hot_spring', 'camping', 'entire_house'][i % 4], check_in_time='15:00', check_out_time='11:00', rooms=options)
            elif kind == 'cruise':
                port = ['基隆港', '新加坡港', '橫濱港'][i % 3]
                product['cruise'] = dict(ship_name=['海風號', '晨星號', '遠洋號', '藍灣號'][i % 4], cruise_line='藍灣假期（虛構品牌）', departure_port=port,
                    route=f'{port}－{dest["name"]}－海上巡遊－{port}', cabins=options, itinerary=itinerary)
            elif kind == 'tour':
                product['tour'] = dict(departure_city='台北', transport='示範直飛航班與遊覽車', minimum_travelers=8, plans=options, itinerary=itinerary)
            else:
                product['activity'] = dict(meeting_point=f'{dest["name"]}示範旅客服務中心' if kind != 'esim' else '線上啟用', voucher_type='electronic',
                    usage_instructions=['選擇日期與方案。', '出示電子憑證。' if kind != 'esim' else '依示範步驟安裝並啟用 eSIM。'], plans=options)
            reviews = []
            for j in range(5):
                score = [5, 5, 4, 4, 5][(i+j) % 5]
                reviews.append(dict(id=f'{pid}-review-{j+1}', product_id=pid, author=f'示範旅人 {j+1}', rating=score,
                    content=['說明清楚，行程安排有餘裕。', '環境舒適，適合和家人一起安排。', '交通方便，體驗符合期待。', '服務親切，出發前資訊很完整。', '喜歡這次慢下來的旅行節奏。'][j],
                    travel_date=f'2026-09-{10+j:02}', created_at=f'2026-09-{15+j:02}'))
            product['rating'] = dict(average=round(sum(r['rating'] for r in reviews)/5, 1), count=5)
            data['reviews'].extend(reviews)
            data['products'].append(product)

    for i, (kind, name, _, _) in enumerate(TYPES):
        ids = [p['id'] for p in data['products'] if p['type'] == kind]
        data['collections'].append(dict(id=f'{kind}-picks', title=f'{name}靈感精選', description=f'從熱門目的地出發，尋找適合自己的{name}方案。', image=image(kind, f'{name}主題示意圖'), product_ids=ids))
    for slug, name, tag in [('family', '親子同行', '親子友善'), ('camping', '露營與包棟', '露營')]:
        data['collections'].append(dict(id=slug, title=name, description=f'{name}主題商品集合。', image=image('stay', name), product_ids=[p['id'] for p in data['products'] if tag in p['tags'] or slug == 'camping' and '包棟' in p['tags']]))
    for i, kind in enumerate(['stay', 'cruise', 'tour']):
        data['campaigns'].append(dict(id=f'escape-{kind}', title=['住進週末的留白', '把假期交給海風', '下一站，一起出發'][i],
            description='原創虛構優惠，僅供前端畫面展示。', image=image(kind, '優惠活動示意圖'), coupon_code=f'DEMO{i+1}00',
            discount_amount=(i+1)*100, minimum_spend=3000, collection_id=f'{kind}-picks', terms=['僅供開發測試。', '不適用任何真實訂單。']))
    for i, d in enumerate(data['destinations'][:8]):
        data['articles'].append(dict(id=f'guide-{d["id"]}', title=f'{d["name"]}慢旅行：留一點時間給街角', summary='三個方向，安排一段從容的假期。', destination_id=d['id'], image=d['image'],
            content=[f'從{d["name"]}的步行街區開始，選擇一間喜歡的小店。', '中午留給在地餐桌，下午安排一段自然散步。', '這篇內容為原創假資料，交通與營業時間請勿作為真實出遊依據。'],
            related_product_ids=[p['id'] for p in data['products'] if p['destination_id'] == d['id']][:4]))
    for i, (cat, q, a) in enumerate([
        ('general', '這些商品可以實際預訂嗎？', '不行。此服務只提供開發用 GET API 與虛構資料。'),
        ('stay', '如何選擇房型？', '依入住人數與房間數查詢日期庫存，再比較房型容量與價格。'),
        ('cruise', '艙房價格包含哪些內容？', '請以商品 included 與 excluded 欄位展示示範費用內容。'),
        ('tour', '如何查看成團狀態？', '出發日期資料提供 forming、guaranteed 與 sold_out 狀態。'),
        ('activity', '憑證如何使用？', '詳細頁提供電子憑證類型與使用步驟。'),
        ('general', '庫存是否會被預訂扣除？', '不會。相同商品、日期與方案會回傳相同的模擬庫存。')]):
        data['faqs'].append(dict(id=f'faq-{i+1:03}', category=cat, question=q, answer=a))
    data['settings'] = {'navigation': {'primary': [{'label': '郵輪', 'href': '/products?type=cruise'},
                            {'label': '跟團旅遊', 'href': '/products?type=tour'},
                            {'label': '住宿', 'href': '/products?type=stay'},
                            {'label': '景點體驗', 'href': '/products?type=activity'},
                            {'label': '鐵道假期', 'href': '/products?type=rail'},
                            {'label': '運動旅遊', 'href': '/products?type=sport'}],
                'footer': [{'title': '探索旅行',
                            'links': [{'label': '目的地', 'href': '/destinations'},
                                      {'label': '主題旅遊', 'href': '/collections'},
                                      {'label': '旅行靈感', 'href': '/articles'}]},
                           {'title': '支援與協助',
                            'links': [{'label': '常見問題', 'href': '/faqs'},
                                      {'label': 'API 文件', 'href': '/docs'}]}]},
 'config': {'api_version': 'v1',
            'currencies': ['TWD'],
            'limits': {'availability_nights': 30,
                       'booking_horizon_days': 365,
                       'calendar_days': 90,
                       'per_page': 100},
            'locales': ['zh-TW'],
            'name': '留白旅行 Mock API',
            'notice': '獨立開發示範。所有商品、圖片、價格、評價與庫存皆為虛構。',
            'price_units': {'night': '每房每晚', 'package': '每份', 'person': '每人每次行程', 'vehicle': '每車'},
            'sort_options': [{'label': '人氣推薦', 'href': 'popular'},
                             {'label': '價格由低至高', 'href': 'price_asc'},
                             {'label': '價格由高至低', 'href': 'price_desc'},
                             {'label': '旅客評分', 'href': 'rating'},
                             {'label': '行程天數', 'href': 'duration'}],
            'timezone': 'Asia/Taipei'},
 'home': {'hero': {'title': '把日子，留給風景。',
                   'subtitle': '從一晚好眠到一段遠行，找到你的下一個旅行靈感。',
                   'image': {'url': '/images/tour.svg', 'alt': '跟團旅遊示意圖', 'width': 1200, 'height': 800}},
          'section_collection_ids': ['stay-picks',
                                     'cruise-picks',
                                     'tour-picks',
                                     'activity-picks',
                                     'transfer-picks',
                                     'esim-picks',
                                     'sport-picks',
                                     'rail-picks'],
          'destination_ids': ['taipei', 'yilan', 'hualien', 'tainan', 'tokyo', 'kyoto'],
          'article_ids': ['guide-taipei', 'guide-yilan', 'guide-hualien'],
          'cards_per_section': 4}}
    build_database(data, media)



if __name__ == '__main__':
    generate()
