.PHONY: dev run build test clean css css-watch

dev: css
	@if [ -f .env ]; then set -a; . ./.env; set +a; fi && go run .

build: css
	go build -o glean .

css:
	npx tailwindcss -i ./static/input.css -o ./static/output.css --minify

css-watch:
	npx tailwindcss -i ./static/input.css -o ./static/output.css --watch

test:
	go test ./...

clean:
	rm -f glean glean.db static/output.css
