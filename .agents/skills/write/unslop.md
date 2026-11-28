# Unslop Writing

Nobody spots AI writing from a single mistake. They spot it from an accumulation of statistical habits: the same fifty words, the same sentence shapes, the same punctuation crutches, over and over. Each one alone is defensible. Together they read as a watermark.

This skill is a pass over any prose, in two halves. First strip the tells. Then put a voice back in. This half is not optional, because text that's merely clean of tells is sterile, and sterile is its own tell.

## The pass

1. Read the text once for what it's actually trying to say. You can't rewrite a sentence you haven't understood; you'll preserve the slop's shape.
2. Sweep in order: words, then sentences, then punctuation and formatting, then claims. Each layer hides behind the one above it: a puffed-up claim is hard to see until the vocabulary around it is plain.
3. Put a voice back in (see "Sounding like someone").
4. Final read: "Which sentence here would make a reader suspect a model wrote this?" Fix it. Repeat until the answer is none.

## Words

### Trade the model vocabulary for the plain word

These words are wildly overrepresented in model output, and readers have learned to pattern-match on them. It doesn't matter that they're real words a human could use. Statistically, they aren't.

| Slop                           | Write instead                       |
| ------------------------------ | ----------------------------------- |
| delve into                     | dig into, look at                   |
| crucial, pivotal               | important, or name the consequence  |
| leverage, utilize              | use                                 |
| facilitate                     | help                                |
| foster, cultivate              | build, grow                         |
| enhance                        | improve, or the specific change     |
| showcase                       | show                                |
| underscore                     | show, stress                        |
| landscape, tapestry (abstract) | field, mix, or delete the sentence |
| garner                         | get, earn                           |
| additionally                   | also, and                           |
| numerous                       | many                                |
| in the event that              | if                                  |

Same rule for metaphor jargon that dresses a plain idea in a technical coat: _substrate_ (base), _wedge_ (way in), _vector_ (way, method), _north star_ (goal), _flywheel_ (the actual feedback loop, described), _primitive_ as a noun, _harness_ and _scaffolding_ as metaphors, _paradigm_, _nexus_. If the concrete word exists, the abstract one is costing you credibility, not adding depth.

### Say "is" and "has"

"Serves as", "stands as", "boasts", "features", "offers" are all fancy costumes for "is" and "has". "The library boasts full TypeScript support" → "The library has full TypeScript support". The costume adds syllables and a brochure smell, nothing else.

### Delete filler on sight

"In order to" → "to". "Due to the fact that" → "because". "It is important to note that" → delete the whole phrase; if it were important, the sentence after it would show that on its own.

### Cut adverbs, or fix the verb they're propping up

"Runs quickly" → "is fast", or better, the number. "Significantly improves" → the measured delta. An adverb leaning on a weak verb means the verb is wrong; a claim leaning on an intensifier means the claim has no evidence attached.

"Actually", "genuinely", "truly", "really" and "quietly" are the same habit at sentence level, and they pile up: one AI-written agency blog post had "actually" 14 times in 1,850 words. Delete them. When "actually" corrects something the reader believes, name that belief in its own sentence instead.

### Use the verb, not the noun made from it

"The implementation of the import" → "importing". "Provides a reduction in" → "reduces". Models write noun-heavy, packed sentences even when asked to sound casual; instruction-tuned models use about twice as many of these nouns as people do. People write "there is", "it has", "we used".

### Promotional adjectives get neutral replacements

"Vibrant", "stunning", "breathtaking", "groundbreaking", "renowned", "seamless", "robust", "nestled". These are travel-brochure words. Describe the thing; let the reader supply the adjective.

## Sentences

### "Not X, but Y" in any form: state Y

The construction exists to make Y sound bigger by paying an X-shaped toll first. "It's not just a linter, it's a way of thinking about code" → say what it does. If Y can't stand without the runway, Y is the problem.

Banning the one phrasing moves it into others, so watch for all of them: "an operating model, not a tool subscription", "It's not about X. It's about Y.", "Not as a copywriting tool. As a build accelerator.", "AI doesn't change what we ship. It changes the rate." Write Y as a plain claim: "The agents build the layouts in Framer, so a designer ships in a day what took a week." Keep the X only when readers really believe X and the piece exists to correct it, and then say it once.

### Don't stack fragments

"No rebuild phase. No handoff. No scope-creep discovery." Runs of short fragments are what models produce now that em dashes are trained out of them, and readers have learned that rhythm too. Write the sentence that says what the fragments imply: "We design in Framer, so there's no separate build and nothing gets lost in a handoff." One short sentence after longer ones is rhythm. Three in a row is a pattern.

### Don't end a paragraph on a verdict

"That's the line." "This is the bar." "A wrapper is not an operating model." A short closing line that restates the paragraph as a maxim tells the reader what to conclude after they already have. End on the last fact, example or step, and let the paragraph stop there.

### Don't announce or count your structure

"Here's the thing.", "Let's break it down.", "We're going to define the term properly. Then we'll show you…", "the four patterns that don't count", "Three questions cut through it". Announcing and counting make a loose argument look complete. Start with the first point. Give a count only when the reader uses it, as in the steps of a procedure.

### Don't perform candor

"Honestly,", "Let's be real,", "Most of them are lying. Not maliciously, usually." These play a personality instead of having one. Candor is a specific admission, such as what went wrong on a project or what the product doesn't do yet. Write that, or nothing.

### Break the rule of three

Models group ideas into threes compulsively, as in "fast, reliable, and secure", because three sounds complete regardless of whether there are three things. Count the real items. If there are two, write two. If there are five, write five or pick the two that matter.

### Kill false ranges

"From startups to enterprises", "from simple scripts to complex applications": "from X to Y" implies a scale with X and Y as endpoints. If there's no actual scale, it's decoration. List the things, or name the one that matters.

