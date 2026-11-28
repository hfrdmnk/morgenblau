# Morgenblau voice

How Dominik wants Morgenblau's text to sound, learned from his notes and his own versions of sentences. The `write` skill reads this before every piece; only `/write tune` changes it.

Rules marked (Billow) come from Dominik's notes on copy for Billow, his other product, and are kept because they are about taste, not about Billow. A Morgenblau note that contradicts one replaces it.

## Rules

Each rule carries its why, so it can be applied to cases it doesn't name. `unslop.md` holds the general tells; `BRAND.md` › Language and the `SPEC.md` glossary own the vocabulary and the tone limits. The rules here are the ones Dominik called out.

- Match formality to the channel. From most to least formal: product copy (in-app text in `app/frontend`); marketing copy (morgen.blue, its FAQ and meta description, announcements); personal writing (the "Why I'm creating" section of morgen.blue, newsletter devlogs, blog posts, Bluesky posts). `unslop.md` › Sounding like someone applies by level:
    - Product copy: varied rhythm only. No reactions or asides, because the reader is in the middle of a task. (Billow)
    - Marketing copy: a reaction, an aside, a light nudge and varied rhythm are welcome.
    - Personal writing: follows `samples.md` closely.
    - Informal never brings the tells back: every other rule here holds on every level.
- The reader is "you". Morgenblau is the subject for what the product does by itself: "Morgenblau gathers what they publish into one web newspaper per day." Dominik is "I", as in "Morgenblau is in an invite-only alpha while I finish the essentials." There is no team, so "we" never means the makers. It can mean people in general, the reader included: "We get sucked into rabbit holes." Why: the morgen.blue copy he wrote.
- The subject is whoever does the action. When the reader does it, write "you": "You can now save a post to your library", not "Morgenblau now lets you save a post". When Morgenblau does it on its own, write "Morgenblau". Why: "lets you is not something that [the product] does, but you do [it]" (Billow).
- Use contractions: "doesn't", never "does not". Why: he wants informal wording (Billow), and his own copy has no uncontracted form.
- Never list three things in a row as examples. Name one thing, or two when both matter. "Add blogs, newsletters, YouTube channels and more" on morgen.blue is the pattern to fix. The one exception is a sentence that states a complete set the app has, such as every kind of source it supports: name all of them, because leaving one out is a wrong fact. Why: "This repeating three examples is very clear indication of LLM speak" (Billow).
- Never open with what other tools do to set up what Morgenblau does. Say what Morgenblau does directly. Why: "Most tools do it this way, we do it this way. We shouldn't have a pattern like this" (Billow).
- End the sentence when its point is made. Delete whatever follows the main clause, such as ", all at a glance", and reread it. If the point still stands, leave it deleted. If the extra fact matters, give it its own sentence: "You can even filter out Shorts." Why: the tail reads "so ambiguous and a bit LLM-speaky", and he cut it every time (Billow).
- Describe things in the app literally. Entries, sources and saves are objects in an app: they "show up" or are "in" a place, they never "land", "sit" or "live" somewhere. Write "Your feed subscriptions and saves are records in your own ATProto account", not "live in your own ATProto account" as morgen.blue says. Why: "this land word is a bit … pretentious" and "this sit right is a bit weird because they are objects in a UI" (Billow).
- Never put "right" in front of a place: "in your library", not "right in your library". Why: "Drop the right" (Billow). It's an intensifier with nothing behind it.
- Never echo a word's root within one sentence, as in "save … what you saved". Name the thing instead. Repeating a noun across sentences to keep track of it is still fine. Why: "The repetition of signed … is a bit weird" (Billow).
- Use "when" for a condition or a moment, never "once": "When you reach the end of the list, you're done." Why: "once reads off, use when" (Billow), and his own sentence above.
- Write in the present tense. Use the past only for what happened before. Why: "Generally we should write in the present" (Billow).
- Write full sentences with a subject, never a verbless fragment like "No unread counts." Write "Morgenblau doesn't count what you haven't read." This holds on every channel, including the fragments in `samples.md`, such as "In one feed." Why: "has no subject" and "worse than before" (Billow).
- Claim the effect a change has directly, not one further down the line. Why: he called a far-off effect "weird in this context" and rewrote it to the direct one (Billow).
- In body copy, name one concrete thing the reader sees in Morgenblau: "When you reach the end of the list, you're done", not "Morgenblau gives you back your time". A headline or a short social post can lead with the general benefit. Why: "on a short social post or marketing headline B, but generally, I lean towards A" (Billow).
- Join two parts into one sentence when the second follows directly from the first, such as an action and its result: "Paste the address of a website and Morgenblau works out the rest." Split them when the second is a separate fact the reader should notice. Why: he confirmed the principle after a round of paired sentences (Billow).
- A light nudge goes only in marketing copy, as its own short line that invites the reader to try something without risk. Never attach it to a task or tack it onto the end of a sentence. Why: the nudge won on a demo and lost everywhere else (Billow).
- Hedge an outcome the reader feels or that depends on someone else with "should", and keep its size: "Digests should load much faster now." Never hedge a fact about what Morgenblau does. Why: he preferred "should" twice and called "a bit easier" "too small" (Billow).
- In-app copy takes whether a line exists at all from `interface-guide` › Hints, descriptions and tooltips. A hint keeps only what its label doesn't say. Why: he cut every dialog description that repeated its title or gave an obvious reason, and kept the ones that clarify a value, give a format or say where a value shows up (Billow).
- On morgen.blue and in announcements, never name a part of the interface the reader hasn't met. Say where it happens in their terms. Why: an internal name "is more of an internal term" (Billow).
- UI labels appear verbatim, as the app writes them, so readers recognize what they see on screen.
- No em dashes. Use a period, or a comma when the second part is short, and keep the sentence whole. An en dash only for ranges, as in "9–17". Why: his rule everywhere, em dashes read as AI writing.
- No filler openers, flattery or superlatives. Claims stay modest and describe what the reader can do.
- Rendered copy uses curly apostrophes and quotes, also in source files such as `site/src/pages/index.astro` and the strings in `app/frontend/src`, as the existing copy does. US spelling: "favorite", "organize".

## Examples

Sentences Dominik approved, and versions he wrote himself, by channel. At most 5 per channel. Personal writing has none: `samples.md` covers it.

Marketing:

- "When you reach the end of the list, you’re done." (own version, morgen.blue)
- "You get a private email address. Use it to sign up to your favorite newsletters." (own version, morgen.blue)
- "Don’t miss a video from the channels you follow." (own version, morgen.blue)
- "You can even filter out Shorts." (own version, morgen.blue)
- "Morgenblau gathers what they publish into one web newspaper per day." (own version, morgen.blue FAQ)

Product copy: none yet.

Announcements: none yet.
