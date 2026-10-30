#!/usr/bin/env bash
set -euo pipefail

base="${1:?usage: check-migrations.sh <base commit>}"
if ! git rev-parse --verify "${base}^{commit}" >/dev/null 2>&1; then
    printf 'Cannot check migrations: base commit is unavailable; fetch the comparison history.\n' >&2
    exit 1
fi

changed="$(git diff --name-status --no-renames --diff-filter=a "$base" HEAD -- internal/database/migrations)"
if [[ -n "$changed" ]]; then
    printf '%s\n' "$changed" >&2
    printf 'Add a new migration instead: goose records applied versions, so production never reruns an edited, renamed or deleted one.\n' >&2
    exit 1
fi
