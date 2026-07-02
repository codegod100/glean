SQLITE3_VER := $(shell grep 'mattn/go-sqlite3' go.mod | awk '{print $$2}')
SQLITE3_INC := $(shell go env GOMODCACHE)/github.com/mattn/go-sqlite3@$(SQLITE3_VER)
export CGO_CFLAGS := -I$(CURDIR)/internal/db/include -I$(SQLITE3_INC)

.PHONY: tools-install
tools-install:
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest

.PHONY: lint
lint:
	go vet ./...
	go fix ./...
	test -z "$(shell gofmt -l ./...)"
	golangci-lint run ./... --fix

.PHONY: web-install
web-install:
	cd web && bun install

# Run the Go API (backend) and the SvelteKit dev server (frontend) together.
# The frontend proxies /api requests to the Go server (GLEAN_API_URL).
.PHONY: dev
dev:
	@if [ -f .env ]; then set -a; . ./.env; set +a; fi; \
	GLEAN_FRONTEND_URL="$${GLEAN_FRONTEND_URL:-http://localhost:3000}"; \
	echo "Starting Go API on :8080 and SvelteKit on :3000 (Ctrl-C stops both)..."; \
	GLEAN_API_URL=http://localhost:8080 GLEAN_FRONTEND_URL=$$GLEAN_FRONTEND_URL go run -tags fts5 . & \
	GO_PID=$$!; \
	trap 'kill $$GO_PID 2>/dev/null' INT TERM EXIT; \
	(cd web && GLEAN_API_URL=http://localhost:8080 bun run dev); \
	kill $$GO_PID 2>/dev/null || true

.PHONY: dev-api
dev-api:
	@if [ -f .env ]; then set -a; . ./.env; set +a; fi; \
	GLEAN_FRONTEND_URL="$${GLEAN_FRONTEND_URL:-http://localhost:3000}" go run -tags fts5 .

.PHONY: dev-web
dev-web:
	cd web && GLEAN_API_URL=http://localhost:8080 bun run dev

.PHONY: web-build
web-build:
	cd web && bun run build

.PHONY: build
build: web-build
	go build -tags fts5 -o glean .

.PHONY: icons
icons:
	magick web/static/favicon.svg -background none -density 1200 -resize 512x512 -depth 8 PNG32:web/static/favicon.png
	magick web/static/favicon.svg -background none -density 1200 -resize 180x180 -depth 8 PNG32:web/static/apple-touch-icon.png

.PHONY: lex-lint
lex-lint:
	goat lex lint

.PHONY: lex-parse
lex-parse:
	goat lex parse $(shell find lexicons -name '*.json' 2>/dev/null)

.PHONY: test
test:
	go test -tags fts5 ./...

.PHONY: check
check:
	cd web && bun run check

.PHONY: clean
clean:
	rm -f glean glean.db
	rm -rf web/build web/.svelte-kit

.PHONY: docker-build
docker-build:
	docker build -t glean:latest -t atcr.io/julien.rbrt.fr/glean:latest .

.PHONY: docker-push
docker-push:
	docker push atcr.io/julien.rbrt.fr/glean:latest
