FROM golang:1.26-alpine AS builder

RUN apk add --no-cache gcc musl-dev nodejs npm && \
    npm install -g bun

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY web/package.json web/bun.lock* ./web/
RUN cd web && bun install --frozen-lockfile

COPY . .

# Build the SvelteKit frontend (SSR via adapter-node).
RUN cd web && bun run build

# Build the Go API binary.
RUN --mount=type=cache,target=/root/.cache/go-build \
    CGO_CFLAGS="-I/src/internal/db/include -I$(go env GOMODCACHE)/github.com/mattn/go-sqlite3@$(grep 'mattn/go-sqlite3' go.mod | awk '{print $2}') -Du_int8_t=uint8_t -Du_int16_t=uint16_t -Du_int64_t=uint64_t" \
    CGO_ENABLED=1 go build -tags fts5 -ldflags="-s -w" -o /glean .

FROM node:22-alpine

RUN apk add --no-cache ca-certificates

# Go API binary.
COPY --from=builder /glean /usr/local/bin/glean

# SvelteKit SSR build output.
COPY --from=builder /src/web/build /app/web/build
COPY --from=builder /src/web/package.json /app/web/package.json

WORKDIR /app/web

# SvelteKit proxies /api to the Go API on localhost:8080.
ENV GLEAN_API_URL=http://127.0.0.1:8080
ENV PORT=3000
ENV ORIGIN=http://localhost:3000

EXPOSE 3000

# Run the Go API in the background, then the SvelteKit Node server in front.
CMD sh -c 'GLEAN_API_URL=http://127.0.0.1:8080 GLEAN_ADDR=127.0.0.1:8080 glean & node build/index.js'
