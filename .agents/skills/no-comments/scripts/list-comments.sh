#!/usr/bin/env bash
set -euo pipefail
[ $# -gt 0 ] || { echo "usage: $0 <path under app/>... such as internal/sync or frontend/src/hooks" >&2; exit 2; }
cd "$(git rev-parse --show-toplevel)/app"

go_prefixes=()
frontend_paths=()
for path in "$@"; do
    path="${path#app/}"
    case "$path" in
        frontend | frontend/*) frontend_paths+=("${path#frontend}") ;;
        *) go_prefixes+=("$path") ;;
    esac
done

show() {
    while IFS=: read -r file line; do
        printf '%s:%s: %s\n' "$file" "$line" "$(sed -n "${line}p" "$file" | sed 's/^[[:space:]]*//')"
    done
}

if [ ${#go_prefixes[@]} -gt 0 ]; then
    backup="$(mktemp)"
    cp comments-baseline.json "$backup"
    trap 'cp "$backup" comments-baseline.json; rm -f "$backup"' EXIT
    jq --args 'with_entries(select(.key as $k | $ARGS.positional | any(. as $p | $k | startswith($p)) | not))' \
        "${go_prefixes[@]}" <"$backup" >comments-baseline.json
    { go test ./internal/comments -run TestCommentsStayWithinBaseline -count=1 || true; } |
        sed -n 's/^[[:space:]]*\([^[:space:]]*\.go:[0-9]*\)$/\1/p' | show
fi

if [ ${#frontend_paths[@]} -gt 0 ]; then
    empty="$(mktemp)"
    echo '{}' >"$empty"
    (
        cd frontend
        targets=()
        for path in "${frontend_paths[@]}"; do targets+=("${path#/}"); done
        [ "${targets[*]}" = "" ] && targets=(.)
        { ./node_modules/.bin/eslint "${targets[@]}" --suppressions-location "$empty" --format json || true; } |
            jq -r --arg root "$PWD/" '.[] | (.filePath | ltrimstr($root)) as $f | .messages[] | select(.ruleId == "comments/no-comments") | "\($f):\(.line)"' |
            show | sed 's|^|frontend/|'
    )
    rm -f "$empty"
fi
