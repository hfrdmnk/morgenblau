# Morgenblau Spec

> Single source of truth for product behavior, content model, architecture, and technical guardrails.
> Keep high-level. Update when core decisions change, not for every feature.
> Brand feeling and art direction live in [BRAND.md](./BRAND.md).

---

<vision>

## What is Morgenblau?

Morgenblau is a personal daily web-newspaper powered by RSS and ATproto. It organizes content into finite days instead of an infinite feed.

ATproto is just a means to an end. We're not advertising as "RSS on ATproto" but as a Social RSS reader (what Google Reader could have been).

**Target users:** People who want to consume content (blogs, microblogs, videos) without the anxiety of unread counts or the pull of endless scrolling. They value the open (social) web.

**What makes it different:**

- Daily digests instead of an unread inbox
- Social layer via ATProto backlinks
- Three first-class content types with dedicated UIs
- A newspaper curated from sources the reader chooses

</vision>

---

<stack>

## Tech Stack

### Backend
- Go
- Indigo
- SQLite (via modernc)

### Frontend
- React 19
- Tailwind CSS 4
- Base UI

### Sidecars
- tap (indigo `cmd/tap`): the firehose reader that feeds discovery. Operated alongside the app, never imported by it; the app reaches it over local HTTP and a websocket channel (`TAP_URL`), and tells it which repos to follow.

</stack>

---

<routes>

## Main Routes

| Route | Description |
|:--|:--|
| Discover | Find new people (to get their recommendations) and sources to follow. |
| Sources | Managing subscribed sources. |
| Library | Saved entries by the current user. Shared entries by their network. |
| Digest | RSS-feed entries, grouped by day. The core of Morgenblau. |

</routes>

---

<terminology>

## Terminology

Three near-synonyms with disciplined assignments to keep the codebase coherent.

| Term             | Where it lives                                                                  | Refers to                                                |
| ---------------- | ------------------------------------------------------------------------------- | -------------------------------------------------------- |
| **Source**       | User-facing copy, routes (`/sources`), page labels                              | What the user *chooses* — their curated input list       |
| **Subscription** | API (`/api/subscriptions`), Go entity, lexicon (`blue.morgen.feed.subscription`) | The PDS record representing one source                   |
| **Feed**         | Internal/technical (RSS/Atom mechanics, fetch pipeline)                         | The underlying RSS/Atom URL the subscription points at   |

A user **adds a source** → the app **creates a subscription record** → the fetcher **polls the feed**.

Use "sources" rather than "subscriptions" or "feeds" in user-facing routes and pages. The other terms describe implementation details.

