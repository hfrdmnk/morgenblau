FROM oven/bun:1.3.11 AS frontend

WORKDIR /src
COPY frontend/package.json frontend/bun.lock ./frontend/
RUN bun install --cwd ./frontend --frozen-lockfile
COPY frontend ./frontend
RUN bun run --cwd ./frontend build

FROM golang:1.26.5-bookworm AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend /src/frontend/dist ./frontend/dist
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/morgenblau ./cmd/api
RUN GOBIN=/out go install github.com/pressly/goose/v3/cmd/goose@v3.26.0

FROM debian:bookworm-slim

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates gosu \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY --from=build /out/morgenblau /app/morgenblau
COPY --from=build /out/goose /app/goose
COPY internal/database/migrations /app/migrations
COPY scripts/container-entrypoint.sh /app/container-entrypoint.sh

RUN mkdir -p /data && chown -R nobody:nogroup /app

EXPOSE 8000 2525
ENTRYPOINT ["/bin/sh", "/app/container-entrypoint.sh"]
