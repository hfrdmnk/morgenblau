# Morgenblau Spec

> Product scope, data ownership, and architectural guardrails for the initial deployable version.
> Brand feeling and art direction live in [BRAND.md](./BRAND.md). Schemas and implementation details live in the files referenced below.

---

<vision>

## A Personal Daily Newspaper

Morgenblau collects sources the reader chooses into finite daily digests. The initial release should replace NetNewsWire for everyday reading, without unread counts or pressure to catch up. It remains a deployed application for multiple users, each with their own sources and Library.

ATProto provides identity and ownership of supported reading records. Social features are outside this release.

</vision>

---

<v1-scope>

## Release Boundary

V1 includes daily digests, an in-app reader, RSS/Atom and native Standardfeed subscriptions, inbound email newsletters, OPML import/export, and Library saves with tags. Blog posts, YouTube videos, newsletters, and existing microblog handling are supported. Source tags, save commentary, and custom Views remain in scope.

The app is responsive and installable as a PWA. Offline reading and offline mutation sync are outside v1.

Sharing, discovery recommendations, following users, public reader profiles, network Library sections, and article social context are excluded. Jetstream and network-wide ingestion are unnecessary for this scope. Resolving a pasted URL into a subscribable source remains part of adding sources.

**Current simplification pass:** remove excluded features and their supporting code, simplify the specification and local schema, and preserve the retained reading flows. This pass does not complete missing v1 features such as email ingestion, OPML, Library tag editing, primary-source presentation, or PWA installation.

</v1-scope>

---

<terminology>

## Glossary

Terms that are easy to conflate. Each line states the distinction; the linked sections and code own the detail.

| Term | Meaning |
|:--|:--|
| Source | What the reader chose to follow, in product copy and routes. A source is followed through a subscription and backed by a feed or a newsletter. |
| Feed | The technical input behind a source: an RSS/Atom URL, a Standardfeed publication, or a generated feed. Feeds and their entries live in the shared catalog. |
| Subscription | One reader's relationship to one source. Feed subscriptions are records in the reader's PDS; SQLite only mirrors them. |
| Shared catalog | Feeds and entries stored once and referenced by every subscriber. It may show that someone follows a feed, never who. Newsletter data never enters it. |
| PDS-authoritative | Data whose truth is the reader's PDS (feed subscriptions, URL saves). Write the PDS first, then mirror locally; reconciliation repairs the mirror. |
| Server-owned | Data whose truth is Morgenblau's database: newsletters, newsletter saves, generated feeds. Never written to a PDS. |
| Standardfeed | A native ATProto publication (`site.standard.*`). A reader follows a site through RSS or Standardfeed, never both. |
| Sidecar | Morgenblau's metadata record (title, tags, primary) beside a Standardfeed subscription. The `site.standard` record decides whether the subscription exists; the sidecar never does. |
| Digest | The finite set of entries for one day of the reader's local calendar, not the server's. No unread counts, no backlog. |
| Primary source | A reader's preference to show a source first. Not a ranking or social signal. |
| Newsletter | An email source delivered to the reader's private random address. Private even when a message has a public web URL. |
| Hidden feed | A site's own feed that its pages fail to advertise. Discovery finds it, and it is followed like any feed, never generated. |
| Generated feed | A feed Morgenblau builds from the listing page of a site with no feed at all; called a web source in code. Server-owned, one per site, shared by all its readers, served at `/feed/<id>`. |
| Listing page | The page that links to a site's posts, such as a blog index. A generated feed is built from it, never from the posts themselves. A homepage that only teases its posts gives way to the section page it links, such as `/blog`, when that page lists them all. |
| Setup and poll | Setup builds a generated feed once and may ask the model. A poll re-reads the listing through the stored recipe and never calls a model on its own. |
| History | Posts a feed already carries when Morgenblau first fetches it, or a generated feed found at setup. Undated ones are stored without a date: Sources lists them, no digest does. Do not backfill them with the fetch time. An undated post that appears later belongs to the day Morgenblau first saw it. |
| Health | Whether a generated feed currently serves the site's posts: fine, needs attention, or blocked by the site. Shown in Sources, never in the digest. |
| Pointing at a post | A reader's fix for an unsure generated feed: paste one real post and setup rebuilds around it. Because the feed is shared, the rebuild must be confirmed before it replaces what other readers see. |
| Blocked by site | The site refuses honest bots. Morgenblau reports it and never works around it. Distinct from unreachable, which is a network or server failure. |