### Repeat the word

Cycling synonyms, such as _the component_, _the element_, _the widget_ and _the control_ all meaning one thing, is a school-essay habit models inherited. Repetition of the right word is not a flaw; it's how the reader tracks what you're talking about. Pick the word and keep it.

### One idea per sentence

If the reader has to backtrack to parse a sentence, split it or drop a clause. Dense subordinate-clause pileups read as generated because generation optimizes for local fluency, not for a reader holding a thread.

### Name the actor

"Queries are validated" → "the compiler validates queries". Passive voice hides who does what, and models default to it because it's always grammatically safe. Passive earns its place only when the actor is unknown or genuinely irrelevant.

### Hedge once, at most

"Could potentially possibly be argued that it might" → "may". One hedge is honesty. Stacked hedges are a model avoiding commitment, and the reader feels the evasion even if they can't name it.

## Punctuation and formatting

### No em dashes

Use periods or commas. Not parentheses, not en dashes, not spaced hyphens. Swapping the em dash for a different interrupter keeps the exact rhythm that reads as generated. If a thought needs separation, end the sentence. But don't chop one sentence into three fragments to avoid the dash: that is the newer tell (see "Don't stack fragments").

### Colons introduce lists and examples, nothing else

The mid-sentence colon as a connector, as in "If you're coming from X: here's the mapping", is a crutch for a comparison the sentence didn't need. Rewrite so the point stands alone.

### Bold is for scanning, not emphasis inflation

Don't bold every proper noun, acronym, or phrase you're proud of. And watch for the inline-header list, the strongest formatting tell there is: a bold label plus colon that restates its own line ("**Performance:** performance improved…"). Convert those to prose. A bold lead-in that ends in a period, names the item, and is followed by genuinely new information is fine. That's structure, not slop.

### Sentence case headings

"Getting started with the API", not "Getting Started With The API". Title case in headings is a default models absorbed from marketing pages.

### Let sections be uneven

Sections of the same length, each with a parallel heading and every list item built the same way, look generated. Give a section as much room as it has things to say, merge a thin one into its neighbor, and write prose where a list would hold only two items.

### No decorative emojis

Not in headings, not as bullet markers. An emoji that carries meaning in chat carries a watermark in prose.

### Straight quotes in source prose

Markdown, commit messages, code comments, and docs source get straight quotes. Curly quotes appearing in raw text are a generation artifact. Rendered UI copy is the opposite territory: there, real typographic characters are correct. The tell is curly quotes where no human would have typed them.

## Claims

### Puffery gets replaced by what happened

"A pivotal moment", "a testament to", "setting the stage for", "left an indelible mark": these assert importance instead of demonstrating it. Cut the frame, state the event, and let the reader judge the size.

### Name the source or cut the claim

"Experts believe", "industry reports suggest", "some critics argue": an attribution that can't be checked is a claim wearing a costume. Name the person or report, or delete it. Same for name-drop lists ("featured in TechCrunch, Wired, and The Verge") with no quote attached: pick one and say what it said.

### Delete the trailing "-ing" clause

"…, highlighting the importance of testing", "…, ensuring reliability", "…, showcasing the team's commitment". These clauses bolt an unsupported conclusion onto a fact. If the point matters, give it its own sentence with its own evidence. Usually it doesn't; delete it.

### Pick the angle only you have

Models pick the most obvious subtopics: a post on website timelines gets the phases, then what speeds a project up, then what slows it down. Ask of each section whether three competitors would write the same one. If they would, lead with what only the writer has: their opinion, a real project, a number, something they got wrong. Content the writer never supplied is the deepest tell, and no sentence-level fix hides it.

### No generic conclusions

"The future looks bright." "Only time will tell." "Exciting times ahead." A conclusion that fits any article fits none. The same goes for the tidy, upbeat wrap-up that restates the piece and lifts the reader at the end. End with a specific plan, a fact, or just end. A format that calls for a summary, such as the short answer and FAQ in a guide, keeps it.

### Remove chatbot residue

"I hope this helps!", "Let me know if you have questions!", "Great question!", "Certainly!": these are conversation artifacts leaking into documents. In prose they address a reader who never spoke.

### Say what it does, not how it feels

"SQL you can read", "types that follow your schema", "the database stays close at hand" describe a feeling. Replace with the mechanism or the number: "`.toSQL()` returns the exact string sent to the database", "a column rename fails the build". Two tests, applied to every sentence of this kind: can you restate it as a concrete instruction, fact, or number? And could it appear unchanged in another project's docs? Fail either, cut it.

## Sounding like someone

Stripping tells produces clean, dead text. Voiceless-but-correct is exactly what a model on its best behavior produces, so this half is what actually separates the writing from slop.

- **Have a reaction.** Facts invite opinions; give one. "Impressive, and a little unsettling" is human in a way "impressive" alone never is. Real reactions are mixed.
- **Vary the rhythm.** Short sentences. Then a longer one that takes its time getting where it's going. Uniform sentence length is one of the most machine-readable features of generated text, and a stack of short ones is as uniform as a stack of long ones.
- **Use "I" when it fits.** First person is not unprofessional; it's evidence a person was here.
- **Be specific over evocative.** Not "this is concerning" but "there's something unsettling about agents churning away at 3am". The specific image can't be produced by averaging over training data, and that's why it works.
- **Let a little mess in.** A tangent, an aside, an admission of uncertainty. Perfect structure looks machine-made because perfection under no time pressure is what machines produce.

## Final audit

Read the result once more and ask the one question: "What here would make a reader think a model wrote this?" You're checking for survivors: a leftover "leverage", a rule-of-three that snuck back in, a paragraph with no opinion in it. Fix, and re-read until the question comes up empty.
