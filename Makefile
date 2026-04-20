.PHONY: dev run build test clean

dev:
	@if [ -f .env ]; then set -a; . ./.env; set +a; fi && go run .

build:
	go build -o glean .

test:
	go test ./...

clean:
	rm -f glean glean.db