The Sources page groups RSS/Atom and Standardfeed subscriptions under **Feeds**, and email subscriptions under **Newsletters**. Standardfeed remains an ordinary source in product language; protocol details belong only where they help someone choose how to subscribe.

</terminology>

---

<content-types>

## Digest and Reader

Opening Morgenblau lands on today's digest. New content can arrive during the day; previous days remain available. Skipped days never accumulate into an unread queue. The digest contains only content from sources the user chose.

Digest dates use the browser's local calendar boundaries for every source type, so a selected day means the reader's local day regardless of the server timezone.

Primary sources appear at the top with distinct presentation. This is a user preference, not a social ranking signal.

The in-app reader remains the default for articles, with an option to open the original URL. YouTube playback and inline microblog rendering remain supported. Default content-type filters and custom Views are retained product scope.

HTML is sanitized before storage. Feed-supplied bodies are processed during ingestion; summary-only entries may lazily extract and cache sanitized article content on first open. Native Standardfeed documents use the canonical web page for the reader rather than requiring renderers for each publisher's content format. Documents without a web path may fall back to plain text.

Content classification belongs to ingestion. The fetch pipelines in `internal/sync/` own classification and entry metadata; reader extraction lives in `internal/api/entries.go`.

</content-types>

---

<feed-sources>

## Sources and Refresh

Readers add feeds by URL, including website feed discovery, YouTube channel resolution, and native Standardfeed publications. The source resolver lives in `internal/feedfinder/`. Source titles, tags, and primary status remain editable. A reader follows a site through its RSS feed or its Standardfeed publication, never both: the picker disables the other kind, and the server rejects a new add or feed URL change that would create the pair (`rejectCrossKindSite` in `internal/api/`). Pairs that already exist stay.

Feed content refreshes on login, manual refresh, source addition, and a background sweep. User-triggered work runs asynchronously; adding a source fetches that source. The background sweep refreshes the shared catalog without reconciling user PDS records. Scheduling, duplicate-work guards, and upstream backoff belong to `internal/sync/` and `internal/fetcher/`; generated feeds keep their own poll schedule, repair budget and backoff in `internal/websource/`.

Fetch failures remain quiet in the digest. Source management can show fetch health, including last success and muted state; failed sources retry and recover automatically.

OPML import/export covers RSS/Atom subscriptions, including YouTube feeds. It excludes native Standardfeed subscriptions, which remain in the PDS, and email subscriptions. Newsletter CSV export is a possible later addition, outside v1.

**Generated feeds.** A site without a feed can still be followed. `internal/feedfinder/` first looks for a feed the site hides (anchors, icons, `meta rss-feed`, common paths); from a homepage it also looks on the writing section the homepage links (`/blog`, `/writing`); only when none exists does the reader get the offer to generate one. A generated feed is a server-owned web source with an opaque ID, created only by signed-in readers and shared per site, served as RSS at `/feed/<id>`; the reader's PDS record is an ordinary `#rssFeed` subscription to that URL, so no lexicon changes. `/web/<id>` is a permanent alias: PDS records made before the move still name it, so it is served, polled and recognized as the same generated feed. Never put a target URL in the feed path: that shape is an open proxy. `internal/webfeed/` owns extraction, the recipe, the poll, the detector and the repair ladder; `internal/websource/` owns storage, the feed endpoint, scheduling and spend limits. A feed reader outside Morgenblau that fetches either URL follows the source as a subscription does: it keeps it polled and out of the sweep.

- Link-only: a generated feed serves title, link, date and a short excerpt. Full text still comes from in-reader extraction, so Morgenblau stays a reader rather than a republisher.
- The first poll is history: undated posts found at setup never fill today's newspaper; a post the site itself dates today appears like any feed's. Later undated posts get the time Morgenblau first saw them.
- A routine poll never calls a model. Claude Haiku 5.5 (`internal/claude/`) runs in three places only: the setup judge when the heuristic is unsure, at most one model repair per site per day, and a weekly check per source. The weekly check can raise an alarm, never close one; its alarm stands until a repair finds a different list, a later check stops doubting, or the reader points at a post. Model spend has a daily budget (`WEB_MODEL_DAILY_USD`).
- The section page wins: when a pasted homepage teases posts that live under a section it links, or lists too few, setup builds from that section page if it serves a healthy list holding the homepage's posts. Pasting the homepage or the section finds the same feed. A source that moves keeps its `/web/<id>`, so readers only gain history and no PDS record changes (`storeSetup`, `checkSection` in `internal/websource/`).
- A listing that builds its post list in the browser gets no generated feed and the reader is told so; Morgenblau does not render pages. The one exception is a Livewire lazy list, which setup and polls load with the same single request the page's own script sends.
- Health lives in Sources: a generated feed that needs attention says so there and stays quiet in the newspaper. A site that blocks us reads "This site blocks feed readers".
- An honest fetcher: `safehttp.UserAgent` identifies Morgenblau without a generic client token, requests carry the page's own language, and listing pages honor robots.txt at setup and on every poll. Sites that block our fetch get their feed or sitemap, or nothing; Morgenblau never renders them in a browser to get around that.

