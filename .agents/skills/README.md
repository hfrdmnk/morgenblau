# Repo skills

Each directory here is one skill: a `SKILL.md` with `name` and `description` frontmatter, plus the scripts and references it links. Amp reads them from `.agents/skills/`; Claude Code reads the same files through the `.claude/skills` symlink. Nothing here depends on a personal or hosted skill, so a fresh clone has everything.

| Skill | Invoked | Owns |
|---|---|---|
| [verify-morgenblau](verify-morgenblau/SKILL.md) | by the agent after user-facing changes, or on request | driving an isolated instance, its harness and fixture feeds, and the evidence format |
| [maintain-verification-skill](maintain-verification-skill/SKILL.md) | on request or by an authorized schedule | keeping verify-morgenblau's recipes and harness in step with the app |
| [garden](garden/SKILL.md) | explicitly only: `garden`, `/garden`, or a scheduled run | the static sweep that ships mechanical fixes and files judgment issues |

## Writing a skill

- `name` matches the directory. The `description` says what the skill does and when to use it, since harnesses pick skills from it alone. A skill that writes to shared state (branches, PRs, issues) says in its description that it runs only when explicitly invoked, and repeats that in its body; `disable-model-invocation: true` is honored by Claude Code but not every harness.
- Keep `SKILL.md` under 500 lines; move recipes and long references into linked files.
- Scripts are plain shell or Node with pinned dependencies, run from the repo root, and work the same whether Amp or Claude Code calls them. Harness-specific behavior (such as `amp orb service` supervision in `verify-morgenblau/bin/verify`) is detected at runtime, never assumed.
- Docs point, they don't mirror (root `AGENTS.md`): name the script or file that owns a detail instead of copying it.
- Adapted third-party material keeps its attribution next to the adapted text.

`garden` checks these rules (its step 2.7).
