---
name: garden
description: Static gardening sweep of the repo. Runs the Go checks (gofmt, vet, golangci-lint, race suite, go mod tidy, sqlc freshness), the frontend test, lint and build scripts, and fallow's whole-tree dead-code, health and dupes views; checks that every test LAWS.md names still exists; and checks AGENTS.md files, SPEC.md, LAWS.md and the repo skills for stale references, rules no check enforces, skill drift and skill size. Ships mechanical fixes as one PR on garden/<date> and files judgment calls as GitHub issues labelled gardening. Use when a scheduled gardening run fires, or when the user says "garden" or "/garden". "/garden dry-run" sweeps without writing anything.
argument-hint: "[dry-run]"
disable-model-invocation: true
---

# Garden

Runs unattended. Never ask a question and never wait for input: when a step cannot run, finish what is safe, then end with outcome `blocked` and the reason.

A run produces at most one PR of mechanical fixes, plus GitHub issues for everything that needs a human, and always ends with the report block from step 6.

Argument passed: `$ARGUMENTS`

## Dry run

With `dry-run`, run step 1 without creating the branch and every sweep command in step 2, but apply no fix, create no label or issue, push nothing and open no PR. Instead list each mechanical fix the run would ship and each issue it would file or update (key, title, new or updated). Undo anything a sweep command wrote, so `git status --porcelain` matches the preflight, and end with the report block, `Outcome: clean` or `blocked`, `PR: none (dry run)`.

## Classification

- **Mechanical**: a tool produced or confirmed the fix, it preserves behaviour, and the checks in step 4 stay green. It goes into the PR.
- **Judgment**: everything else, including every change to rule or law wording, deleting prose, regenerating stale sqlc output, adding or dropping a dependency and refactoring complexity. It becomes an issue.

When in doubt, it is judgment. A mechanical fix that turns any check red is reverted and filed as judgment.

## Guardrails

- No behaviour changes.
- Never add or reword rules or prose in any AGENTS.md, SPEC.md or skill. The only doc edit allowed is correcting a stale reference (path, make target, bun script, Go symbol, route) whose replacement is unambiguous. Everything else goes to an issue.
- Never suppress a check: no `fallow-ignore` comments or config ignores, no `eslint-disable`, no `//nolint`, no `t.Skip`, no new entries in an invariant test's allowlist (such as `unscopedNewsletterQueries`), no lowered fallow thresholds.
- Never touch `internal/database/migrations/` (goose owns them), `internal/database/db/` (sqlc-generated), `lexicons/`, `LAWS.md` (law changes need the maintainer), `.agents/skills/verify-morgenblau/features/` (the verify skill maintains its own feature map) or the shadcn components in `frontend/src/components/ui/`.
- Never run `make migrate-*`, `make dev` or anything that touches `./data/` or a PDS.
- Never change the state or labels of an existing issue.

Every repository-scoped `gh` call targets `--repo hfrdmnk/morgenblau` and runs as a collaborator. In Amp orbs, use the authenticated `gh` directly; a stored token lookup may fail even when repository access works. Elsewhere, prefix it with `GH_TOKEN="$(gh auth token -u hfrdmnk)"` where that account is logged in. Before publishing, confirm the identity with `gh api user --jq .login` and write access with `gh api repos/hfrdmnk/morgenblau --jq .permissions.push`; unavailable access is `blocked`, not a reason to request or print a token. The shell may be zsh, so check exit codes with `$?` right after each command, never `PIPESTATUS`.

## 1. Preflight

```bash
git fetch origin
git switch main && git merge --ff-only origin/main
git status --porcelain          # must print nothing
gh pr list --repo hfrdmnk/morgenblau --state open --json headRefName,url --jq '.[] | select(.headRefName | startswith("garden/"))'
bun install --cwd ./frontend --frozen-lockfile
```

Outcome `blocked` when the tree is dirty, `main` cannot fast-forward, a `garden/*` PR is still open (name its URL: a second PR would overlap it), or the install fails. Without `gh`, skip the open-PR check and say so in the report.

Invoking this skill authorises creating one branch, `garden/<date>`. Create no other branch. Write tool output to `.scratch/garden/<date>/` (gitignored); `$G` below stands for that path, so set it in every new shell.

```bash
git switch -c "garden/$(date +%F)"
mkdir -p ".scratch/garden/$(date +%F)"
```

## 2. Sweep

Record every finding as you go: kind, `file:line`, evidence, mechanical or judgment. A red check on `main` is a judgment finding unless this section names its mechanical fix.

### 2.1 Go

The Go packages embed `frontend/dist`; when it is missing, run 2.2's `build` first.

```bash
gofmt -l .
go vet ./...
go test -race ./... > "$G/go-test-race.txt" 2>&1
go mod tidy -diff
sqlc diff
golangci-lint run ./... > "$G/golangci-lint.txt" 2>&1
```

