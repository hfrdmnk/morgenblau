#!/usr/bin/env bash
set -euo pipefail

test_file="$(realpath "${BASH_SOURCE[0]}")"
source "$(dirname "$test_file")/../bin/verify"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
VERIFY_ROOT="$tmp/verify"
mkdir -p "$VERIFY_ROOT"

test_same_slug_is_checked_under_lock() {
    local dir="$VERIFY_ROOT/$(date +%F)-collision" result
    node_major() { printf 0; }
    oauth_scope() { printf atproto; }
    lock_ports() {
        mkdir -p "$dir/run"
        printf '%s\n' '{"status":"starting","base_url":"http://127.0.0.1:8199","winner":true}' >"$dir/run/state.json"
    }
    claimed_ports() { printf 'allocated after another start reserved the slug\n' >&2; return 99; }
    result="$(up collision 2>&1)" && return 1
    [[ "$result" == *"already up"* ]]
    jq -e '.winner == true' "$dir/run/state.json" >/dev/null
}

test_abandoned_startup_is_stale() {
    local dir="$VERIFY_ROOT/abandoned" reason
    mkdir -p "$dir/run"
    jq -n --argjson pid "$$" --argjson now "$(date +%s)" \
        '{status:"starting", startup_pid:$pid, startup_start:"previous process", pids:{}, started_epoch:$now}' >"$dir/run/state.json"
    reason="$(stale_reason "$dir")"
    [[ "$reason" == *"startup process is gone"* ]]
}

test_live_startup_is_not_stale() {
    local dir="$VERIFY_ROOT/live"
    mkdir -p "$dir/run"
    jq -n --argjson pid "$$" --arg start "$(pid_start "$$")" --argjson now "$(date +%s)" \
        '{status:"starting", startup_pid:$pid, startup_start:$start, pids:{}, started_epoch:$now}' >"$dir/run/state.json"
    if stale_reason "$dir"; then return 1; fi
}

test_restarted_service_is_live() {
    local dir="$VERIFY_ROOT/restarted"
    mkdir -p "$dir/run"
    jq -n --argjson now "$(date +%s)" \
        '{status:"up", pids:{server:999999999}, services:{server:"verify-example-server"}, started_epoch:$now}' >"$dir/run/state.json"
    systemctl() {
        [[ "$*" == *amp-svc-verify-example-server.service* ]]
        printf 'LoadState=loaded\nActiveState=active\n'
    }
    if stale_reason "$dir"; then return 1; fi
    [[ "$(process_status "$dir" server)" == yes ]]
}

test_unavailable_supervisor_blocks_cleanup() {
    local dir="$VERIFY_ROOT/unavailable" code
    mkdir -p "$dir/run"
    printf '%s\n' '{"slug":"unavailable","status":"up","pids":{"pds":999999999,"server":999999999},"services":{"pds":"verify-example-pds","server":"verify-example-server"},"started_epoch":0}' >"$dir/run/state.json"
    systemctl() {
        [[ "$*" == *amp-svc-verify-example-pds.service* ]] || return 1
        printf 'LoadState=loaded\nActiveState=inactive\n'
    }
    down_run() { printf 'unexpected cleanup\n' >&2; return 99; }
    if stale_reason "$dir" >"$tmp/reason"; then return 1; else code=$?; fi
    [[ "$code" == 2 ]]
    if stale false >"$tmp/status" 2>&1; then return 1; fi
    grep -q 'blocked:' "$tmp/status"
    [[ "$(state "$dir" .status)" == up ]]
    [[ "$(process_status "$dir" server)" == unknown ]]
}

test_stopped_service_is_stale() {
    local dir="$VERIFY_ROOT/stopped" reason
    mkdir -p "$dir/run"
    jq -n --argjson pid "$$" --arg start "$(pid_start "$$")" --argjson now "$(date +%s)" \
        '{status:"up", pids:{server:$pid}, pid_starts:{server:$start}, services:{server:"verify-example-server"}, started_epoch:$now}' >"$dir/run/state.json"
    systemctl() { printf 'LoadState=loaded\nActiveState=inactive\n'; }
    reason="$(stale_reason "$dir")"
    [[ "$reason" == *"server process is gone"* ]]
}

test_doctor_rejects_unusable_browser() {
    local result
    tools_installed() { return 0; }
    browser_ready() { return 1; }
    result="$(doctor 2>&1)" && return 1
    [[ "$result" == *"Chrome missing or unusable"* ]]
}