YouTube subscriptions can also be imported from an extracted Google Takeout CSV as ordinary public feed subscriptions. `internal/api/subscriptions_youtube.go` owns the conversion; the shared source importer owns confirmation, duplicate handling, and PDS writes.

</feed-sources>

---

<newsletter-privacy>

## Private Newsletter Data

Each user receives one random inbound email address for v1, privately mapped to their DID. Replacing or rotating that address is outside v1. The address is neither derived from the DID nor published in the PDS. An address derived from a public identity would be predictable and unnecessarily connect incoming email to that identity.

[`internal/newsletter/address.go`](internal/newsletter/address.go) generates memorable adjective–animal–place names using cryptographic randomness. Address allocation enforces uniqueness atomically and retries collisions; an assigned address stays stable.

The authenticated app-entry handler in [`internal/api/profiles.go`](internal/api/profiles.go) provisions the address when newsletter delivery is enabled. Subsequent entries reuse it; opening the source picker only reads it.

Giving that address to a publisher is the reader's decision to add the source. The first incoming message automatically creates a private Newsletter source without another approval step. While the source is active, confirmation, welcome, account, and other messages delivered to the address remain readable.

Forwarded mail remains readable and uses structured original newsletter identity when available; otherwise it is grouped under the forwarding sender so the reader can correct that message's attribution, without guessing among quoted messages or silently losing or misassigning content. A correction applies only to that message and does not create a rule for future deliveries.

Newsletter messages belong to the digest date on which they are received, so forwarded or delayed mail is not hidden in an older digest. The reader may also display the original sent date.

Remote images are blocked by default so image requests do not reveal message opens. Loading images is a private per-message choice remembered for that message only; all other messages remain blocked by default. Embedded images display as part of the message. V1 does not include general attachment downloads.

Newsletter addresses, email subscriptions, message content, and Newsletter Library saves are private server-owned data. A save originating from a newsletter remains private even when the message includes a public web-version URL. This is an explicit exception to PDS authority: public records must not expose personal email content, delivery addresses, or personalized links such as unsubscribe URLs.

Stopping a Newsletter moves it to **Stopped newsletters**, where it can be re-enabled. Unsaved newsletter messages are deleted, while saved messages remain in the Library. Future mail that still matches the stopped source is accepted and discarded without storing a new message, avoiding a bounce that could prevent the reader from resuming later; re-enabling affects only future deliveries. V1 has no separate permanent deletion action, and a publisher identity change may arrive as a new source rather than matching the stopped one.

Newsletter data is scoped to its owner and must never enter the shared feed-content cache. The existing public lexicons remain unchanged for this release.

Owner matching is enforced by both the private service and relational constraints. A newsletter save uses the private path even when its message has a public web-version URL.

</newsletter-privacy>

---

<library>

## Saving

The Library contains the user's own saved entries, with optional commentary and flat, user-defined tags. There is no Shared or Network section.

Existing URL-based saves remain `blue.morgen.feed.save` records. They are product-private: those PDS records are technically public, but Morgenblau does not display another person's saves or derive popularity signals from them. Newsletter saves follow the private newsletter storage rule above, including when the message has a public web-version URL.

</library>

---

<authentication>

## Identity and Sessions

ATProto OAuth is the deployed login mechanism. The Go server is a confidential backend-for-frontend client; browsers never receive PDS tokens. The user's DID is the stable account key. Handles and profile presentation are resolved as needed.

Alpha deployments may restrict admission by DID. `internal/middleware/auth/alpha.go` owns the environment policy; it must gate both verified sign-in and existing sessions, before authenticated application work starts. Public OAuth metadata remains reachable. Enabled alpha mode must refuse startup without a valid non-empty allowlist.

Operator usage pages live under `/admin` and `/api/admin/`. The auth gate in `internal/middleware/auth/` admits only DIDs in `ADMIN_DIDS` there (unset admits nobody) and answers everyone else as if the route did not exist; admin handlers also refuse any request the gate did not mark. They show counts per reader, never feed URLs, titles or newsletter content. `admin_routes_test.go` in `internal/server/` drives every admin route.