- `gofmt -l` output is mechanical: `gofmt -w` the listed files.
- `go mod tidy -diff` output is mechanical when it touches only `go.sum` or `// indirect` markers: apply `go mod tidy`. An added or dropped direct `require` is judgment.
- `sqlc diff` prints a diff and exits 1 when `internal/database/db/` is stale. That is judgment: the committed Go embeds the SQL that runs, so regenerating changes behaviour. A diff limited to the `// sqlc vX` header means the local CLI differs from the one that generated the code; report it, file nothing.
- A red `go vet`, golangci-lint or race suite is judgment. The race suite runs every law check, so name the failing tests in the issue. CI pins the golangci-lint version in `.github/workflows/ci.yml`; a finding that only a different local version reports goes in the report, not an issue.

### 2.2 Frontend

`frontend/package.json` owns the scripts; run the ones CI runs:

```bash
bun run --cwd ./frontend test
bun run --cwd ./frontend lint
bun run --cwd ./frontend build
```

A red script on `main` is judgment. `build` writes only the gitignored `frontend/dist/`.

### 2.3 fallow

The pre-commit hook and CI run `fallow audit`, which gates only findings a changeset introduces. The whole-tree sweep uses the per-analysis commands from `frontend/` with `frontend/.fallowrc.jsonc`:

```bash
cd frontend
./node_modules/.bin/fallow dead-code --format json --quiet > "../$G/fallow-dead-code.json"   # exits 1 when it finds anything
./node_modules/.bin/fallow health --format json --quiet > "../$G/fallow-health.json"
./node_modules/.bin/fallow dupes --format json --quiet > "../$G/fallow-dupes.json"
```

Dead code (`.summary` holds the counts per kind):

- **Unused exports, types and enum members** are mechanical once `fallow dead-code --trace <file>:<export>` shows no reference. Preview with `fallow fix --dry-run --no-create-config`. When the preview edits only source files, apply it with `fallow fix --yes --no-create-config`; when it also touches `package.json` or config, make the source edits by hand instead.
- **Unused files** are mechanical when `git grep -n <basename without extension>` finds no reference outside the file itself (`components.json` aliases, `index.html`, `vite.config.ts` and dynamic imports included). Remove them with `git rm`.
- **Stale suppressions** are mechanical: delete only the suppression comment fallow names, then rerun dead-code to confirm nothing new surfaced.
- Unused class members, dependencies, unresolved or unlisted imports, cycles and boundary violations are judgment.

Before filing a dead-code finding, look for its uses (`git grep -nw <name> -- frontend/src`). A finding the code disproves is a fallow false positive: name it in the report and file nothing.

Health (`.findings`) and dupes (`.clone_groups`) are judgment: one issue per function over a threshold, keyed `fallow-health:<path>:<function>`, and one per file that holds a clone group's first instance, keyed `fallow-dupes:<path>`, with the numbers and fragments as evidence.

### 2.4 Laws

`LAWS.md` owns the laws and names their checks under each "**Enforced by:**" list. Every test it names must still exist; a `*` in a name matches any run of identifier characters:

```bash
grep -oE '`Test[A-Za-z0-9_*]+`' LAWS.md | tr -d '`' | sort -u | while read -r t; do
  git grep -qE "^func ${t//\*/[A-Za-z0-9_]*}\(" -- '*_test.go' || echo "missing: $t"