test_orb_services_keep_secrets_out_of_command() {
    local dir="$VERIFY_ROOT/2026-10-02-sign-in-20261002t190000-suffix" service script
    mkdir -p "$dir/run"
    printf '%s\n' "${VERIFY_TEST_LEAK-}" >"$tmp/amp-calls"
    printf '%s\n' '{"slug":"service","pids":{},"pid_starts":{},"services":{}}' >"$dir/run/state.json"
    AMP_ORB=1
    amp() {
        printf '%s\n' "$*" >>"$tmp/amp-calls"
        printf '%s\n' "$$" >"$dir/run/pds.pid"
    }
    start_process "$dir" pds "$ROOT" 'EXAMPLE_PASSWORD=fixture secret' -- sleep 60
    service="$(jq -r .services.pds "$dir/run/state.json")"
    [[ "$service" == verify-*-pds ]]
    [[ "${#service}" -le 32 ]]
    grep -q 'orb service start' "$tmp/amp-calls"
    if grep -q 'fixture secret\|EXAMPLE_PASSWORD' "$tmp/amp-calls"; then return 1; fi
    script="$dir/run/pds.sh"
    [[ "$(stat -c %a "$script")" == 700 ]]
    grep -q 'export EXAMPLE_PASSWORD=' "$script"
    playwright_cli() { return 0; }
    down_run "$dir" false
    grep -q "orb service stop $service" "$tmp/amp-calls"
    [[ ! -e "$script" ]]
    [[ "$(jq -r .status "$dir/run/state.json")" == down ]]
}

test_service_secret_assertion_rejects_a_leak() {
    if VERIFY_TEST_LEAK=EXAMPLE_PASSWORD bash "$test_file" test_orb_services_keep_secrets_out_of_command >"$tmp/leak-result" 2>&1; then return 1; fi
}

test_browser_uses_owned_secret_config() {
    local dir="$VERIFY_ROOT/browser-config"
    mkdir -p "$dir/run"
    printf '%s\n' '{"slug":"browser-config","base_url":"http://127.0.0.1:8100","pds":{}}' >"$dir/run/state.json"
    printf '%s\n' '{"password":"example-account-credential","app_password":"example-app-credential"}' >"$dir/run/account.json"
    playwright_cli() { printf '%s\n' "$*" >>"$tmp/browser-calls"; }
    browser "$dir" --headed false open /login
    grep -q -- "--config=$dir/run/playwright.config.json" "$tmp/browser-calls"
    [[ "$(stat -c %a "$dir/run/playwright.config.json")" == 600 ]]
    jq -e '.secrets.MORGENBLAU_VERIFY_PASSWORD == "example-account-credential" and .secrets.MORGENBLAU_VERIFY_APP_PASSWORD == "example-app-credential"' "$dir/run/playwright.config.json" >/dev/null
    browser "$dir" fill-password "getByRole('textbox', { name: 'Password' })"
    grep -q 'fill .* MORGENBLAU_VERIFY_PASSWORD' "$tmp/browser-calls"
    browser "$dir" open -- /login
    grep -q -- '--config=.* open -- http://127.0.0.1:8100/login' "$tmp/browser-calls"
    if grep -q 'example-account-credential\|example-app-credential' "$tmp/browser-calls"; then return 1; fi
    down_run "$dir" false
    [[ ! -e "$dir/run/playwright.config.json" ]]
}

