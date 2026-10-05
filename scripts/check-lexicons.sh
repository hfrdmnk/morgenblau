#!/usr/bin/env bash
set -euo pipefail

base="${1:?usage: check-lexicons.sh <base commit>}"
if ! git rev-parse --verify "${base}^{commit}" >/dev/null 2>&1; then
    printf 'Cannot check the lexicon freeze: base commit is unavailable; fetch the comparison history.\n' >&2
    exit 1
fi

changed="$(git diff --name-status "$base" HEAD -- ':(glob)lexicons/**/*.json')"
if [[ -n "$changed" ]]; then
    printf '%s\n' "$changed" >&2
    printf 'Keep lexicon schemas unchanged: SPEC.md freezes external compatibility during the v1 simplification, including additions, removals and revision bumps.\n' >&2
    exit 1
fi
