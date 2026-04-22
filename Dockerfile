FROM golang:1.26-alpine AS builder

RUN apk add --no-cache gcc musl-dev nodejs npm

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY package.json package-lock.json ./
RUN npm ci

COPY . .
RUN npx tailwindcss -i ./static/input.css -o ./static/output.css --minify

RUN --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=1 go build -tags fts5 -ldflags="-s -w" -o /glean .

FROM alpine:3.21

RUN apk add --no-cache ca-certificates

COPY --from=builder /glean /usr/local/bin/glean

EXPOSE 8080
ENTRYPOINT ["glean"]
