"""Build the complete single-file catalog from generated records."""
import json
import os
from pathlib import Path
import sqlite3

ROOT = Path(__file__).resolve().parents[1]
JSON_FIELDS = {
    'categories': ['image'], 'destinations': ['image'],
    'products': ['cover', 'tags', 'amenities', 'images', 'highlights', 'included', 'excluded', 'notices'],
    'options': ['amenities', 'image'], 'activity_details': ['usage_instructions'],
    'itinerary_days': ['meals'], 'collections': ['image'], 'campaigns': ['image', 'terms'],
    'articles': ['image', 'content'],
}


def encode(value):
    return json.dumps(value, ensure_ascii=False, separators=(',', ':'))


def insert(db, table, record):
    columns = [row[1] for row in db.execute(f'PRAGMA table_info({table})')]
    values = [encode(record[c]) if c in JSON_FIELDS.get(table, []) else record[c] for c in columns]
    db.execute(f'INSERT INTO {table} ({",".join(columns)}) VALUES ({",".join("?" for _ in columns)})', values)


def object_sql(table, exclude=(), extra=None):
    columns = [row[1] for row in table[0].execute(f'PRAGMA table_info({table[1]})')]
    fields = {c: f'json(t.{c})' if c in JSON_FIELDS.get(table[1], []) else f't.{c}'
              for c in columns if c not in ('position', *exclude)}
    fields.update(extra or {})
    return 'json_object(' + ','.join(f"'{key}',{value}" for key, value in fields.items()) + ')'


def group(view, foreign_key, parent='t.id', order='position, id'):
    return f'(SELECT json_group_array(json(body)) FROM (SELECT body FROM {view} WHERE {foreign_key}={parent} ORDER BY {order}))'


def create_views(db):
    def view(table, exclude=(), extra=None, name=None):
        projection = object_sql((db, table), exclude, extra)
        db.execute(f'CREATE VIEW {name or table+"_json"} AS SELECT t.*, {projection} AS body FROM {table} t')
    for table in ['categories', 'destinations', 'reviews', 'campaigns', 'faqs']:
        view(table)
    price = "json_object('amount',t.amount,'original_amount',t.original_amount,'currency',t.currency,'unit',t.unit,'tax_included',json(CASE t.tax_included WHEN 1 THEN 'true' ELSE 'false' END))"
    view('options', ['product_id', 'amount', 'original_amount', 'currency', 'unit', 'tax_included'], {'price': price})
    view('itinerary_days', ['product_id'])
    view('collections', extra={'product_ids': "json((SELECT json_group_array(product_id) FROM (SELECT product_id FROM collection_products WHERE collection_id=t.id ORDER BY position)))"})
    view('articles', extra={'related_product_ids': "json((SELECT json_group_array(product_id) FROM (SELECT product_id FROM article_products WHERE article_id=t.id ORDER BY position)))"})
    options = f'json({group("options_json", "product_id", "t.product_id")})'
    itinerary = f'json({group("itinerary_days_json", "product_id", "t.product_id", "day")})'
    view('stay_details', ['product_id'], {'rooms': options})
    view('cruise_details', ['product_id'], {'cabins': options, 'itinerary': itinerary})
    view('tour_details', ['product_id'], {'plans': options, 'itinerary': itinerary})
    view('activity_details', ['product_id'], {'plans': options})
    card_fields = ['id', 'type', 'title', 'summary', 'destination_id', 'location_name', 'latitude', 'longitude',
                   'cover', 'tags', 'amenities', 'duration_days', 'popularity', 'free_cancellation', 'confirmation']
    all_columns = [row[1] for row in db.execute('PRAGMA table_info(products)')]
    excluded = [c for c in all_columns if c not in card_fields]
    card = object_sql((db, 'products'), excluded, {
        'free_cancellation': "json(CASE t.free_cancellation WHEN 1 THEN 'true' ELSE 'false' END)",
        'price': "json(json_extract(o.body,'$.price'))",
        'rating': "json((SELECT json_object('average',round(coalesce(avg(rating),0),1),'count',count(*)) FROM reviews WHERE product_id=t.id))",
        'detail_url': "'/api/v1/products/'||t.id",
    })
    db.execute(f'''CREATE VIEW product_cards AS SELECT t.*, o.amount AS price_amount, c.body AS cruise_body,
        s.property_type, c.departure_port, c.cruise_line, g.departure_city,
        (SELECT round(coalesce(avg(rating),0),1) FROM reviews WHERE product_id=t.id) AS rating_average,
        {card} AS body FROM products t
        JOIN options_json o ON o.id=(SELECT id FROM options WHERE product_id=t.id ORDER BY amount,position,id LIMIT 1)
        LEFT JOIN stay_details s ON s.product_id=t.id
        LEFT JOIN cruise_details_json c ON c.product_id=t.id
        LEFT JOIN tour_details g ON g.product_id=t.id''')
    details = object_sql((db, 'products'), card_fields, {'stay': "json((SELECT body FROM stay_details_json WHERE product_id=t.id))",
        'cruise': "json((SELECT body FROM cruise_details_json WHERE product_id=t.id))",
        'tour': "json((SELECT body FROM tour_details_json WHERE product_id=t.id))",
        'activity': "json((SELECT body FROM activity_details_json WHERE product_id=t.id))"})
    db.execute(f'CREATE VIEW product_details AS SELECT t.id,t.position,json_patch(c.body,{details}) AS body FROM products t JOIN product_cards c ON c.id=t.id')


