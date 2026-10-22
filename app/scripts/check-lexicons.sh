#!/usr/bin/env bash
set -euo pipefail

base="${1:?usage: check-lexicons.sh <base commit>}"
if ! git rev-parse --verify "${base}^{commit}" >/dev/null 2>&1; then
    printf 'Cannot check the lexicon freeze: base commit is unavailable; fetch the comparison history.\n' >&2
    exit 1
fi

head_tree="HEAD:$(git rev-parse --show-prefix)lexicons"
base_tree="$base:$(git rev-parse --show-prefix)lexicons"
# Bases from before the app moved into app/ keep lexicons at the repo root.
git cat-file -e "$base_tree" 2>/dev/null || base_tree="$base:lexicons"
changed="$(git diff --name-status "$base_tree" "$head_tree" -- ':(top,glob)**/*.json')"
if [[ -n "$changed" ]]; then
    printf '%s\n' "$changed" >&2
    printf 'Keep lexicon schemas unchanged: SPEC.md freezes external compatibility during the v1 simplification, including additions, removals and revision bumps.\n' >&2
    exit 1
fi
