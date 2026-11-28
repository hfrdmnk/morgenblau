---
name: write
description: "Drafts Morgenblau's user-facing text in Dominik's voice: morgen.blue copy in site/, in-app copy in app/frontend, FAQ answers, meta descriptions, announcements, newsletter devlogs and blog posts about Morgenblau. In-app copy comes out ready to ship; marketing and long-form text is a first draft that Dominik rewrites. Use when asked to write, draft, rewrite, de-slop or polish any of these. \"/write tune\" is the only mode that changes the voice profile: it turns Dominik's notes, his rewrites of a draft or an earlier write session into rules and examples in voice.md."
argument-hint: "[what to write] | tune [notes, his version, or a thread]"
---

# Write

Morgenblau's text should sound like Dominik. Read these in full before the first sentence of a session:

- `unslop.md` is the pass that strips the tells of AI writing and puts a voice back in.
- `voice.md` is Dominik's profile: rules he stated, each with its why, and sentences he wrote or approved. Where it disagrees with `unslop.md`, `voice.md` wins. Two known cases: rendered copy uses curly quotes and apostrophes, as the `unslop.md` section on quotes allows, and `unslop.md` › Sounding like someone applies only as far as the channel's formality allows, which `voice.md` sets.
- `samples.md` is Dominik's own writing, verbatim. Read it for marketing and personal writing; product copy carries no personal voice and skips it. Take rhythm, word choice, asides and how a paragraph ends from it, never content. Where a sample does something `unslop.md` warns against, it belongs to his voice at the rate the samples show it: one parenthesis in a paragraph is his voice, five are not. `voice.md` still wins over the samples. They keep his typos; never copy a spelling or grammar mistake. Why: given real samples, models match a person's style far more often than given a description of it, because the style lives in small words and sentence shapes that no rule names.
- `BRAND.md` › Language and the glossary in `SPEC.md` own the product's words and tone limits. When `voice.md` contradicts them, follow them and tell Dominik, so one of the two gets changed.

## Two kinds of text

- **Product copy** is in-app text in `app/frontend/src`: labels, hints, dialogs, empty and error states, sign-in. Write it ready to ship, in place, under the rules in `app/frontend/AGENTS.md`. Its facts come from the code, so check every label, limit and behavior in `app/` before you write about it.
- **Marketing and long-form text** is morgen.blue in `site/`, its FAQ and meta description, announcements, newsletter devlogs, blog posts and social posts. Dominik writes the final version; this skill writes the first draft, good enough that he edits it instead of starting over. Show it in chat, and put it into `site/` or anywhere else only when he asks.

## Check every sentence

Walk every sentence through these steps, in order, before Dominik sees it:

1. Run the `unslop.md` pass on it: words, then sentences, then punctuation, then claims.
2. Check it against every rule in `voice.md`. Rewrite it until it breaks none.
3. Ask: "What here would make a reader think a model wrote this?" For marketing and personal writing, also ask whether the sentence would stand out among his samples. If anything, rewrite and start again at step 1.

Why: his attention is for taste. In the first calibration of these rules, on Billow copy, 7 of 11 notes were about tells a checklist catches: a list of three, a "most tools do X" opening, two tacked-on tails, a dressed-up verb, "we" and a missing contraction.

## Write a piece

1. For marketing and long-form text, get the substance from Dominik before anything else. Ask him to talk through the piece at length, dictated or typed, in any order: what he thinks and why, a real case from his own use, the numbers, what others say that he disagrees with. Then ask for whatever the piece still lacks. Why: a model with nothing specific to say falls back on the most typical angle, claim and example, and that reads as AI writing however clean each sentence is. Product copy takes its facts from the code and skips this step.
2. Plan it together in chat. Settle who reads it, the one thing they should take away, where it will be published and how long it is. Then agree on an outline with one line per paragraph that states its point. Each point comes from what he said in step 1; a paragraph nothing he said supports becomes a question to him, never the obvious point filled in. Check every fact the outline needs against `app/` and `SPEC.md`. Get his go before you write. A handful of product strings skips the outline.
3. Write it. Start from his own words in step 1 where they work, and change only what a rule in `voice.md` or `unslop.md` requires.
4. Show it whole in chat and work his notes into the text. Leave `voice.md` and `samples.md` alone, and don't ask whether a note should become a rule: keeping notes is what tune is for. Why: Dominik doesn't want to tune the profile every time he starts a write session.
5. Deliver it as "Two kinds of text" says.

## Tune

Runs only when Dominik invokes `/write tune` or explicitly asks to update the voice profile. It works from any of these:

- Notes he gives now.
- His final version next to the draft it replaced, such as a published post, a commit that changed `site/` copy, or pasted text. Compare them sentence by sentence; each change he made is a rewrite in the tree below. This is the richest source, because he rewrites every draft anyway.
- An earlier write session, such as an Amp thread link: read the notes he gave there.
- Nothing yet: write 4–6 pairs of sentences for the channel he names, each pair differing in one choice no rule settles, and ask which he picks and why.

Walk this tree for each note:

```text
├── States a preference outright ("use doesn't", "don't use we")
│   → add it to voice.md › Rules, with his words as the why.
├── Names a pattern as wrong ("LLM speak", "pretentious")
│   → add a rule for that pattern, with his words as the why and the draft's sentence as the example.
└── Rewrites a sentence
    → add his version to Examples, and add a rule for what he changed
      when the change is a pattern and no rule covers it yet.
```

Keep at most 5 examples per channel and replace older ones. When a note contradicts a rule, ask which one holds before you change `voice.md`; a Morgenblau note replaces a rule marked (Billow) without asking. When he publishes a new post or page, offer to add it to `samples.md` verbatim, and drop the oldest post to keep the file under about 400 lines. At the end, show him what changed in `voice.md` and `samples.md` and offer to commit it.
