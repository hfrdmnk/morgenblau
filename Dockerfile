FROM golang:1.24.4-alpine AS build

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN go build -o main cmd/api/main.go

FROM alpine:3.20.1 AS prod
WORKDIR /app
COPY --from=build /app/main /app/main
EXPOSE ${PORT}
CMD ["./main"]


FROM oven/bun:1.3.11-alpine AS frontend_builder
WORKDIR /frontend

COPY frontend/package.json frontend/bun.lock* ./
RUN bun install --frozen-lockfile
COPY frontend/. .
RUN bun run build

FROM oven/bun:1.3.11-alpine AS frontend
RUN bun add -g serve
COPY --from=frontend_builder /frontend/dist /app/dist
EXPOSE 5173
CMD ["serve", "-s", "/app/dist", "-l", "5173"]