done
grep -n 'Check by hand' LAWS.md
```

Every law heading carries an "Enforced by:" list. A "Check by hand" line is allowed only with a one-line reason why no check can express it. A missing test, a law without checks or a reasonless "Check by hand" is judgment, even when the rename is obvious, since `LAWS.md` is never edited here. Linked test files are covered by 2.5.

### 2.5 Docs and skills vs code

Scope: every AGENTS.md (the `.claude/rules/*.md` symlinks and `CLAUDE.md` point at them), `SPEC.md`, `LAWS.md`, and every `SKILL.md` under `.agents/skills/` with the files it links. For the verify skill's `features/`, check only that references resolve; its content is the verify skill's job. Resolve every reference:

- repo paths exist (`test -e`): markdown links relative to the doc, backticked paths relative to the doc's directory or else the repo root; `#anchors` and prose section references ("SPEC.md, Newsletters") match a heading in the target
- `make <target>` is a target in `Makefile`
- `bun run <script>` is in `frontend/package.json`, or is a Bun built-in
- Go symbols cited by name have a definition (`git grep -nwE 'func (\([^)]*\) )?<Name>|type <Name>' -- '*.go'`)
- routes written as `METHOD /path` are registered in `internal/server/routes.go`
- `bin/verify` subcommands and flags exist in `.agents/skills/verify-morgenblau/bin/verify`

A stale reference is mechanical when exactly one replacement exists: a rename in `git log --follow --diff-filter=R --name-status -- <old path>`, or a single match for the renamed symbol, target or script. Otherwise it is judgment. A stale reference in `LAWS.md` or `features/` is always judgment.

### 2.6 Prose without a check

Judgment only; never edit. For each rule in the AGENTS.md files, look for the check that enforces it: Go tests (the source-scan and invariant tests next to the code they guard), `.golangci.yml`, `frontend/eslint.config.js`, `frontend/.fallowrc.jsonc`, `.github/workflows/ci.yml`, `.githooks/pre-commit` and constraints in the migrations. File an issue when:

- a test or lint rule could express a rule that has none
- a checked rule still carries prose beyond a pointer, or prose restates what the code already owns
- an unchecked rule runs longer than 3 lines
- a rule cites files, functions or routes as examples instead of stating the principle (naming the canonical artifact to use is fine)
- a check's failure message says what is wrong without saying why

Skip rules no check could express, such as how to write comments.

Group these findings: one issue per doc file keyed `prose:<doc path>`, with each finding as a checklist item, and one issue keyed `prose:failure-messages` for every failure message that lacks its why. A rule the code already breaks gets its own issue as well, keyed `rule-violation:<violating path>:<rule slug>`, with the violating lines as evidence.

### 2.7 Skills

For each `.agents/skills/*/SKILL.md`:

- frontmatter `name` matches the directory
- every capability and trigger the `description` promises has a matching section in the body, and the body does nothing the description hides
- over 500 lines is a finding
- `.claude/skills` is still a symlink to `.agents/skills`

All of these are judgment.

## 3. File judgment findings

Keys are `<kind>:<path>[:<symbol or rule slug>]` and never contain line numbers. Ensure the label exists, then load every issue once:

```bash
gh label list --repo hfrdmnk/morgenblau --search gardening --json name --jq '.[] | select(.name == "gardening") | .name'
gh label create gardening --repo hfrdmnk/morgenblau --color 0e8a16 --description "Found by the garden sweep"   # only when missing
gh issue list --repo hfrdmnk/morgenblau --state all --limit 1000 --json number,state,stateReason,body > "$G/issues.json"
jq --arg k "garden-key: <key>" '.[] | select(.body | split("\n") | map(rtrimstr("\r")) | index($k)) | {number, state, stateReason}' "$G/issues.json"
```

Match on the whole `garden-key:` line across open and closed issues, with or without the label:

- An open issue with the key: comment with `gh issue comment` only when the evidence changed.
- A closed issue with reason `NOT_PLANNED` or `DUPLICATE`: the maintainer declined it; skip it.
- Only issues closed as completed: the finding came back. Create a new issue that names the old number.
- No match: create one issue per finding with `gh issue create --label gardening --body-file <file>`:

```markdown
### What
One or two sentences.

### Evidence
- `path:line`: the quote or tool output

### Proposed check
Where the check lives and its failure message (what to do and why), or "none" with the reason no check can express it.

garden-key: <key>
```

When GitHub is unreachable, list the unfiled findings in the PR body and end `blocked`.

## 4. Verify

With every mechanical fix in place:

```bash
gofmt -l .
go vet ./...
go test -race ./...                       # these two when a Go file, go.mod or go.sum changed
golangci-lint run ./...
bun run --cwd ./frontend test             # these three when anything under frontend/ changed
bun run --cwd ./frontend lint
bun run --cwd ./frontend build
cd frontend && ./node_modules/.bin/fallow dead-code --quiet   # when fallow-driven fixes landed
git diff --check
```

Then read the whole diff: every hunk must trace to a finding from step 2.

When a check fails, stash the fixes and rerun it. Still red means it was red on `main`: file it and continue. Green means a fix broke it: revert that fix, file it as judgment, and rerun.

## 5. Ship

With no mechanical changes left, switch back to `main`, delete the local branch and open no PR.

Otherwise stage only the files the fixes touched and commit `chore: gardening <date>`. The pre-commit hook runs `fallow audit`; never pass `--no-verify`. If the hook blocks, drop the offending fix and file it.

```bash
git push -u origin "garden/$(date +%F)"
gh pr create --repo hfrdmnk/morgenblau --base main --head "garden/$(date +%F)" --title "chore: gardening $(date +%F)" --body-file "$G/pr.md"
git switch main
```

The PR body:

```markdown
## Summary of Changes
- <fix>: <files>, <tool that produced or confirmed it>

## Filed for judgment
- #<num>: <title> (new | updated)

## Verification
- `<command>`: <result>
```

The PR closes nothing: the issues it lists are future work. Without a working `gh`, push the branch and end `blocked`.

## 6. Report

End every run with this block and nothing after it:

```
Outcome: clean | changed | blocked
Reason: <one line>
PR: <url | none>
Issues: new <#nums> | updated <#nums> | none
Go: gofmt <n files> vet <ok|red> lint <ok|red> race <ok|red> tidy <ok|drift> sqlc <ok|stale>
Frontend: test <ok|red> lint <ok|red> build <ok|red>
Fallow: dead-code <n> health <n> dupes <n> false positives <n>
Laws: <n> tests named, <n> missing
```

- `clean`: no PR, and no issue created or updated.
- `changed`: a PR opened or at least one issue was created or updated.
- `blocked`: a step could not run. Name the step, and list what finished before it.
