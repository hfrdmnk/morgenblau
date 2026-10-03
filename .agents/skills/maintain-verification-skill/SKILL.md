---
name: maintain-verification-skill
description: "Maintains Morgenblau's verification feature map, recipes and isolated harness from live evidence. Use for biweekly maintenance in a fresh orb or when asked to check verification-skill drift; leaves fixes local for review."
---

# Maintain Verification Skill

Compare verification coverage with the current app, drive the recipes, and repair documentation drift or harness regressions without hiding product failures.

## Scope and authority

- Intended cadence: every two weeks, each invocation in a fresh orb. This file defines the job, not a schedule; provisioning or changing a schedule requires separate authorization.
- Run unattended: make safe decisions, finish independent work, and report unavailable prerequisites as `blocked`, never `pass`.
- Own the behavioral maintenance of [verify-morgenblau](../verify-morgenblau/SKILL.md), its [feature map and recipes](../verify-morgenblau/features/README.md), and its isolated harness. Keep fixes within that skill directory.
- [Garden](../garden/SKILL.md) owns the broad static sweep and reference audit. Do not invoke its shipping workflow or duplicate its repository-wide checks.
- Leave all edits local for review. Do not commit, push, open PRs, create or update issues, send messages, publish evidence, or make other shared writes without additional user authorization.
- Do not change application behavior, spec decisions, or repo-wide rules. Record product bugs and ambiguous expectations for review instead.

## 1. Fresh-run preflight

1. Read the repository guidance, `SPEC.md` and `LAWS.md`. Load `verify-morgenblau` and use its current protocol, safety rules, proof standards and change-type checks throughout; do not substitute a copied command table. Load `building-skills` before editing any skill documentation.
2. Confirm this is a fresh orb and the checkout is clean, including untracked files. If not, stop as `blocked`; never reset, stash or overwrite someone else's work.
3. Fetch `origin`, switch to `main`, fast-forward only to `origin/main`, and verify a clean tree and identical HEAD. Record the baseline SHA. Fetch failure, divergence or inability to establish latest main is `blocked`; do not drive an old revision as current. Unshallow before any history investigation if needed.
4. Inspect the current verification harness help, dependency manifests and setup instructions. Install only the documented prerequisites using pinned versions; do not use personal credentials or copy a user's `.env`. Run the verification doctor before drives and again after surprises. Preserve its exit-code distinctions, including signed-in coverage blocked by unsupported Node.
5. Discover the harness smoke/regression entry point from the current skill directory, package scripts, Makefile or CI. If present, read its isolation requirements and run the actual command, recording output and exit status. Do not invent a target or assume one exists. If absent, report `not available` and use disposable smoke drives; missing dependencies for an existing check are `blocked`.

## 2. Compare coverage to supported behavior

Read the feature map and every linked recipe. Compare them to the current frontend routes, pages and API client, backend route registration and handlers, and supported jobs and newsletter paths. Inspect only the code needed to establish behavior.

- Track each mapped flow: recipe, current entry point, expected proof, negative proofs, and whether it can run here.
- Revisit the map's unmapped list: distinguish supported reachable behavior from placeholders, dev-only surfaces and intentionally unavailable integrations.
- Discover supported flows missing from the map, including API-only behavior. Record the source path and user/API entry point; do not claim coverage from code inspection alone.
- Use the spec and current implementation to identify drift. An implementation contradicting the spec is a product finding, not permission to weaken the recipe.
- Plan to drive every supported mapped recipe and safe unmapped flow. Explicitly list any omitted or blocked coverage and why; a partial sweep cannot be an overall pass.

## 3. Drive with isolation and evidence

Follow the loaded verification workflow for every recipe, including its state inspection, job settling, negative proofs and evidence capture.

- Give every attempt a unique slug combining feature, UTC timestamp and a random suffix. Retries get new slugs; never reuse another run's state or evidence directory.
- Use only harness-created instances, disposable PDSes and accounts. Never touch the user's server, account, database, `data/`, or a public PDS. Do not substitute an external account when local sign-in is blocked.
- Wrap each run in shell cleanup or equivalent `try/finally`: register the matching `down` before attempting `up`, and call it even after partial startup, failed assertions or abandoned work. Confirm the recorded processes and disposable state are gone while evidence remains. Cleanup failures are `blocked` and name the remaining run; never kill by process name.
- Keep secrets out of logs, screenshots and reports. Do not print account files, cookies, passwords, tokens, `.env` or secret-bearing state. Use the harness's secret-safe login controls and inspect evidence before retaining it. Evidence stays local in the verification workflow's evidence directories.
- Capture the actual action, observable result and state delta. Missing network access, fixtures, tools or safe isolation means `blocked`, not a skipped assertion counted as success.

## 4. Classify, repair, re-drive

- **Recipe or map drift:** evidence shows a supported flow works but the documented selector, path, setup, assertion or coverage is stale. Make the smallest local correction; add a recipe for a newly driven supported flow and link it from the map. Preserve meaningful negative proofs and document limitations honestly.
- **Harness regression:** isolate the failure from the product, reproduce it with the discovered regression check or a disposable smoke drive, and repair only the harness. Add focused regression coverage using its existing test conventions. Run the relevant check before and after the fix and re-drive affected recipes with fresh slugs.
- **Product bug:** the real user path violates an expectation. Stop that recipe, retain reproduction evidence, and report `fail`. Never rewrite assertions, change the entry point, seed away the failure, or alter app code to make maintenance green.
- **Uncertain or unavailable:** record the unresolved question or prerequisite as `blocked`; continue independent safe recipes.

After fixes, rerun the available harness checks and affected drives. Use the loaded verification skill's change-type static checks, plus content, relative-link and format checks for docs. Read the entire diff, including new files, and run `git diff --check`. Ensure only intended verification-skill files changed and every hunk traces to evidence. Leave the patch uncommitted.

## 5. Report

Give per-feature outcomes and one aggregate outcome: `fail` if any confirmed product or harness failure remains, otherwise `blocked` if any required coverage, prerequisite, check or cleanup is incomplete, otherwise `pass`. Include blockers even when the aggregate is `fail`. A corrected recipe can pass only after a successful re-drive.

```text
maintain-verification-skill: pass | fail | blocked
baseline: <main SHA, orb freshness, run identifier>
reason: <summary; all blockers and remaining failures>
features driven: <recipe/flow, slug, pass|fail|blocked and reason>
recipe fixes: <local paths, evidence-backed correction, re-drive result | none>
harness: <discovered checks and results, fixes and regression proof | unavailable>
unmapped flows: <supported flows, added coverage or explicit exclusions>
product bugs: <expected vs actual, reproduction and evidence | none>
evidence: <local paths per attempt; no secrets>
checks: <commands actually run and outcomes; content/link/diff checks>
cleanup: <down confirmed per run | remaining state and reason>
local changes: <files for review | none; no shared writes>
```
