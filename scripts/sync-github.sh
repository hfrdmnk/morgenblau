#!/usr/bin/env sh
set -eu

GITHUB_URL="git@github.com:hfrdmnk/morgenblau.git"

if ! git remote get-url github >/dev/null 2>&1; then
    echo "Adding 'github' remote → $GITHUB_URL"
    git remote add github "$GITHUB_URL"
fi

echo "Pruning deleted branches from origin (tangled)…"
git fetch origin --prune

echo "Mirroring local refs to github…"
git push --mirror github

echo "Done. GitHub now matches tangled."