Standardfeed sources get **no user-facing noun** (not "Publication", not "Standardfeed") — a source is a source. The only user-facing differentiator is a "Subscribe via ATProto" affordance in the picker with a tooltip that leads with the benefit (subscription lives in the user's own account, portable across apps, shares reach the Atmosphere), not the mechanism. RSS candidates carry no label at all. Internally the Tier-2 kind is `standardfeed`.

**Reader network**: the set of apps whose reading records feed discovery: Morgenblau, Skyreader, Glean, Standardfeed. It supplies discovery *candidates* (subscriptions, saves, shares, recommends). margin.at is not a member: annotations stay a render-time layer, never a discovery signal. Adjacent social graphs (Bluesky follows, Tangled follows) are also not members; they contribute only to personal ranking, as weaker trust signals than a follow inside the reader network (Morgenblau, Skyreader). Two-layer rule: **the reader network supplies the candidates; social graphs rank them.**

</terminology>

---

<lexicons>

## Lexicons

### Why Our Own Lexicon

Morgenblau writes all user-owned records under `blue.morgen.*`. Skyreader's schemas are too verbose for our purpose: they denormalize metadata like `author`, `excerpt`, `image`, and `wordCount` that we'd rather look up from our Tier-2 cache. Glean's overlap is narrow: no share concept, plus `rating` and `quote` dimensions on annotations that we don't model.

Owning the schema lets us stay minimal and move fast while still interoperating where it earns its keep. We import existing subscriptions and saves from other readers (user can choose to do so), and count cross-reader signals in discovery. The end goal is a shared standard between RSS readers in the atmosphere. Until that exists, `blue.morgen.*` is our contribution to the conversation.

Standardfeed (`site.standard.*`) is the first piece of that shared standard we adopt outright, and it gets a different posture than Glean/Skyreader: its records are **co-owned, always-synced state** (read and written like `blue.morgen.*`), not opt-in imports. A subscription created in Leaflet is a Morgenblau source on next sync, no consent step — it's the user's own canonical data, not another app's. The one-way import posture applies only to foreign reader lexicons we read but never write.

### Why a Separate Follow Graph

A Bluesky follow means "I want this person's posts in my timeline." A Morgenblau follow means "I trust this person's reading taste." Morgenblau leans slower and more curatorial than Bluesky's fast-paced feed, so the same word carries different intent in each app.

Bluesky follows are never auto-mirrored as Morgenblau follows. They surface only as discovery suggestions ("people you know from Bluesky") on the Discover route, which solves cold start without conflating the two signals.

### Our Records

| NSID | Purpose |
|:--|:--|
| `blue.morgen.feed.subscription` | A user's curated feed source |
| `blue.morgen.feed.save` | Saved item for later reading (private) |
| `blue.morgen.feed.share` | Broadcast item (public "I like this") |
| `blue.morgen.graph.follow` | In-app follow, distinct from Bluesky's |

All four use `tid` rkeys, and all require `createdAt`. Subscriptions additionally require the `source` union; every other field is optional. The record carries identity and intent. The Tier-2 cache renders the rest (see `<sync-architecture>`).

**Schema evolution stance:** once a `blue.morgen.*` lexicon has real adoption, it evolves in place with non-breaking changes only (add optional field, relax required to optional; both blessed by the [lexicon compat rules](https://atproto.com/specs/lexicon#versioning-and-breaking-changes)), bumping `revision` each time and republishing the `com.atproto.lexicon.schema` record. Sum types are the exception worth deciding at birth: subscription's `source` is a required **open union** (`#rssFeed` | `#standardPublication`) rather than flat optional fields with an app-enforced XOR, because exactly-one-source is the record's core invariant and belongs in the schema, and union decisions are one-shot (restructuring later is breaking). The restructure shipped as revision 2 while adoption was zero, the only window where breaking is free. Readers must tolerate unknown `source` variants; the reconciler skips records whose variant it doesn't recognize and logs them. `feed.share` deliberately stays flat: its `document` field complements `itemUrl` rather than excluding it, so no union is warranted there.

#### `blue.morgen.feed.subscription`

```json
{
  "lexicon": 1,
  "id": "blue.morgen.feed.subscription",
  "revision": 2,
  "defs": {
    "main": {
      "type": "record",
      "description": "A subscription to a content source, identified by the source union: an RSS/Atom feed or a Standardfeed publication.",
      "key": "tid",
      "record": {
        "type": "object",
        "required": ["source", "createdAt"],
        "properties": {
          "source": {
            "type": "union",
            "refs": ["#rssFeed", "#standardPublication"],
            "description": "Identity of the subscribed source. Open union; readers tolerate unknown variants."
          },
          "title": {
            "type": "string",
            "maxGraphemes": 128,
            "maxLength": 1280,
            "description": "Display title. Auto-prefilled from the source, user-editable."
          },
          "tags": {
            "type": "array",
            "maxLength": 10,
            "items": { "type": "string", "maxGraphemes": 64, "maxLength": 640 },
            "description": "Free-form, user-defined tags. No controlled vocabulary."
          },
          "primary": {
            "type": "boolean",
            "description": "Whether the source receives special treatment in the digest."
          },
          "createdAt": {
            "type": "string",
            "format": "datetime"
          }
        }
      }
    },
    "rssFeed": {
      "type": "object",
      "required": ["feedUrl"],
      "properties": {
        "feedUrl": {
          "type": "string",
          "format": "uri",
          "maxLength": 2048,
          "description": "URL of the RSS/Atom feed."
        },
        "siteUrl": {
          "type": "string",
          "format": "uri",
          "maxLength": 2048,
          "description": "Human-facing site URL associated with the feed."
        }
      }
    },
    "standardPublication": {
      "type": "object",
      "required": ["publication"],
      "properties": {
        "publication": {
          "type": "string",
          "format": "at-uri",
          "description": "AT-URI of the site.standard.publication record. This subscription is a metadata sidecar; the paired site.standard.graph.subscription record is the existence authority."
        }
      }
    }
  }
}
```

#### `blue.morgen.feed.save`

```json
{
  "lexicon": 1,
  "id": "blue.morgen.feed.save",
  "defs": {
    "main": {
      "type": "record",
      "description": "A saved feed item for later reading.",
      "key": "tid",
      "record": {
        "type": "object",
        "required": ["itemUrl", "createdAt"],
        "properties": {
          "itemUrl": {
            "type": "string",
            "format": "uri",
            "maxLength": 2048,
            "description": "URL of the saved item."
          },
          "feedUrl": {
            "type": "string",
            "format": "uri",
            "maxLength": 2048,
            "description": "URL of the source feed (optional provenance)."
          },
          "comment": {
            "type": "string",
            "maxGraphemes": 3000,
            "maxLength": 30000,
            "description": "User's note on why they saved it."
          },
          "facets": {
            "type": "array",
            "items": { "type": "ref", "ref": "app.bsky.richtext.facet" },
            "description": "Rich-text annotations (mentions, links, tags) over `comment`."
          },
          "tags": {
            "type": "array",
            "maxLength": 10,
            "items": { "type": "string", "maxGraphemes": 64, "maxLength": 640 },
            "description": "Free-form, user-defined tags (e.g. 'read-later', 'favorite'). No controlled vocabulary."
          },
          "createdAt": {
            "type": "string",
            "format": "datetime"
          }
        }
      }
    }
  }
}
```

#### `blue.morgen.feed.share`

```json
{
  "lexicon": 1,
  "id": "blue.morgen.feed.share",
  "revision": 2,
  "defs": {
    "main": {
      "type": "record",
      "description": "A shared feed item. Broadcasts 'I like this' with optional commentary. For Standardfeed documents this is the lazy comment sidecar of a site.standard.graph.recommend record.",
      "key": "tid",
      "record": {
        "type": "object",
        "required": ["itemUrl", "createdAt"],
        "properties": {
          "itemUrl": {
            "type": "string",
            "format": "uri",
            "maxLength": 2048,
            "description": "URL of the shared item."
          },
          "document": {
            "type": "string",
            "format": "at-uri",
            "description": "AT-URI of the site.standard.document this share refers to, when the item is a Standardfeed document. Joins the share to its paired recommend record."
          },
          "feedUrl": {
            "type": "string",
            "format": "uri",
            "maxLength": 2048,
            "description": "URL of the source feed (optional provenance)."
          },
          "comment": {
            "type": "string",
            "maxGraphemes": 3000,
            "maxLength": 30000,
            "description": "User's commentary on the share."
          },
          "facets": {
            "type": "array",
            "items": { "type": "ref", "ref": "app.bsky.richtext.facet" },
            "description": "Rich-text annotations (mentions, links, tags) over `comment`."
          },
          "createdAt": {
            "type": "string",
            "format": "datetime"
          }
        }
      }
    }
  }
}
```

#### `blue.morgen.graph.follow`

```json
{
  "lexicon": 1,
  "id": "blue.morgen.graph.follow",
  "defs": {
    "main": {
      "type": "record",
      "description": "An in-app follow of another Morgenblau user. Distinct from a Bluesky follow.",
      "key": "tid",
      "record": {
        "type": "object",
        "required": ["subject", "createdAt"],
        "properties": {
          "subject": {
            "type": "string",
            "format": "did",
            "description": "DID of the user being followed."
          },
          "createdAt": {
            "type": "string",
            "format": "datetime"
          }
        }
      }
    }
  }
}
```

### Permission Set

| NSID | Purpose |
|:--|:--|
| `blue.morgen.access` | Bundles write access to the four `blue.morgen.*` collections into a single OAuth scope (`include:blue.morgen.access`) |

Published as a `permission-set` lexicon so OAuth clients request one scope instead of enumerating four `repo:` grants. Authority for `blue.morgen.*` is claimed via `_lexicon.morgen.blue` DNS TXT pointing at the publisher DID.

### External Lexicons

| NSID | Source | How we use it |
|:--|:--|:--|
| `site.standard.graph.subscription` | Standardfeed | **Read + write.** Existence record for publication subscriptions; two-way synced with `blue.morgen.feed.subscription` sidecars |
| `site.standard.publication` | Standardfeed | Read. Publication identity + display metadata (name, icon, url) for Tier-2 |
| `site.standard.document` | Standardfeed | Read. Entry ingestion for publication sources — PDS-native, no RSS involved |
| `site.standard.graph.recommend` | Standardfeed | **Read + write.** Existence record for shares of Standardfeed documents; `blue.morgen.feed.share` is its lazy comment sidecar. Popularity signal (1.5×) |
| `pub.leaflet.publication` | Leaflet | Read. Leaflet dual-writes discoverable publications into `site.standard.publication` under the same rkey; discovery resolves that sibling first and falls back to this record's `base_path` RSS feed only for legacy un-mirrored publications |
| `app.bsky.graph.follow` | Bluesky | Suggestions only ("people you know from Bluesky"); never auto-mirrored |
| `at.margin.note` | margin.at | Margin annotations rendered alongside articles |
| `at.glean.like` | Glean | Popularity signal (1×); importable as `blue.morgen.feed.save` |
| `at.glean.subscription` | Glean | Importable as `blue.morgen.feed.subscription` |
| `app.skyreader.feed.subscription` | Skyreader | Importable; respected in discovery |
| `app.skyreader.social.follow` | Skyreader | Read. Reader-network trust signal: strong trust tier, one-hop people discovery, trending follower counts. Unpublished lexicon (shape-identical to `blue.morgen.graph.follow`); consumed by NSID convention |
| `app.skyreader.feed.saved` | Skyreader | Popularity signal (1×); importable as `blue.morgen.feed.save` |
| `app.skyreader.social.share` | Skyreader | Popularity signal (1.5×); importable as `blue.morgen.feed.share` |

**Popularity weighting:** saves count 1×, shares 1.5×. Equivalent records on Skyreader and Glean are counted at the same weights; Standardfeed recommends count as shares (1.5×). Glean's `annotation` (note, quote, rating) is not consumed. Annotations are delegated to margin.at.

</lexicons>

---

<authentication>

## Authentication

ATProto OAuth is the only auth mechanism — no passwords, email, or registration. The Go server is a **confidential BFF client** (browsers never see access tokens) built on `indigo/atproto/auth/oauth`.

Session state lives in two SQLite tables, both encrypted at rest before any public deploy:

- `oauth_sessions(did, session_id, data BLOB)` — opaque indigo session blob (refresh token, DPoP key, granted scope, expiry, PDS+AS endpoints). Composite PK allows multiple concurrent sessions per DID (laptop + phone).
- `oauth_auth_requests(state PK, data BLOB, expires_at)` — short-lived pre-callback state; cron-GC'd, never read in the hot path.

Handle is never persisted in the DB. It's re-resolved on demand via indigo's cached identity directory (`identity.DefaultDirectory`), so handle changes at the PDS are reflected without migration. Profile data (avatar, display name) is re-fetched live from the PDS when needed.

The browser session cookie carries `(did, session_id)` only (sealed, `HttpOnly; Secure; SameSite=Lax`) — all OAuth material stays server-side.

Scopes: `atproto include:blue.morgen.access repo:site.standard.graph.subscription repo:site.standard.graph.recommend`, following [Dan Abramov's guidance](https://underreacted.leaflet.pub/3mjfozhlhys2z). The permission set expands to per-collection `repo:` writes on the four `blue.morgen.*` collections; the two `site.standard.graph.*` grants cover the co-owned Standardfeed records (see `<lexicons>`) and are requested together even though recommends may ship after subscriptions — one re-auth, not two. Avoid `transition:generic` and the unsupported partial wildcard `repo:blue.morgen.*`.

Public endpoints: `/oauth-client-metadata.json` (advertised `client_id` in prod), `/oauth-jwks.json` (public half of the P-256 client key), `/oauth/login` (POST), `/oauth/callback` (GET), `/oauth/logout` (POST).

References:
- [ATProto OAuth](https://atproto.com/specs/oauth), [ATProto permissions](https://atproto.com/specs/permission)

</authentication>

---

<content-types>

## Three Content Types

All three are first-class citizens in v1, each with a UI optimized for its format.

| Type      | Description                        | Playback                 |
| --------- | ---------------------------------- | ------------------------ |
| Blogpost  | Articles with titles and body text | In-app reader + link out |
| Microblog | Short posts without a title        | Inline in digest         |
| Video     | YouTube              | Video player in in-app reader            |

### Reading Mode

In-app reader by default — fetch and render article content directly. Users can always open the original URL. Both options available.

### Classification & Sanitization

Content type is **classified at fetch time and persisted** — entries land in storage with a `content_type` column already set, not derived at render. Same applies to HTML sanitization for the in-app reader: sanitize once during the fetch pipeline, store the safe form, never sanitize at render.

Documented exception: when the RSS feed only ships a summary, the reader **lazily** runs readability extraction against the source URL on first open and caches the sanitized result on the entry row. Eager sanitize-at-fetch still governs the feed-shipped body; the lazy path applies only to the readability-extracted body.

Standardfeed document entries always take this lazy path: the digest summary comes from the record's `description`/`textContent`, and the reader body is readability-extracted from the document's canonical URL (`site` + `path`) on first open. The record's `content` field is an open union of per-app formats (Leaflet blocks, etc.) and is deliberately not rendered — we refuse to maintain per-publisher format renderers. Documents without a `path` fall back to plaintext `textContent`.

Type-specific metadata (reading-time, YouTube video id, etc.) lives in a `metadata` JSON column on `feed_entries`. Fields get promoted to typed columns only when their content-type UI ships and the access patterns are known.

</content-types>

---

<views>

## Views

Views are filters of content, different lenses to look through.

### Default Views (Predefined)

The app provides default views based on content type (e.g., Blogposts, Videos, Microblogs).

### Custom Views (User-Created)

Users can create their own views with custom filter criteria — by tags, sources, content types, or combinations.

### Default Landing

When a user opens Morgenblau, they land on **today's digest** — a unified view of the current day's content across all sources. Views are available for filtering from there.

</views>

---

<feed-sources>

## Feed Sources

### Adding Sources

Users add sources by pasting a URL. Morgenblau resolves the URL into one or more feeds: it follows `<link rel="alternate" type="application/rss+xml">` (and Atom equivalents) on HTML pages, and maps YouTube channel / `@handle` / `/c/` / `/user/` URLs to the corresponding `feeds/videos.xml`. Each subscription is stored as a `blue.morgen.feed.subscription` record in the user's ATProto repo.

### Organization

Flat list of subscriptions. Views handle the filtering/viewing (see `<views>`).

### Primary Sources

Users can mark feeds as **primary sources**. These receive prominent placement in the digest — front-page treatment.

### Refresh Cadence

Refresh has **three user-initiated triggers** plus a **background sweep**:

- **Manual refresh** is available on the digest view.
- **On subscription add**, the new subscription is fetched immediately (only that feed, not the whole set).
- **On login**, all of the user's subscriptions are refreshed (behaves like manual refresh), subject to a 5-minute in-flight guard so repeated logins don't thrash upstream feeds.
- **Background sweep** re-fetches every feed in the shared Tier-2 catalog on a global timer (`FETCH_INTERVAL_MINUTES`, default 30; `0` disables). It is system-wide, not per-user: it touches no PDS records and creates no jobs, so it never lights up a refresh indicator. Conditional GET (etag/last-modified) keeps re-fetches cheap, and the fetcher's worker pool bounds upstream load. This timer governs Tier-2 content only; discovery ingestion runs continuously off tap on its own cadence (see `<discovery>`).

Upstream politeness for ATProto is enforced at the client, not at each call site: every outbound XRPC path is built by `atxrpc.New`, which installs a shared per-host cooldown honoring `Retry-After` and rate-limit headers (`internal/atxrpc/cooldown.go`). New fetch loops inherit it by construction rather than hand-rolling retries. RSS fetches keep the fetcher's own per-feed backoff (see Failure Handling).

User-initiated refreshes (manual, add, login) dispatch asynchronously, controllers return immediately. While any job is in flight the digest renders its loading skeleton, and once the job goes quiet the digest re-fetches in place. No count, no badge, no persistent indicator. Consistent with the "no unread counts" anti-feature.

Architecture must permit evolving toward finer real-time refresh per feed (HTTP caching headers, per-feed `next_check_at`, exponential backoff on errors), and toward throttling the background sweep per user (e.g. a bounded number of updates per user per day) instead of re-fetching the whole catalog every tick.

### Failure Handling

Failed fetches back off exponentially (5min → 15min → 1h → 6h → 24h cap) until success or auto-disable. After **20 consecutive failures** the feed is silently muted. Muted feeds still auto-retry once per day. On the first success they silently re-enable, no user action required.

Failure state is **visible only in the sources list** as quiet metadata (last successful fetch time, muted state). The digest itself never surfaces feed errors.

</feed-sources>

---

<social-layer>

## Social Layer (ATProto)

The core differentiator. For each piece of content, the app checks for ATProto backlinks and displays social context alongside it.

### Scope

- **Read:** Show shares (including Standardfeed recommends), Bluesky reposts and margin.at annotations. No like counts, shares are higher signal.
- **Follow:** In-app follows stored as `blue.morgen.graph.follow` records (separate from Bluesky social graph follows).

### The Follow Contract

Following a person does exactly two things: their shares appear in the user's Library (network section), and their taste feeds personal source suggestions at the strongest trust tier (see `<discovery>`). It deliberately does **not** auto-subscribe to publications they author (their publication ranks top of suggestions via the authorship signal; becoming a digest source stays an explicit act — the digest is built only from sources the user chose) and puts **nothing** in the digest. You follow a person's taste, not a timeline of them.

A person can never follow themselves. Self remains searchable for profile access with the follow action omitted, and an invalid self-follow record encountered during reconciliation is removed.

### UX Principle

Social context is available but not forced. The reading experience comes first. Reactions are opt-in per article — shown only if the user wants to see them.

</social-layer>

---

<discovery>

## Discovery

The Discover route answers "where do I find good stuff to read?" using the reader network (see `<terminology>`). Two tabs (Sources, People) toggled **in-page**, not in the app chrome — a chrome-level subnav would put a second "Sources" at equal visual weight beside the main nav's. Each tab has two suggestion classes: **Personal** (ranked by the user's own graphs) and **Global/Trending** (network-wide aggregates).

### Data acquisition

Ingestion runs through a **tap sidecar** (indigo `cmd/tap`, see `<stack>`) that consumes the firehose for the repos Morgenblau tracks. Relay enumeration survives only as the seed that tells tap which repos to follow; it no longer acquires records itself.

- **Personal**: `listRecords` crawls of the repos of people the user follows (bounded set), plus single-repo crawls when the user inspects one person (card expansion, profile page), cached in local SQLite with a TTL. Crawls fan out per signal in batches, never per candidate, and the assembled payload is memoized per user for a short TTL (`internal/discovermemo`), so paging and re-visits reuse one assembly instead of re-running the fan-out. Same posture as Tier-1/Tier-2: local tables are derived caches, re-derivable from PDSes.
- **Global/Trending**: tap streams every tracked repo's records into a local mirror and marks the repo dirty; a rebuild worker drains the dirty set and reduces each repo's mirrored records into local aggregate tables (canonical source key → per-signal counts). Records therefore arrive continuously rather than in a nightly pass. A seeder enumerates the relay per collection (`com.atproto.sync.listReposByCollection`) on an interval (`DISCOVER_BATCH_INTERVAL_HOURS`, daily by default) and hands tap the repos it has not been told about, so enumeration only ever widens the tracked set. One computation serves all users. Ingestion lives in `internal/tapingest`; the signal reduction and the aggregate writers stay in `internal/discoverbatch`.

Aggregates are keyed by the same canonical source keys as Tier-2 (canonical feed URL for `rss`, publication AT-URI for `standardfeed`), so cross-reader dedup falls out of the keying.

### Personal ranking (Sources)

`score(source) = Σ over followed people p: trust(p) × strongest signal(p, source)`. The orderings are spec; the exact numbers live in code.

- **Trust tiers:** reader-network follow (Morgenblau, Skyreader) > Bluesky/Tangled follow. A person in both tiers takes the higher, not the sum.
- **Signal ordering:** authors the publication > subscribes to it > shared an item from it. Saves are excluded — a save is never attributed to a person, so it never enters personal ranking or reason lines (see `<saving-sharing>`); saves count only in anonymous trending aggregates.
- All signals are per-source: an item reaction counts toward the source it came from (via `feedUrl`/`document` provenance, Tier-2 `itemUrl` lookup as fallback).
- **Time:** standing signals (authorship, subscription) are timeless with a mild recency lean (newer subscription slightly outranks an old one, actively publishing author outranks a dormant one). Reaction signals (shares — and saves on the trending side) decay with age, Hacker-News-gravity style.
- **One signal per person per source** (strongest wins), so prolific sharers don't dominate.
- **Weak-tier cap:** total Bluesky/Tangled contribution per source is capped at roughly two strong subscribers' worth, so wide-graph virality can surface a source but never bury what reader-network friends actually read.
- **Filters before ranking:** sources the user already subscribes to (by canonical key) and hidden sources drop out.
- **Every suggestion carries its reason** ("3 people you follow subscribe", "@alice shared this"), derived from its top contributors, plus up to 3 contributor DIDs so the card can show an avatar stack of exactly the people the count claims.

Future direction: collaborative filtering over the mirror's subscription matrix (or a small learned ranker) can replace the hand-tuned weights without touching acquisition.

### Global/Trending ranking (Sources)

Same signal weights and gravity decay as personal ranking (plus saves at their `<lexicons>` popularity weight, which trending may count because aggregates are anonymous), summed over the whole network from the trending aggregate tables, with no trust term. Two filters keep it signal instead of noise:

- **Language:** trending only shows sources in the languages the user demonstrably reads, inferred from their own subscriptions (content-based detection is primary, the feed's `language` tag is a hint only). App locale is the cold-start fallback for users with no subscriptions. Sources whose language can't be determined **pass the filter**: occasionally showing a wrong-language source beats silently eating a good one.
- **Quality bar:** a source needs signals from **≥3 distinct repos** to trend at all. Kills single-repo spam and self-promotion, and doubles as a floor on quality.

Already-subscribed and hidden sources drop out, same as personal.

Trending sources are delivered inside the unified sources response, not a separate endpoint: personal cards carry a per-card trending flag, and trending-only sources (no personal signal) are appended as their own cards. A source that has any personal signal never appears as a trending-only card, and an aggregate read failure degrades the list to personal-only rather than erroring.

### People

**Eligibility:** only people with visible reader-network presence (≥1 subscription, share, or authored publication) are ever suggested — following someone with no reader records yields nothing under the follow contract (see `<social-layer>`). Saves don't confer eligibility: they're invisible under save privacy, so a saves-only person would surface with nothing to preview.

**Personal candidates:** (1) the user's Bluesky follows active in the reader network, (2) their Tangled follows likewise, (3) one hop inside the reader network: people followed by their Morgenblau/Skyreader follows. Ranked by gravity-decayed reader-network activity plus a taste-overlap bonus (shared canonical source keys), reason on every card ("you follow on Bluesky", "followed by @alice", "reads 4 of your sources").

**Global/Trending:** ranked by reader-network follower count (`blue.morgen.graph.follow` + `app.skyreader.social.follow`) plus decayed share activity; same ≥3-distinct-repos bar.

**Search:** the People tab opens with whole-network person search (Bluesky AppView typeahead), which replaces the bare handle-follow form as the manual path — suggestions and search are a convenience, never a gate. Search finds, it never follows directly: selecting a result materializes that person as an expanded card in place, with follow but no hide (hide is a suggestion signal; a searched person isn't a suggestion). The follow action is omitted when the result is self. Results with reader-network presence rank first and carry a taste hint; anyone else stays followable, but a presence-less result shows its emptiness honestly ("not in the reader network yet") instead of permitting a silent no-op follow.

Already-followed people drop out; hide works identically to sources (same snooze mechanism, keyed by DID).

### The user's own foreign records

The user's own Skyreader/Glean subscriptions are wired into personal source suggestions ("For you") as regular candidates at the highest trust tier (self > reader-network follow), each carrying its reason ("you subscribe on Skyreader") and one-tap subscribe. This per-source flow is the primary import path. A bulk import wizard (settings, with consent step) may ship later; it is out of discovery v1.

**De-dup rule, all entry points:** a foreign record whose canonical source key matches an existing Morgenblau subscription is invisible everywhere — never suggested, never importable, silently skipped in bulk import. For saves/shares the dedup key is `itemUrl`.

### Hiding and rotation

- **Hide state is never a PDS record.** A hidden suggestion is a private negative taste signal; PDS records are public, so publishing it would leak taste and pollute the repo. Hides live in server SQLite only (`did`, canonical source key, `hidden_until`) — synced across the user's devices via the server, not portable to other apps, and that's fine for an ephemeral signal.
- **Hide = snooze, never forever:** 30 days on first hide, 180 days when the same source is hidden again. No permanent-block concept and no management UI for hidden items.
- **Fresh seeded rotation:** each uncached visit gets a random seed and freezes the ranking time. Within a score band (near-ties dominate at small network scale), the seed shuffles order without letting weaker bands jump stronger ones. The seed and ranking time travel in the pagination cursor, so manual pagination and tab switches within the one-hour in-memory cache keep one stable sequence. A hard reload starts a fresh sequence. A small pool or separated score bands may still produce the same first page. Future refinement, not v1: impression-based demotion of repeatedly ignored suggestions.

### Presentation

Sources and People are each **one unified list, no sections**, loaded eight suggestions at a time. Each page takes up to four personal-ranked cards followed by up to four trending-only cards; when one pool has fewer than four remaining, the other fills the open slots. Every eligible candidate can eventually appear. Trending is a per-card reason line ("Trending in the reader network", where the reader-network term earns its user-facing existence), suppressed by any personal signal. People trending mirrors sources delivery: one response, per-card trending flag, aggregate read failure degrades to personal-only. `GET /api/discover/sources` and `GET /api/discover/people` return `{ items, nextCursor? }`; the opaque, endpoint-specific cursor continues the frozen sequence, and these responses are never cached by HTTP intermediaries. The People tab runs top-to-bottom as find → consider → manage: search, then the unified suggestion list and its Load more button, then the user's follow list (the only unfollow surface). Suggestion lists are finite and allow deliberate manual pagination. There is no infinite scroll, so the user can still finish the page like the digest.

**Cold start:** no follows and no subscriptions means no personal cards; the sources list still shows trending-only cards (language-filtered by app locale) plus the two manual affordances: add source by URL, follow by handle. No onboarding wizard.

**Cards:** a source card shows favicon and title, a collapsible preview of its last 3 posts (lazily fetched, server-cached with a TTL, best-effort: the card renders fine without it; post titles link to the original site, candidates get no in-app reader since the preview's job is the subscribe decision, not reading), one reason line (the highest-ranked signal only, with contributor avatars), subscribe (reusing the existing add flow, incl. the standardfeed affordance per `<terminology>`), and hide. Subscribing keeps the card in place in an inert subscribed state rather than removing it. Person cards get follow, hide, and a small taste preview (a few source names they read, or their latest share) — a bare avatar gives no basis to judge someone's taste. Expanding a person card (same grammar as source cards) answers **writes / reads / shares**: up to 2 authored publications (subscribable — the follow contract's explicit act, given a home), 3–4 subscriptions (already-subscribed ones marked inert per the de-dup rule, novel ones one-tap subscribable), and their latest share (a preview of the Library payoff). Lazily fetched on expand, server-cached with a TTL, best-effort, like source post previews. Person cards expand for taste context and provide access to the full profile; person affordances elsewhere (reason avatars, Library sharers) continue to link there too.

**Profile page** (`/profile/{handle-or-did}`, handle preferred in links, DID as the stable fallback): the full version of the person card. Header: avatar, display name, handle, Bluesky bio, follow/unfollow (hidden on self), and a reader-network meta line. Body: segmented **Writes | Shares | Reads** (Writes hidden when the person authors nothing — no dead tab), newest first, max 10 items per segment with load-more. Profile segments are archives the user chose to browse, so pagination is allowed; the no-infinite-scroll rule also governs suggestion surfaces and the digest. The page resolves for any identity; zero reader records renders the honest empty state, never an error. Saves are never shown (see `<saving-sharing>`).

</discovery>

---

<sync-architecture>

## Sync Architecture

Two-tier storage with different authority and sharing models.

The discovery mirror tap fills (see `<discovery>`) belongs to neither tier: it is a network-wide, anonymous derived cache that feeds trending aggregates only. It never supplies Tier-2 entries and never stands in for a Tier-1 reconcile.

### Tier 1 — PDS-mirrored (per-user)

User-owned records (`blue.morgen.feed.subscription`, `feed.save`, `feed.share`, `graph.follow`) live authoritatively on the user's PDS. Local SQLite tables holding the same data are **derived indexes only** (per `<lexicons>`). Reconciliation: `listRecords` against PDS, diff against local index, apply changes. Reconciliation triggers are the same as the user-initiated fetch triggers in `<feed-sources>` (login, manual refresh, add); the background sweep is Tier-2 only and never reconciles PDS records. A local row created *after* the PDS listing snapshot is never deleted by that reconcile: it is absent from the listing by timing, not by remote delete, so an in-flight write survives a concurrent sync (`internal/sync/reconcile_guard.go`).

**Mutations are PDS-first:** dedupe, validate against the lexicon, write to the PDS, then mirror into the local index. The mirror write can never fail the response, because the PDS write it follows already committed and the PDS is the authority; a failed mirror dispatches a reconcile instead of surfacing an error (`mirrorOrRepair` in `internal/api/mirror.go`).

**Publication sources (Standardfeed).** For sources backed by a `site.standard.publication`, the `site.standard.graph.subscription` record is the **sole existence authority** — "am I subscribed" is answered only by that record. The `blue.morgen.feed.subscription` sidecar (joined by publication AT-URI) carries Morgenblau-only metadata (title, tags, primary) and is created **lazily**, on the user's first metadata edit; subscribing writes only the standard record. This split makes deletes tombstone-free in both directions: an orphaned sidecar (standard record gone — user unsubscribed in another app) is deleted on reconcile; a standard record without a sidecar is a healthy subscription with default metadata (title falls back to the cached `publication.name`). Reconcile reads both collections; its **only** PDS write is deleting a redundant sidecar — an orphan (existence record gone) or a non-canonical duplicate left by a sync/PATCH race, where the newest sidecar wins and the older is removed (subscription and share sidecars alike) — otherwise it never writes.

The same pattern governs shares of Standardfeed documents: `site.standard.graph.recommend` is the existence record, `blue.morgen.feed.share` (joined by `document` AT-URI) is the lazy sidecar created only when the user writes a comment. "My shares" is derived from the union of both collections; orphaned and duplicate share sidecars are deleted on reconcile.

### Tier 2 — Upstream cache (shared, global)

Parsed feed metadata (etag, last-modified, title, etc.) and parsed entries are cached in local SQLite, **deduped by canonical source key across all Morgenblau users**. Tier-2 is **one catalog with two kinds** of source: `rss` rows keyed by canonical feed URL, `standardfeed` rows keyed by publication AT-URI. One catalog row per key; many Tier-1 subscriptions can point at it; entries hang off a single FK regardless of kind. The fetcher polls each upstream source once regardless of how many users subscribe, branching on kind:

- **rss** — HTTP fetch with conditional GET (etag, last-modified), as before.
- **standardfeed** — full `listRecords` diff of the publisher repo's `site.standard.document` collection: new records become entries, changed CIDs update them, and records missing upstream **hard-delete** the cached entry (ATProto deletes are honored; a saver keeps only the itemUrl, like a saved RSS item whose page 404s). RSS entries, by contrast, persist as cache forever — RSS has no delete signal. Document entries render via the lazy readability path (see `<content-types>`); the open-union `content` field is deliberately left unrendered.

Local is canonical for Tier 2 — there is no PDS path *authoritative* for entries; RSS entries come from upstream feeds, standardfeed entries are re-derivable from publisher repos. Cache invalidation follows HTTP semantics for rss and repo-diff semantics for standardfeed.

### Privacy posture

Cross-user dedup means *the existence of a feed URL in our cache* implies "at least one Morgenblau user subscribes to it." Subscription lists themselves remain per-user (in PDS). No user can enumerate which other users subscribe to a given feed via Morgenblau's API. To be documented in any future privacy/data policy.

</sync-architecture>

---

<saving-sharing>

## Saving & Sharing

Simple and minimal.

- Users can **save** individual articles to a separate saved-items view — stored as `blue.morgen.feed.save` records
- Users can **share** articles with optional commentary — stored as `blue.morgen.feed.share` records
- Share surfaces present shared items as readable links and never use raw record identifiers as labels.
- Saves carry optional user-defined `tags` (e.g. `read-later`, `favorite`). Flat tag list, no folders or hierarchies. Filtering surfaces through Views.
- Saves are **product-private**: the PDS records are technically public, but Morgenblau never renders another user's saves anywhere (profile pages, person cards, counts). Shares are the public voice; saves are the private shelf. Aggregate popularity counting (see `<lexicons>`) is the sole exception, and it never attributes a save to a person.

</saving-sharing>

---

<anti-features>

## Anti-Features

Things Morgenblau will never do.

### Hard No

- **No unread counts.** Never show unread badges, counts, or inbox-zero mechanics. This is the foundational design principle.

</anti-features>
