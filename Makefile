.PHONY: lint
lint:
	go vet ./...
	go fix ./...
	test -z "$(shell gofmt -l ./...)"
	golangci-lint run ./... --fix

.PHONY: lint-install
lint-install:
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest

.PHONY: dev
dev: css
	@if [ -f .env ]; then set -a; . ./.env; set +a; fi && go run .

.PHONY: build
build: css
	go build -o glean .

.PHONY: css
css:
	npx tailwindcss -i ./static/input.css -o ./static/output.css --minify

.PHONY: css-watch
css-watch:
	npx tailwindcss -i ./static/input.css -o ./static/output.css --watch

.PHONY: test
test:
	go test ./...

.PHONY: clean
clean:
	rm -f glean glean.db static/output.css

.PHONY: docker-build
docker-build:
	docker build -t glean:latest -t atcr.io/julien.rbrt.fr/glean:latest .

.PHONY: docker-push
docker-push:
	docker push atcr.io/julien.rbrt.fr/glean:latest