Explicitly opted-in local development may sign into a dedicated development account with server-held ATProto credentials, without an OAuth callback. This grants real PDS access, including writes, and must be unavailable outside local mode. `internal/session/` owns the development gate, in-memory password-session lifecycle, and shared session representation. Password authority must not be represented as an OAuth scope grant. Development sessions do not survive server restart; the shortcut can establish a new session independently.

OAuth session material is encrypted at rest. The browser receives only a sealed session reference in an HttpOnly cookie, secure in deployment. Session creation, encryption, refresh coordination, and scope requirements are owned by `internal/oauth/` and `internal/middleware/auth/`.

Keep the existing Morgenblau permission-set lexicon unchanged. Standardfeed subscription writes still need their own grant; Standardfeed recommendation writes are outside v1. A missing required grant prompts reauthentication rather than silently falling back to another authority.

</authentication>

---

<sync-architecture>

## Data Ownership

**PDS-authoritative records:** existing feed subscriptions and URL-based saves belong to the user's PDS. Their SQLite rows are derived indexes, partitioned by user. Reconciliation lists the relevant PDS collections and applies changes locally. A concurrent local mirror write must survive a listing snapshot that predates it; see `internal/sync/reconcile_guard.go`.

Mutations validate against the existing lexicon, write to the PDS, then mirror locally. Each mutation request makes at most one PDS commit, either one record or several records in one `applyWrites` transaction, so a reported failure means nothing committed. A failed mirror must not report a committed PDS write as a failed mutation; `commitThenMirror` in `internal/api/mirror.go` schedules reconciliation instead. OPML import is the bulk exception: it commits per source and reports each source's outcome.

An authenticated app entry starts a coalesced reconciliation, including for an existing session. It does not force the current page to refresh; navigation reads the committed local state. A `sync_user` job is done only after subscription and save reconciliation have both committed. Feed fetch and sidecar cleanup failures can retry separately.

**Standardfeed subscriptions:** `site.standard.graph.subscription` is the existence authority. The Morgenblau subscription record is a lazy metadata sidecar for title, tags, and primary status. Subscribing creates the standard record; a metadata edit may create the sidecar. Reconciliation honors subscriptions made in other apps and cleans up orphaned or duplicate subscription sidecars. The policy lives in `internal/sync/reconcile_subscriptions.go` and `internal/sync/reconcile_sidecar.go`.

Creating a customized Standardfeed subscription commits its existence record and metadata sidecar in one PDS transaction. Failure cannot leave an existence record that lost the requested metadata.

**Shared upstream cache:** RSS sources are keyed by canonical feed URL; Standardfeed sources by publication AT-URI. Multiple user subscriptions can reference one source and its entries. RSS polling follows HTTP cache semantics. Native Standardfeed ingestion reconciles publisher documents, including upstream deletions; RSS entries persist because RSS has no equivalent deletion signal. The pipelines live in `internal/sync/`. A source nobody follows stops being refreshed at once and is deleted, entries and generated feed included, after a grace that lets a returning reader keep its history; a saved entry keeps its source until read-later gives saves their own copy. `SweepUnfollowedFeeds` in `internal/database/` owns the policy.

The shared cache reveals that at least one user has requested a feed, but the API must not expose which other users subscribe to it. Private email data never participates in this deduplication.

Removing social features stops their runtime reads, writes, and reconciliation. Existing external share, follow, and recommendation records remain untouched. Subscription-sidecar cleanup required by retained Standardfeed behavior remains in place.

</sync-architecture>

---

<lexicons>

## Schema Compatibility

The JSON files under `lexicons/` are the schema authority. Keep the existing schemas, NSIDs, and permission set unchanged in this simplification. Historical share/follow schemas remain available even though their product features are removed.

Runtime collection constants and validation live in `internal/lexicon/`. Active subscription and save behavior must remain compatible with existing external records. The simplification must not republish lexicons or migrate, delete, or rewrite external records to fit the reduced feature set.

</lexicons>

---

<guardrails>

## Guardrails

- Never introduce unread counts, inbox-zero mechanics, infinite scrolling, or pressure to catch up.
- Preserve user isolation, encrypted session storage, PDS-first writes, and safe outbound fetching when removing features.
- Retain Go, React, SQLite, goose, and sqlc. The composition root is `internal/server/`; database schema and query details live in `internal/database/`.
- Local database compatibility is unnecessary because the application has not been deployed. A fresh local schema is allowed; external data and lexicon compatibility must be preserved.

</guardrails>
