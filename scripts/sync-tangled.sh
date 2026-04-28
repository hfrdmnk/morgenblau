#!/usr/bin/env sh
set -eu

TANGLED_URL="git@tangled.org:dominik.social/morgenblau"

if ! git remote get-url tangled >/dev/null 2>&1; then
    echo "Adding 'tangled' remote → $TANGLED_URL"
    git remote add tangled "$TANGLED_URL"
fi

echo "Pruning deleted branches from origin (GitHub)…"
git fetch origin --prune

echo "Mirroring local refs to tangled…"
git push --mirror tangled

echo "Done. Tangled now matches GitHub."
