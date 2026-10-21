#!/usr/bin/env python3
"""Record or compare existing GET responses during a transport migration."""
import argparse
import json
import urllib.parse
from verify_api import request


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--base-url', default='http://localhost:8080')
    parser.add_argument('--snapshot', required=True)
    parser.add_argument('--record', action='store_true')
    args = parser.parse_args()
    base = args.base_url.rstrip('/')
    if args.record:
        spec = json.loads(request(base, '/openapi.json')[2])
        paths = {op['get']['x-smoke-path'] for op in spec['paths'].values() if 'get' in op}
        cards = json.loads(request(base, '/api/v1/products?per_page=100')[2])['data']
        for card in cards:
            path = card['detail_url']
            resource = {'stay': 'rooms', 'cruise': 'cabins'}.get(card['type'], 'plans')
            paths.update((path, path+'/'+resource, path+'/calendar', path+'/availability', path+'/reviews', path+'/related'))
            if card['type'] in ('cruise', 'tour'):
                paths.update((path+'/itinerary', path+'/departures'))
        paths.update('/api/v1/products?'+urllib.parse.urlencode({key: value}) for key, values in {
            'type': ['stay','cruise','tour','activity','transfer','esim','sport','rail'],
            'sort': ['price_asc','price_desc','rating','duration'],
            'country': ['TW','JP','KR','TH','SG'], 'q': ['東京','no-result',"' OR 1=1 --"],
            'min_rating': ['4.5','NaN'], 'page': ['0','2','999999'],
            'bbox': ['121,24,122,26','NaN,0,10,10'], 'free_cancellation': ['true','false'],
        }.items() for value in values)
        paths.update(('/missing', '/api/v1/products/missing', '/api/v1/products/stay-001/cabins'))
        baseline = {}
        for path in sorted(paths):
            status, _, raw = request(base, path)
            baseline[path] = {'status': status, 'body': json.loads(raw)}
        with open(args.snapshot, 'w') as output:
            json.dump(baseline, output, ensure_ascii=False)
        print(f'Recorded {len(baseline)} GET responses.')
    else:
        with open(args.snapshot) as source:
            baseline = json.load(source)
        for path, expected in baseline.items():
            status, _, raw = request(base, path)
            assert {'status': status, 'body': json.loads(raw)} == expected, path
        print(f'PASS: {len(baseline)} GET responses unchanged.')


if __name__ == '__main__':
    main()
