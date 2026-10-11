// Package webfeed derives a feed from a site's listing page when the site publishes none.
//
// Terms used across this package, internal/websource and internal/claude:
//
//   - Setup: the one-time pass that fetches the listing, clusters its links into candidate lists, checks a few
//     articles and builds the recipe (Setup, SetupAnchored).
//   - Recipe: the stored result of setup, keyed on each stream's URL template rather than CSS classes, so polls
//     need no model (Recipe).
//   - Poll: the routine re-read of the listing through the recipe; it never calls a model on its own (Poll).
//   - Detector: the free rules that grade a poll ok, suspect or broken against recent good polls (detect.go).
//   - Repair: the ladder a suspect poll climbs, free steps first and the model at most once per site per day
//     (Repair).
//   - Judge: Claude Haiku picking among the candidate lists when the heuristic is unsure; it only chooses lists
//     the code found (judge.go).
//   - Induction: Haiku writing CSS selectors when fewer than three posts survive setup (induce.go).
//   - Weekly check: one Haiku call per source per week asking whether the served posts still match the page; it
//     can raise an alarm, never close one (WeeklyCheck).
//   - Anchor: a post URL the reader points at so setup rebuilds around lists holding it or sharing its shape.
//   - Blocked: the site refuses honest bots (401/403/429, a challenge page, or robots.txt); Morgenblau does not
//     work around it.
package webfeed