test_live_browser_redacts_password() {
    local verify="$ROOT/.agents/skills/verify-morgenblau/bin/verify" slug="sign-in-redaction-$(date +%s)-$RANDOM" dir handle
    trap "$(printf %q "$verify") down $(printf %q "$slug") >/dev/null 2>&1; rm -rf $(printf %q "$tmp")" EXIT
    "$verify" doctor
    "$verify" up "$slug" >"$tmp/live-state.json"
    dir="$(jq -r .dir "$tmp/live-state.json")"
    handle="$(jq -r .pds.handle "$tmp/live-state.json")"
    "$verify" browser "$slug" open /login >"$dir/browser-open.txt"
    "$verify" browser "$slug" fill "getByRole('textbox', { name: 'Handle' })" "$handle" >"$dir/browser-handle.txt"
    "$verify" browser "$slug" click "getByRole('button', { name: 'Continue' })" >"$dir/browser-continue.txt"
    "$verify" browser "$slug" fill-password "getByRole('textbox', { name: 'Password' })"
    "$verify" browser "$slug" snapshot >"$dir/password-snapshot.txt"
    "$verify" browser "$slug" eval '() => document.querySelector("input[type=password]").value' --raw >"$dir/password-eval.txt"
    "$verify" browser "$slug" eval '() => console.log(document.querySelector("input[type=password]").value)' >"$dir/password-console-action.txt"
    "$verify" browser "$slug" console >"$dir/password-console.txt"
    "$verify" browser "$slug" requests >"$dir/password-requests.txt"
    node - "$dir" "$verify" "$slug" <<'JS'
const fs = require('fs');
const path = require('path');
const { spawnSync } = require('child_process');
const [dir, verify, slug] = process.argv.slice(2);
const account = JSON.parse(fs.readFileSync(path.join(dir, 'run/account.json')));
const secrets = [account.password, account.app_password];
const marker = '<secret>MORGENBLAU_VERIFY_PASSWORD</secret>';
const unsafeFiles = [];
function checkEvidence(directory) {
    for (const entry of fs.readdirSync(directory, { withFileTypes: true })) {
        if (entry.name === 'run' || entry.isSymbolicLink()) continue;
        const file = path.join(directory, entry.name);
        if (entry.isDirectory()) checkEvidence(file);
        else if (secrets.some(secret => fs.readFileSync(file).includes(Buffer.from(secret)))) unsafeFiles.push(file);
    }
}
checkEvidence(dir);
for (const file of unsafeFiles) fs.unlinkSync(file);
if (unsafeFiles.length) throw new Error('plaintext credentials found; unsafe artifacts removed');
for (const name of ['password-snapshot.txt', 'password-eval.txt', 'password-console.txt']) {
    if (!fs.readFileSync(path.join(dir, name), 'utf8').includes(marker)) throw new Error(`${name} did not redact the populated password`);
}
if (spawnSync(verify, ['down', slug], { stdio: 'inherit' }).status !== 0) throw new Error('cleanup failed');
for (const name of ['account.json', 'playwright.config.json', 'morgenblau.db', 'pds']) {
    if (fs.existsSync(path.join(dir, 'run', name))) throw new Error(`${name} survived cleanup`);
}
checkEvidence(dir);
for (const file of unsafeFiles) fs.unlinkSync(file);
if (unsafeFiles.length) throw new Error('plaintext credentials found after cleanup; unsafe artifacts removed');
console.log(`PASS live browser secret redaction and cleanup; evidence: ${dir}`);
JS
    rm -rf "$tmp"
    trap - EXIT
}

test_cleanup_continues_after_service_stop_failure() {
    local dir="$VERIFY_ROOT/cleanup"
    mkdir -p "$dir/run/pds"
    printf '%s\n' '{"slug":"cleanup","services":{"server":"server","vite":"vite","pds":"pds"}}' >"$dir/run/state.json"
    playwright_cli() { return 0; }
    amp() {
        printf '%s\n' "$*" >>"$tmp/amp-calls"
        [[ "$4" != server ]]
    }
    if (down_run "$dir" false) >"$tmp/result" 2>&1; then return 1; fi
    grep -q 'orb service stop vite' "$tmp/amp-calls"
    grep -q 'orb service stop pds' "$tmp/amp-calls"
    [[ -d "$dir/run/pds" ]]
}

test_browser_guard_rejects_escapes() {
    local dir="$VERIFY_ROOT/browser" args
    mkdir -p "$dir/run"
    ln -s "$tmp" "$dir/escape"
    for args in 'run-code' 'open https://example.com' 'screenshot --filename=/tmp/out.png' 'screenshot --filename=run/private.png' 'screenshot --filename=escape/out.png' 'snapshot --config=/tmp/config.json'; do
        if (guard_browser_args "$dir" http://127.0.0.1:8100 $args) >"$tmp/refusal" 2>&1; then return 1; fi
        grep -q 'verify browser refused:' "$tmp/refusal"
    done
    guard_browser_args "$dir" http://127.0.0.1:8100 goto /login
    [[ "${ARGS[1]}" == http://127.0.0.1:8100/login ]]
    guard_browser_args "$dir" http://127.0.0.1:8100 screenshot --filename=allowed.png
}

if [[ $# -gt 0 ]]; then "$1"; exit; fi

failed=0
for test in test_same_slug_is_checked_under_lock test_abandoned_startup_is_stale test_live_startup_is_not_stale test_restarted_service_is_live test_unavailable_supervisor_blocks_cleanup test_stopped_service_is_stale test_doctor_rejects_unusable_browser test_orb_services_keep_secrets_out_of_command test_service_secret_assertion_rejects_a_leak test_browser_uses_owned_secret_config test_cleanup_continues_after_service_stop_failure test_browser_guard_rejects_escapes; do
    if bash "$test_file" "$test" >"$tmp/result" 2>&1; then
        printf 'PASS %s\n' "$test"
    else
        printf 'FAIL %s\n' "$test"
        cat "$tmp/result"
        failed=$((failed + 1))
    fi
done
[[ "$failed" == 0 ]]
