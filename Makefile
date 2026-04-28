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

.PHONY: lex-lint
lex-lint:
	goat lex lint

.PHONY: lex-parse
lex-parse:
	goat lex parse $(shell find lexicons -name '*.json' 2>/dev/null)

.PHONY: dev
dev: css
	@if [ -f .env ]; then set -a; . ./.env; set +a; fi && go run -tags fts5 .

.PHONY: build
build: css
	go build -tags fts5 -o glean .

.PHONY: css
css:
	npx tailwindcss -i ./static/input.css -o ./static/output.css --minify

.PHONY: css-watch
css-watch:
	npx tailwindcss -i ./static/input.css -o ./static/output.css --watch

.PHONY: icons
icons:
	magick static/favicon.svg -background none -density 1200 -resize 512x512 -depth 8 PNG32:static/favicon.png
	magick static/favicon.svg -background none -density 1200 -resize 180x180 -depth 8 PNG32:static/apple-touch-icon.png

.PHONY: test
test:
	go test -tags fts5 ./...

.PHONY: clean
clean:
	rm -f glean glean.db static/output.css static/favicon.png static/apple-touch-icon.png

.PHONY: docker-build
docker-build:
	docker build -t glean:latest -t atcr.io/julien.rbrt.fr/glean:latest .

.PHONY: docker-push
docker-push:
	docker push atcr.io/julien.rbrt.fr/glean:latest
