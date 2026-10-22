FROM node:24-slim AS build
COPY --from=oven/bun:1.3.11 /usr/local/bin/bun /usr/local/bin/bun

WORKDIR /src
COPY package.json bun.lock ./
RUN bun install --frozen-lockfile
COPY . .
RUN bun run build

FROM nginx:1.29-alpine
COPY --from=build /src/dist /usr/share/nginx/html
