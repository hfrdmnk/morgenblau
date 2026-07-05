# Morgenblau Spec

> Single source of truth for product vision, content model, and guardrails.
> Keep high-level. Update when core decisions change, not for every feature.

---

<vision>

## What is Morgenblau?

A calm content platform powered by RSS and ATproto. A window into the Atmosphere that organizes content into finite daily digests instead of infinite feeds.

ATproto is just a means to an end. We're not advertising as "RSS on ATproto" but as a Social RSS reader (what Google Reader could have been).

**Core emotional promise:** Intentionality without deprivation. You still get the good stuff, but on your terms.

**Target users:** People who want to consume content (blogs, microblogs, videos) without the anxiety of unread counts or the pull of endless scrolling. They value the open (social) web.

**What makes it different:**

- Daily digests instead of an unread inbox
- Social layer via ATProto backlinks
- Three first-class content types with dedicated UIs
- The "editor of your own publication" identity: you curate the sources you value

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

Avoid: "manage subscriptions" in user copy (per `<brand>` — they're editors, not managers). Avoid: "feeds" in user-facing routes/pages (leaks the mechanism).

Standardfeed sources get **no user-facing noun** (not "Publication", not "Standardfeed") — a source is a source. The only user-facing differentiator is a "Subscribe via ATProto" affordance in the picker with a tooltip that leads with the benefit (subscription lives in the user's own account, portable across apps, shares reach the Atmosphere), not the mechanism. RSS candidates carry no label at all. Internally the Tier-2 kind is `standardfeed`.

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
| `app.bsky.graph.follow` | Bluesky | Suggestions only ("people you know from Bluesky"); never auto-mirrored |
| `at.margin.note` | margin.at | Margin annotations rendered alongside articles |
| `at.glean.like` | Glean | Popularity signal (1×); importable as `blue.morgen.feed.save` |
| `at.glean.subscription` | Glean | Importable as `blue.morgen.feed.subscription` |
| `app.skyreader.feed.subscription` | Skyreader | Importable; respected in discovery |
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

All four are first-class citizens in v1, each with a UI optimized for its format.

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
- **Background sweep** re-fetches every feed in the shared Tier-2 catalog on a global timer (`FETCH_INTERVAL_MINUTES`, default 30; `0` disables). It is system-wide, not per-user: it touches no PDS records and creates no jobs, so it never lights up a refresh indicator. Conditional GET (etag/last-modified) keeps re-fetches cheap, and the fetcher's worker pool bounds upstream load.

User-initiated refreshes (manual, add, login) dispatch asynchronously, controllers return immediately. While any job is in flight the digest renders its loading skeleton, and once the job goes quiet the digest re-fetches in place. No count, no badge, no persistent indicator. Consistent with the "no unread counts" anti-feature.

Architecture must permit evolving toward finer real-time refresh per feed (HTTP caching headers, per-feed `next_check_at`, exponential backoff on errors), and toward throttling the background sweep per user (e.g. a bounded number of updates per user per day) instead of re-fetching the whole catalog every tick.

### Failure Handling

Failed fetches back off exponentially (5min → 15min → 1h → 6h → 24h cap) until success or auto-disable. After **20 consecutive failures** the feed is silently muted. Muted feeds still auto-retry once per day. On the first success they silently re-enable, no user action required.

Failure state is **visible only in the sources list** as quiet metadata (last successful fetch time, muted state). The digest itself never surfaces feed errors — the calm-brand promise extends to "no apologies for missing content."

</feed-sources>

---

<social-layer>

## Social Layer (ATProto)

The core differentiator. For each piece of content, the app checks for ATProto backlinks and displays social context alongside it.

### Scope

- **Read:** Show shares (including Standardfeed recommends), Bluesky reposts and margin.at annotations. No like counts, shares are higher signal.
- **Follow:** In-app follows stored as `blue.morgen.graph.follow` records (separate from Bluesky social graph follows).

### UX Principle

Social context is available but not forced. The reading experience comes first. Reactions are opt-in per article — shown only if the user wants to see them.

</social-layer>

---

<sync-architecture>

## Sync Architecture

Two-tier storage with different authority and sharing models.

### Tier 1 — PDS-mirrored (per-user)

User-owned records (`blue.morgen.feed.subscription`, `feed.save`, `feed.share`, `graph.follow`) live authoritatively on the user's PDS. Local SQLite tables holding the same data are **derived indexes only** (per `<lexicons>`). Reconciliation: `listRecords` against PDS, diff against local index, apply changes. Reconciliation triggers are the same as the user-initiated fetch triggers in `<feed-sources>` (login, manual refresh, add); the background sweep is Tier-2 only and never reconciles PDS records.

**Publication sources (Standardfeed).** For sources backed by a `site.standard.publication`, the `site.standard.graph.subscription` record is the **sole existence authority** — "am I subscribed" is answered only by that record. The `blue.morgen.feed.subscription` sidecar (joined by publication AT-URI) carries Morgenblau-only metadata (title, tags, primary) and is created **lazily**, on the user's first metadata edit; subscribing writes only the standard record. This split makes deletes tombstone-free in both directions: an orphaned sidecar (standard record gone — user unsubscribed in another app) is deleted on reconcile; a standard record without a sidecar is a healthy subscription with default metadata (title falls back to the cached `publication.name`). Reconcile reads both collections; its **only** PDS write is deleting an orphaned sidecar (subscription and share sidecars alike) — otherwise it never writes.

The same pattern governs shares of Standardfeed documents: `site.standard.graph.recommend` is the existence record, `blue.morgen.feed.share` (joined by `document` AT-URI) is the lazy sidecar created only when the user writes a comment. "My shares" is derived from the union of both collections; orphaned share sidecars are deleted on reconcile.

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
- Saves carry optional user-defined `tags` (e.g. `read-later`, `favorite`). Flat tag list, no folders or hierarchies. Filtering surfaces through Views.

</saving-sharing>

---

<anti-features>

## Anti-Features

Things Morgenblau will never do.

### Hard No

- **No unread counts.** Never show unread badges, counts, or inbox-zero mechanics. This is the foundational design principle.

</anti-features>

---

<brand>

## Brand

See [BRAND.md](./BRAND.md) for the brand layer: essence, the Edition and Morning ideas, voice, color and light, typography, the mark, and motion.

</brand>
