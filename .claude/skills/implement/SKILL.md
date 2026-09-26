---
name: implement
description: Implement agreed Morgenblau work from an interview-with-spec conversation, a PRD, or a planned issue. Carry settled decisions through Go test-first development, project verification, and the global review skill. Use for requests such as "implement what we agreed", "build slice 03", or "implement this PRD" once the scope is decided.
---

# Implement

Turn agreed work into a verified, reviewable change. The global `interview-with-spec` skill owns unresolved product decisions; this skill carries settled decisions into code; the global `review` skill provides independent review where its scope applies.

## 1. Recover the handoff

Resolve the requested work from the explicit argument or the current conversation:

- **After an interview:** use the user's settled answers, accepted corrections, and implementation request. An interview need not produce a PRD. `interview-with-spec` deliberately records only durable decisions in `SPEC.md`, so the spec alone may not contain the full task.
- **PRD or plan:** read the supplied document, including acceptance criteria, testing decisions, exclusions, and open questions. Implement the requested scope; a reference to one slice does not authorize the whole PRD.
- **Issue file:** read the issue, its parent PRD, and its blockers. A bare slice number must resolve within the task already identified in the conversation. Ask if multiple tasks match; never choose by modification time alone.
- **No identifiable work:** ask what to implement. Do not start a new feature from the general release scope in `SPEC.md`.

Read `SPEC.md`, the applicable `AGENTS.md` files, and `.claude/rules/testing.md`. Inspect the relevant code and tests before asking the user for facts. Confirm blockers against the current implementation, not just a checkbox.

Before editing, give a brief handoff summary: intended behavior and acceptance criteria, exclusions, the affected modules, test seams, and any remaining decisions. Preserve explicitly agreed seams and constraints. If no seam was discussed, identify an existing seam that exercises the behavior and state it; ask before introducing or changing an architectural seam.

A request to implement a settled interview is the go-ahead. Do not replay the interview or require another general approval. Assistant proposals and unanswered interview questions are not decisions. If a material ambiguity, unfinished blocker, or conflict remains, pause the dependent work and use `interview-with-spec` to resolve only the open branch. Follow explicit user corrections over stale artifacts and surface any spec conflict before changing the affected behavior.

Do not rewrite the PRD or spec to fit the implementation. Capture newly agreed durable decisions through `interview-with-spec` under its existing documentation gate.

## 2. Establish the working scope

Inspect the current branch, staged and unstaged diffs, and untracked files. Record pre-existing changes so verification, review, and any later commit can distinguish this task's work. Preserve other work, including edits in files this task also touches.

Work on the current branch unless the user instructs otherwise. Never create a branch without confirmation. A commit, push, PR, merge, or deployment requires authorization for that action; invoking this skill alone does not supply it. Preserve configured signing and hooks when publication is authorized.

Use one coherent vertical slice at a time, covering the layers needed for its observable behavior. For a request spanning several slices, follow their dependency order and keep going through the authorized scope. Do not turn slice completion into repeated approval requests.

## 3. Implement at the agreed seams

For Go changes, follow the red-green-refactor workflow in `AGENTS.md` and `internal/AGENTS.md`:

1. Write a focused test for the next observable behavior at the agreed seam.
2. Run it and observe the intended failure before writing the implementation. An environment or fixture failure does not establish red.
3. Implement the smallest change that makes it pass, then simplify within the slice while keeping tests green.

Use the project's existing testing patterns. Tests should survive internal refactors and exercise the contract, including relevant failure paths. Do not add an abstraction solely to make a low-level mock convenient. Frontend behavior uses the existing Bun test setup where useful; visual-only changes do not need tests that merely restate markup or class names.

Load additional guidance only for the work at hand:

