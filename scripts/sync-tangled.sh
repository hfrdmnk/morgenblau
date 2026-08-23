#!/usr/bin/env sh
set -eu

TANGLED_URL="git@tangled.org:dominik.social/morgenblau"

if ! git remote get-url tangled >/dev/null 2>&1; then
    echo "Adding 'tangled' remote: $TANGLED_URL"
    git remote add tangled "$TANGLED_URL"
fi

if [ "$(git remote get-url tangled)" != "$TANGLED_URL" ]; then
    echo "The 'tangled' remote does not point to $TANGLED_URL" >&2
    exit 1
fi

echo "Fetching main from origin (GitHub)..."
git fetch origin main

echo "Pushing origin/main to Tangled..."
git push --force tangled refs/remotes/origin/main:refs/heads/main

echo "Done. Tangled main matches GitHub main."
