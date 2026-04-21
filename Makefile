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

.PHONY: test
test:
	go test -tags fts5 ./...

.PHONY: clean
clean:
	rm -f glean glean.db static/output.css

.PHONY: docker-build
docker-build:
	docker build -t glean:latest -t atcr.io/julien.rbrt.fr/glean:latest .

.PHONY: docker-push
docker-push:
	docker push atcr.io/julien.rbrt.fr/glean:latest
