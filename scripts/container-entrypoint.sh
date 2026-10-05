#!/bin/sh
set -eu

export DB_PATH="${DB_PATH:-/data/morgenblau.db}"
db_path="$DB_PATH"
db_dir="$(dirname "$db_path")"
replica="${LITESTREAM_REPLICA_URL:-}"
if [ "${APP_ENV:-}" = "production" ] && [ -z "$replica" ]; then
  echo 'LITESTREAM_REPLICA_URL is required in production' >&2
  exit 1
fi

mkdir -p "$db_dir"
chown nobody:nogroup "$db_dir"
if [ -n "$replica" ] && [ ! -e "$db_path" ]; then
  # Only the first deployment may initialize without a backup; recovery must fail closed.
  set --
  if [ "${LITESTREAM_ALLOW_EMPTY_REPLICA:-false}" = "true" ]; then
    set -- -if-replica-exists
  fi
  # Litestream publishes its output before validation, so it must not be the live database.
  restore_dir="$(gosu nobody mktemp -d "$db_dir/.restore.XXXXXX")"
  trap 'rm -rf "$restore_dir"' EXIT
  gosu nobody /app/litestream restore -config /etc/litestream.yml \
    -o "$restore_dir/database.db" -integrity-check quick "$@" "$db_path"
  if [ -f "$restore_dir/database.db" ]; then
    mv "$restore_dir/database.db" "$db_path"
  fi
  rm -rf "$restore_dir"
  trap - EXIT
fi

case "${SMTP_ACME_ENABLED:-false}" in
  true|TRUE|True|1|t|T)
    cert_dir="${SMTP_ACME_STORAGE:-/data/certmagic}"
    mkdir -p "$cert_dir"
    chmod 700 "$cert_dir"
    chown nobody:nogroup "$cert_dir"
    ;;
esac

gosu nobody /app/goose -dir /app/migrations sqlite3 "$db_path" up

if [ -n "$replica" ]; then
  exec gosu nobody /app/litestream replicate -config /etc/litestream.yml -exec /app/morgenblau
fi
exec gosu nobody /app/morgenblau
