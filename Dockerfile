# ── Build stage (pure Go, no CGO needed thanks to modernc/sqlite)
FROM golang:1.22-alpine AS build
WORKDIR /app
COPY go.mod ./
RUN go mod download
COPY . .
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    CGO_ENABLED=0 go build -o server ./cmd/server

# ── Runtime
FROM alpine:3.20
WORKDIR /app
RUN adduser -D -h /app app
COPY --from=build /app/server /app/server
COPY web /app/web
RUN mkdir -p /app/data && chown -R app:app /app
USER app
EXPOSE 8080
ENTRYPOINT ["/app/server"]
