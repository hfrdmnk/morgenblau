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

## Terminology

| Term | Meaning |
|:--|:--|
| Source | A reader-selected input, used in product copy and routes. |
| Subscription | The relationship between a user and a source. Existing feed subscriptions are represented by PDS records. |
| Feed | The technical RSS/Atom input, or a native Standardfeed publication in the shared content catalog. |
| Newsletter | An email source delivered to the user's private inbound address. |
| Digest | The finite collection of content for a selected day. |
| Library | The user's saved entries. |
| View | A filter by source, tags, content type, or a combination. |

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

Readers add feeds by URL, including website feed discovery, YouTube channel resolution, and native Standardfeed publications. The source resolver lives in `internal/feedfinder/`. Source titles, tags, and primary status remain editable.

Feed content refreshes on login, manual refresh, source addition, and a background sweep. User-triggered work runs asynchronously; adding a source fetches that source. The background sweep refreshes the shared catalog without reconciling user PDS records. Scheduling, duplicate-work guards, and upstream backoff belong to `internal/sync/` and `internal/fetcher/`.

Fetch failures remain quiet in the digest. Source management can show fetch health, including last success and muted state; failed sources retry and recover automatically.

OPML import/export covers RSS/Atom subscriptions, including YouTube feeds. It excludes native Standardfeed subscriptions, which remain in the PDS, and email subscriptions. Newsletter CSV export is a possible later addition, outside v1.

</feed-sources>

---

<newsletter-privacy>

## Private Newsletter Data

Each user receives one random inbound email address for v1, privately mapped to their DID. Replacing or rotating that address is outside v1. The address is neither derived from the DID nor published in the PDS. An address derived from a public identity would be predictable and unnecessarily connect incoming email to that identity.

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

ATProto OAuth remains the only login mechanism. The Go server is a confidential backend-for-frontend client; browsers never receive OAuth tokens. The user's DID is the stable account key. Handles and profile presentation are resolved as needed.

OAuth session material is encrypted at rest. The browser receives only a sealed session reference in an HttpOnly cookie, secure in deployment. Session creation, encryption, refresh coordination, and scope requirements are owned by `internal/oauth/` and `internal/middleware/auth/`.

Keep the existing Morgenblau permission-set lexicon unchanged. Standardfeed subscription writes still need their own grant; Standardfeed recommendation writes are outside v1. A missing required grant prompts reauthentication rather than silently falling back to another authority.

</authentication>

---

<sync-architecture>

## Data Ownership

**PDS-authoritative records:** existing feed subscriptions and URL-based saves belong to the user's PDS. Their SQLite rows are derived indexes, partitioned by user. Reconciliation lists the relevant PDS collections and applies changes locally. A concurrent local mirror write must survive a listing snapshot that predates it; see `internal/sync/reconcile_guard.go`.

Mutations validate against the existing lexicon, write to the PDS, then mirror locally. A failed mirror must not report an already committed PDS write as a failed mutation; `mirrorOrRepair` in `internal/api/mirror.go` schedules reconciliation instead.

An authenticated app entry starts a coalesced reconciliation, including for an existing session. It does not force the current page to refresh; navigation reads the committed local state. A `sync_user` job is done only after subscription and save reconciliation have both committed. Feed fetch and sidecar cleanup failures can retry separately.

**Standardfeed subscriptions:** `site.standard.graph.subscription` is the existence authority. The Morgenblau subscription record is a lazy metadata sidecar for title, tags, and primary status. Subscribing creates the standard record; a metadata edit may create the sidecar. Reconciliation honors subscriptions made in other apps and cleans up orphaned or duplicate subscription sidecars. The policy lives in `internal/sync/reconcile_subscriptions.go` and `internal/sync/reconcile_sidecar.go`.

Creating a customized Standardfeed subscription commits its existence record and metadata sidecar in one PDS transaction. Failure cannot leave an existence record that lost the requested metadata.

**Shared upstream cache:** RSS sources are keyed by canonical feed URL; Standardfeed sources by publication AT-URI. Multiple user subscriptions can reference one source and its entries. RSS polling follows HTTP cache semantics. Native Standardfeed ingestion reconciles publisher documents, including upstream deletions; RSS entries persist because RSS has no equivalent deletion signal. The pipelines live in `internal/sync/`.

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
