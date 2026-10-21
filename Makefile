GO ?= go
PYTHON ?= python3
GOFLAGS ?= -tags=nomsgpack
export GOFLAGS

.PHONY: run build test race vet generate examples verify verify-writes verify-examples
run:
	$(GO) run ./cmd/server
build:
	$(GO) build -o bin/travel-api ./cmd/server
test:
	$(GO) test ./...
race:
	$(GO) test -race ./...
vet:
	$(GO) vet ./...
generate:
	$(PYTHON) scripts/generate_fixtures.py
	$(GO) run ./cmd/specgen
	$(GO) run ./cmd/examplegen
verify:
	$(PYTHON) scripts/verify_api.py --base-url http://localhost:8080

verify-writes:
	$(PYTHON) scripts/verify_writes.py --base-url http://localhost:8080

examples:
	$(GO) run ./cmd/examplegen

verify-examples:
	$(PYTHON) scripts/verify_examples.py
