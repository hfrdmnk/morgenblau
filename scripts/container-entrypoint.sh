#!/bin/sh
set -eu

db_path="${DB_PATH:-/data/morgenblau.db}"
db_dir="$(dirname "$db_path")"
mkdir -p "$db_dir"
chown nobody:nogroup "$db_dir"
gosu nobody /app/goose -dir /app/migrations sqlite3 "$db_path" up

exec gosu nobody /app/morgenblau
