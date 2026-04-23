#!/usr/bin/env sh
files=$(git diff --name-only --diff-filter=ACMR -- resources/; git ls-files --others --exclude-standard -- resources/)
[ -z "$files" ] && exit 0
echo "$files" | xargs prettier --write --ignore-unknown
