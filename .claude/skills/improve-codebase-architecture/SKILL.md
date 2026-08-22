---
name: improve-codebase-architecture
description: Scan a codebase for deepening opportunities, present them as a visual HTML report, then interview you through whichever one you pick.
disable-model-invocation: true
---

# Improve Codebase Architecture

Surface architectural friction and propose **deepening opportunities**: refactors that turn shallow modules into deep ones. The aim is testability and AI-navigability.

This command is _informed_ by the project's domain model and built on a shared design vocabulary:

- Call the Skill tool with "codebase-design" for the architecture vocabulary (**module**, **interface**, **depth**, **seam**, **adapter**, **leverage**, **locality**) and its principles (the deletion test, "the interface is the test surface", "one adapter = hypothetical seam, two = real"). Use these terms exactly in every suggestion, and don't drift into "component," "service," "API," or "boundary."
- The domain language in `SPEC.md` gives names to good seams, and the decisions recorded there are ones this command should not re-litigate. Read the section tags present in the file rather than assuming a fixed taxonomy; the routing rules live in the `interview-with-spec` skill.

## Process

### 1. Explore

**Scope before you scan: YAGNI.** Deepening a module pays off by making future changes to it easier, so put extra weight on the parts of the codebase that have recently changed. Decide *where* to look before you look:

- If the user named a direction (a module, a subsystem, a pain point), take it, and skip the inference below.
- Otherwise, walk back a good stretch of the commit history (`git log --oneline`) to find the codebase's hot spots, the files and areas that keep coming up, and let those paths pull your attention first. If the changes are scattered with no clear hot spot, widen the net.

Read the project's `SPEC.md` first, in full. It is the source of truth for domain language, business rules, and guardrails.

Then spawn a sub-agent to walk the codebase. Don't follow rigid heuristics; explore organically and note where you experience friction:

- Where does understanding one concept require bouncing between many small modules?
- Where are modules **shallow**, with an interface nearly as complex as the implementation?
- Where have pure functions been extracted just for testability, but the real bugs hide in how they're called (no **locality**)?
- Where do tightly-coupled modules leak across their seams?
- Which parts of the codebase are untested, or hard to test through their current interface?

Apply the **deletion test** to anything you suspect is shallow: would deleting it concentrate complexity, or just move it? A "yes, concentrates" is the signal you want.

### 2. Present candidates as an HTML report

Write a self-contained HTML file to the OS temp directory so nothing lands in the repo. Resolve the temp dir from `$TMPDIR`, falling back to `/tmp` (or `%TEMP%` on Windows), and write to `<tmpdir>/architecture-review-<timestamp>.html` so each run gets a fresh file. Open it for the user (`xdg-open <path>` on Linux, `open <path>` on macOS, `start <path>` on Windows) and tell them the absolute path.

The report uses **Tailwind via CDN** for layout and styling, and **Mermaid via CDN** for diagrams where a graph/flow/sequence reliably communicates the structure. Mix Mermaid with hand-crafted CSS/SVG visuals: use Mermaid when relationships are graph-shaped (call graphs, dependencies, sequences), and hand-built divs/SVG when you want something more editorial (mass diagrams, cross-sections, collapse animations). Each candidate gets a **before/after visualisation**. Be visual.

For each candidate, render a card with:

- **Files**: which files/modules are involved
- **Problem**: why the current architecture is causing friction
- **Solution**: plain English description of what would change
- **Benefits**: explained in terms of locality and leverage, and how tests would improve
- **Before / After diagram**: side-by-side, custom-drawn, illustrating the shallowness and the deepening
- **Recommendation strength**: one of `Strong`, `Worth exploring`, `Speculative`, rendered as a badge

End the report with a **Top recommendation** section: which candidate you'd tackle first and why.

**Use SPEC.md vocabulary for the domain, and the `codebase-design` vocabulary for the architecture.** `<terminology>` assigns each near-synonym a home, so respect it: "the Subscription intake module" if the seam sits at the PDS record, "the Feed polling module" if it sits at the RSS mechanics. Never "the FooBarHandler," and never "the Subscription service."

**Spec conflicts**: if a candidate contradicts a rule documented in `SPEC.md`, only surface it when the friction is real enough to warrant reopening that decision. Mark it clearly in the card (e.g. a warning callout: _"contradicts the `<anti-features>` Hard No on unread counts, but worth reopening because…"_). Don't list every theoretical refactor the spec forbids.

See [HTML-REPORT.md](HTML-REPORT.md) for the full HTML scaffold, diagram patterns, and styling guidance.

Do NOT propose interfaces yet. After the file is written, ask the user: "Which of these would you like to explore?"

### 3. Interview loop

Once the user picks a candidate, call the Skill tool with "interview-with-spec" to walk the decision tree with them: constraints, dependencies, the shape of the deepened module, what sits behind the seam, what tests survive. That skill runs the interview under `interview-me` rules (design tree, frontier rounds, question format) and layers the spec awareness on top, so don't re-derive either here.

Two things it does that matter for this loop:

- **Spec updates go through its high-bar gate.** A resolved decision only reaches `SPEC.md` when it is hard to reverse, invisible from the code, and the result of a real trade-off. Naming a deepened module after a fresh concept is usually not enough on its own; a seam that encodes an invariant a reader could not recover from the implementation usually is.
- **A load-bearing rejection is a spec candidate.** If the user rejects the candidate for a reason a future architecture review would need in order to not re-suggest it, offer: _"Want me to record this in `SPEC.md` so future reviews don't re-propose it?"_ Let `interview-with-spec` pick the section. Skip ephemeral reasons ("not worth it right now") and self-evident ones.

**Want to explore alternative interfaces for the deepened module?** Call the Skill tool with "codebase-design" and use its design-it-twice parallel sub-agent pattern.
