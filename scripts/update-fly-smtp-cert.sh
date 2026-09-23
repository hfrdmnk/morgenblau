#!/bin/sh
set -eu

: "${FLY_APP:?set FLY_APP to the Fly application name}"
: "${RENEWED_LINEAGE:?Certbot must set RENEWED_LINEAGE}"

umask 077
secrets_file="$(mktemp)"
trap 'rm -f "$secrets_file"' EXIT HUP INT TERM

{
  printf 'SMTP_TLS_CERT_B64='
  base64 < "$RENEWED_LINEAGE/fullchain.pem" | tr -d '\n'
  printf '\nSMTP_TLS_KEY_B64='
  base64 < "$RENEWED_LINEAGE/privkey.pem" | tr -d '\n'
  printf '\n'
} > "$secrets_file"

fly secrets import --app "$FLY_APP" < "$secrets_file"