def build_database(data, media):
    target = ROOT / 'data/travel.sqlite3'
    target.parent.mkdir(exist_ok=True)
    temporary = target.with_suffix('.sqlite3.tmp')
    temporary.unlink(missing_ok=True)
    try:
        with sqlite3.connect(temporary) as db:
            db.executescript((ROOT / 'db/schema.sql').read_text())
            for url, content in media.items():
                insert(db, 'media', {'url': url, 'mime_type': 'image/svg+xml', 'content': content.encode()})
            for table in ['categories', 'destinations']:
                for position, record in enumerate(data[table]):
                    insert(db, table, dict(record, position=position))
            for position, product in enumerate(data['products']):
                insert(db, 'products', dict(product, position=position))
                for kind, option_key in [('stay', 'rooms'), ('cruise', 'cabins'), ('tour', 'plans'), ('activity', 'plans')]:
                    if kind not in product:
                        continue
                    detail = product[kind]
                    insert(db, kind+'_details', dict(detail, product_id=product['id']))
                    for n, option in enumerate(detail[option_key]):
                        insert(db, 'options', dict(option, **option['price'], product_id=product['id'], position=n))
                    for day in detail.get('itinerary', []):
                        insert(db, 'itinerary_days', dict(day, product_id=product['id']))
            for table in ['reviews', 'collections', 'campaigns', 'articles', 'faqs']:
                for position, record in enumerate(data[table]):
                    insert(db, table, dict(record, position=position))
                    if table in ('collections', 'articles'):
                        relation = 'collection' if table == 'collections' else 'article'
                        key = 'product_ids' if table == 'collections' else 'related_product_ids'
                        for n, pid in enumerate(record[key]):
                            insert(db, relation+'_products', {relation+'_id': record['id'], 'product_id': pid, 'position': n})
            for key, value in data['settings'].items():
                insert(db, 'settings', {'key': key, 'value': encode(value)})
            create_views(db)
            assert db.execute('PRAGMA integrity_check').fetchone()[0] == 'ok'
            assert not db.execute('PRAGMA foreign_key_check').fetchall()
            db.commit()
            db.execute('VACUUM')
        os.replace(temporary, target)
    finally:
        temporary.unlink(missing_ok=True)
    print(f'Generated {target.relative_to(ROOT)} ({target.stat().st_size:,} bytes).')