- **UI:** `BRAND.md` and `frontend/AGENTS.md` before visual work. For motion, use a relevant animation skill.
- **Storage:** `internal/database/AGENTS.md`. Edit SQL sources and regenerate through `make sqlc`; do not hand-edit generated Go. Verify migrations on a disposable database, including Down, without resetting the user's database.
- **ATProto:** the matching protocol skill required by root `AGENTS.md`. Resolve the installed skill by name; if required guidance is unavailable, report the gap before dependent protocol changes.
- **Ownership and synchronization:** the relevant sections of `SPEC.md` and the implementation they point to. Preserve the distinction between PDS-authoritative records, the shared upstream cache, and private newsletter data.

Keep acceptance criteria in view while implementing. A newly discovered product decision returns to the interview; an ordinary implementation detail within the agreed design does not.

## 4. Verify the completed scope

Use `Makefile`, `frontend/package.json`, and `.github/workflows/ci.yml` as the maintained command sources. Select checks by the changed behavior, and report their actual results.

- During Go development, run the focused package/test selection, for example `go test ./internal/api -run TestName -count=1`. Finish Go work with the full Go suite and `go vet ./...`. For concurrency changes, use `make test-race` as the final suite run rather than repeating both full variants.
- During frontend development, run relevant test files and type checking as needed. Finish frontend work with its test, lint, and build scripts using Bun. The build includes TypeScript checking; there is no separate `types` script. Follow `frontend/AGENTS.md` for the feature-level `doctor` check.
- For changes affecting the embedded application or deployment build, run `make build-linux` to verify the pure-Go Linux build. Avoid rebuilding the frontend twice when this target already covers it.
- After schema/query changes, inspect the regenerated sqlc diff and run the affected storage/integration tests against real temporary SQLite files.
- Run `git diff --check` and inspect the final diff, including added files. Documentation-only work needs content, reference, and format validation rather than unrelated application test runs.

Root `AGENTS.md` reserves browser navigation for the user, including during delegated review. Where manual verification is needed, give the user concrete actions and expected results, and mark them pending until feedback arrives. For runtime checks through logs or HTTP, use `README.md` and `Makefile` for launch instructions and `internal/server/server.go` for startup behavior. Stop only processes started for this task.

Fix failures caused by this work. Identify unrelated baseline failures or environment blockers precisely; do not claim a blocked check passed. After fixes, rerun the affected checks, expanding coverage when the change warrants it.

## 5. Hand off to the global review skill

Read and invoke the installed global `review` skill for changes within its scope. Respect its exclusions for documentation-only work and simple single-file fixes; inspect those directly. Do not maintain a second lens catalog here.

Give `review` the agreed behavior, relevant spec constraints, test results, and a precise diff scope. Include staged, unstaged, and new files that belong to this task. Identify pre-existing edits so reviewers have context without treating them as this implementation. Use the global skill's scope and lens confirmation step, reusing explicit selections already supplied by the user. Honor any requested model and reasoning level.

The review skill owns dispatch and synthesis. Reviewers remain read-only and obey the project's manual-browser rule. After findings are presented, the implementer fixes verified P1/P2 issues within the authorized scope and reruns the affected checks. Return to the interview if a finding requires a new product or architectural decision. Re-review materially changed behavior; do not repeat an unchanged review for ceremony.

Keep broad `techdebt` work separate unless requested. Findings outside the task can be reported without expanding the implementation.

## 6. Report and preserve the handoff

Report the resulting behavior, acceptance criteria met, verification evidence, review outcome, and any remaining failures or manual checks. If review was skipped or awaits confirmation, say so. Do not call the work fully verified while required checks remain open.

For a local issue file, tick only criteria supported by evidence. Leave the parent PRD intact. If continuing in another session, record the remaining scope, decisions, and verification state in the existing task artifact when one exists; do not manufacture a new documentation hierarchy.

Leave the change ready for inspection. Commit or publish only when the user has authorized it, and report exactly which actions completed.

## Sources

Inspired by [Matt Pocock's implement skill](https://github.com/mattpocock/skills/tree/main/skills/engineering/implement) and the local Billow `implement` skill. Morgenblau's project instructions and the installed global interview/review skills own their respective details.
