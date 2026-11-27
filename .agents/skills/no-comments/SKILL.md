---
name: no-comments
description: "Burns down the comments that app/comments-baseline.json and app/frontend/eslint-suppressions.json still allow in one scope of app/, such as a Go package or a frontend directory: deletes them, reshapes code so it says what they said, encodes rules as tests or lint rules, and keeps only notes about outside behavior with an upstream link. Commits locally and never pushes. Use when asked to burn down, clear or work through baselined comments in part of app/."
argument-hint: "<path under app/>..."
---

# No comments

The policy and its checks live in [app/AGENTS.md](../../../app/AGENTS.md#comments): `TestCommentsStayWithinBaseline` in `app/internal/comments` and `comments/no-comments` in `app/frontend/eslint/comments.js`. This skill takes one scope to zero counted comments without changing what the app does. It adapts Lauren Tan's `/no-comments` skill and Comment Sicko agent from pstack (https://github.com/cursor/plugins/tree/main/pstack).

Scope: `$ARGUMENTS`, paths relative to `app/`. Touch files outside the scope only for a root-cause fix that a comment in the scope points to, and name each such file in your report.

## 1. List the comments

Run `.agents/skills/no-comments/scripts/list-comments.sh <path>...` from the repo root. It prints `file:line: text` for every comment the checks count in those paths, ignoring the baselines. A comment group continues on the next lines, so read each one in its file before you judge it.

## 2. Decide each comment

Give every comment exactly one outcome. When you are unsure whether a keep applies, the comment goes.

- **Delete** when it restates a name, the next line, or a test's name, narrates history, or is commented-out code.
- **Reshape** when it explains our own code: rename, extract a function or predicate, name a constant, or derive a value from the one it was explained against. In the pilot, `unsubscribeTimeout = writeTimeout / 3` replaced "well under the server's WriteTimeout", and `isFlyHealthCheckOrACMEChallenge` replaced the comment above that condition.
- **Encode** when it states a rule the next change must keep. Move the reason into an existing test's name or failure message, or write a test or lint rule whose failure message gives it. A rule an existing test already checks only needs its reason moved into that test.
- **Fix the root cause** when the comment justifies a workaround or an ordering ("must come after", "keep this empty", "do not remove"). Write a failing test first (Red-Green TDD per `app/AGENTS.md`), fix the code so the constraint disappears, then delete the comment. In the pilot, "WithLocalFeeds must come after them" became `fetcher.New` applying options in any order, with `TestLocalFeedsSurviveARebuiltSafeClient`.
- **Keep** only a note about behavior of a dependency, platform, protocol or vendor we cannot change, and give the group a link to the upstream docs, issue or RFC that states it. Open the page and confirm it says what the note claims before you link it; never link a page you have not read. Rewrite the note to the outside fact only. Links to Morgenblau's own domains and repos do not count.

A reason that fits none of these, such as a trade-off, goes in the commit message of the commit that deletes it, under "Reasons not encoded anywhere".

## 3. Keep behavior

Delete, reshape and encode commits change no behavior: the existing tests pass unchanged except for names and failure messages you moved reasons into. A root-cause fix changes behavior on purpose, so it gets its own commit with its failing test, ahead of the deletions that depend on it. Never edit migrations, lexicons or generated sqlc code.

## 4. Prune and check

From `app/`, run `make comments-prune`, then the checks for what you touched:

- Go: `gofmt -l .` prints nothing, then `go vet ./...`, `go test ./...` and `golangci-lint run ./...` pass.
- Frontend, from `app/frontend/`: `bun run test`, `bun run lint`, `bun fmt --check` and `bun check` pass.

When a merge conflicts in either baseline file, take the other side's version and rerun `make comments-prune`; prune only lowers counts, so the result is right either way.

## 5. Report

Report the comments removed per outcome, each root-cause fix with its test, each kept note with its link, the reasons left unencoded, and any file outside the scope you changed.
