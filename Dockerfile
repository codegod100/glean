FROM golang:1.26-alpine AS builder

RUN apk add --no-cache gcc musl-dev nodejs npm && \
    npm install -g bun

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY package.json bun.lock ./
RUN bun install --frozen-lockfile

COPY . .
RUN bunx tailwindcss -i ./static/input.css -o ./static/output.css --minify

RUN --mount=type=cache,target=/root/.cache/go-build \
    CGO_CFLAGS="-I/src/internal/db/include -I$(go env GOMODCACHE)/github.com/mattn/go-sqlite3@$(grep 'mattn/go-sqlite3' go.mod | awk '{print $2}') -Du_int8_t=uint8_t -Du_int16_t=uint16_t -Du_int64_t=uint64_t" \
    CGO_ENABLED=1 go build -tags fts5 -ldflags="-s -w" -o /glean .

FROM alpine:3.21

RUN apk add --no-cache ca-certificates

COPY --from=builder /glean /usr/local/bin/glean

EXPOSE 8080
ENTRYPOINT ["glean"]
