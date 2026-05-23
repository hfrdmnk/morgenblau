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

</terminology>

---

<lexicons>

## Lexicons

### Why Our Own Lexicon

Morgenblau writes all user-owned records under `blue.morgen.*`. Skyreader's schemas are too verbose for our purpose: they denormalize metadata like `author`, `excerpt`, `image`, and `wordCount` that we'd rather look up from our Tier-2 cache. Glean's overlap is narrow: no share concept, plus `rating` and `quote` dimensions on annotations that we don't model.

Owning the schema lets us stay minimal and move fast while still interoperating where it earns its keep. We import existing subscriptions and saves from other readers (user can choose to do so), and count cross-reader signals in discovery. The end goal is a shared standard between RSS readers in the atmosphere. Until that exists, `blue.morgen.*` is our contribution to the conversation.

### Why a Separate Follow Graph

A Bluesky follow means "I want this person's posts in my timeline." A Morgenblau follow means "I trust this person's reading taste." Morgenblau leans slower and more curatorial than Bluesky's fast-paced feed, so the same word carries different intent in each app.

Bluesky follows are never auto-mirrored as Morgenblau follows. They surface only as discovery suggestions ("people you know from Bluesky") on the Discover route, which solves cold start without conflating the two signals.

Skyreader's follow records are importable. The user's own `blue.morgen.graph.follow` records remain authoritative for their personal follow graph.

### Our Records

| NSID | Purpose |
|:--|:--|
| `blue.morgen.feed.subscription` | A user's curated feed source |
| `blue.morgen.feed.save` | Saved item for later reading (private) |
| `blue.morgen.feed.share` | Broadcast item (public "I like this") |
| `blue.morgen.graph.follow` | In-app follow, distinct from Bluesky's |

All four use `tid` rkeys, and all require `createdAt`. Every other field is optional. The record carries identity and intent. The Tier-2 cache renders the rest (see `<sync-architecture>`).

#### `blue.morgen.feed.subscription`

```json
{
  "lexicon": 1,
  "id": "blue.morgen.feed.subscription",
  "defs": {
    "main": {
      "type": "record",
      "description": "A subscription to an RSS/Atom feed.",
      "key": "tid",
      "record": {
        "type": "object",
        "required": ["feedUrl", "createdAt"],
        "properties": {
          "feedUrl": {
            "type": "string",
            "format": "uri",
            "maxLength": 2048,
            "description": "URL of the RSS/Atom feed."
          },
          "title": {
            "type": "string",
            "maxGraphemes": 512,
            "description": "Display title. Auto-prefilled from the feed, user-editable."
          },
          "siteUrl": {
            "type": "string",
            "format": "uri",
            "maxLength": 2048,
            "description": "Human-facing site URL associated with the feed."
          },
          "tags": {
            "type": "array",
            "maxLength": 10,
            "items": { "type": "string", "maxGraphemes": 64 },
            "description": "User-defined tags."
          },
          "primary": {
            "type": "boolean",
            "description": "Whether the feed receives special treatment in the digest."
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
            "description": "User's note on why they saved it."
          },
          "tags": {
            "type": "array",
            "maxLength": 10,
            "items": { "type": "string", "maxGraphemes": 64 },
            "description": "User-defined tags (e.g. 'read-later', 'favorite')."
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
  "defs": {
    "main": {
      "type": "record",
      "description": "A shared feed item. Broadcasts 'I like this' with optional commentary.",
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
          "feedUrl": {
            "type": "string",
            "format": "uri",
            "maxLength": 2048,
            "description": "URL of the source feed (optional provenance)."
          },
          "comment": {
            "type": "string",
            "maxGraphemes": 3000,
            "description": "User's commentary on the share."
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

### External Lexicons We Read

| NSID | Source | How we use it |
|:--|:--|:--|
| `app.bsky.graph.follow` | Bluesky | Suggestions only ("people you know from Bluesky"); never auto-mirrored |
| `at.margin.note` | margin.at | Margin annotations rendered alongside articles |
| `at.glean.like` | Glean | Popularity signal (1×); importable as `blue.morgen.feed.save` |
| `at.glean.subscription` | Glean | Importable as `blue.morgen.feed.subscription` |
| `app.skyreader.feed.subscription` | Skyreader | Importable; respected in discovery |
| `app.skyreader.feed.saved` | Skyreader | Popularity signal (1×); importable as `blue.morgen.feed.save` |
| `app.skyreader.social.share` | Skyreader | Popularity signal (1.5×); importable as `blue.morgen.feed.share` |
| `app.skyreader.social.follow` | Skyreader | Importable, never auto-mirrored |

**Popularity weighting:** saves count 1×, shares 1.5×. Equivalent records on Skyreader and Glean are counted at the same weights. Glean's `annotation` (note, quote, rating) is not consumed. Annotations are delegated to margin.at.

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

Scopes: granular `repo:blue.morgen.*` per-collection, following [Dan Abramov's guidance](https://underreacted.leaflet.pub/3mjfozhlhys2z). Avoid `transition:generic`.

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

Flat list of subscriptions. Windows handle the filtering/viewing.

### Primary Sources

Users can mark feeds as **primary sources**. These receive prominent placement in the digest — front-page treatment.

### Refresh Cadence

Refresh has **exactly three triggers**:

- **Manual refresh** is available on the digest view.
- **On subscription add**, the new subscription is fetched immediately (only that feed, not the whole set).
- **On login**, all of the user's subscriptions are refreshed (behaves like manual refresh), subject to a 5-minute in-flight guard so repeated logins don't thrash upstream feeds.

User-initiated refreshes (manual, add, login) dispatch asynchronously, controllers return immediately. While any job is in flight the digest renders its loading skeleton, and once the job goes quiet the digest re-fetches in place. No count, no badge, no persistent indicator. Consistent with the "no unread counts" anti-feature.

Architecture must permit evolving toward finer real-time refresh per feed (HTTP caching headers, per-feed `next_check_at`, exponential backoff on errors).

### Failure Handling

Failed fetches back off exponentially (5min → 15min → 1h → 6h → 24h cap) until success or auto-disable. After **20 consecutive failures** the feed is silently muted. Muted feeds still auto-retry once per day. On the first success they silently re-enable, no user action required.

Failure state is **visible only in the sources list** as quiet metadata (last successful fetch time, muted state). The digest itself never surfaces feed errors — the calm-brand promise extends to "no apologies for missing content."

</feed-sources>

---

<social-layer>

## Social Layer (ATProto)

The core differentiator. For each piece of content, the app checks for ATProto backlinks and displays social context alongside it.

### Scope

- **Read:** Show shares, Bluesky reposts and margin.at annotations. No like counts, shares are higher signal.
- **Follow:** In-app follows stored as `blue.morgen.graph.follow` records (separate from Bluesky social graph follows).

### UX Principle

Social context is available but not forced. The reading experience comes first. Reactions are opt-in per article — shown only if the user wants to see them.

</social-layer>

---

<sync-architecture>

## Sync Architecture

Two-tier storage with different authority and sharing models.

### Tier 1 — PDS-mirrored (per-user)

User-owned records (`blue.morgen.feed.subscription`, `feed.save`, `feed.share`, `graph.follow`) live authoritatively on the user's PDS. Local SQLite tables holding the same data are **derived indexes only** (per `<lexicons>`). Reconciliation: `listRecords` against PDS, diff against local index, apply changes. Reconciliation triggers are the same as fetch triggers in `<feed-sources>` (login, manual refresh, add).

### Tier 2 — Upstream cache (shared, global)

Parsed feed metadata (etag, last-modified, title, etc.) and parsed entries are cached in local SQLite, **deduped by canonical feed URL across all Morgenblau users**. One `feeds` row per URL; many Tier-1 subscriptions can point at it. The fetcher polls each upstream feed once regardless of how many users subscribe.

Local is canonical for Tier 2 — there is no PDS path for entries, only upstream RSS/Atom. Cache invalidation follows HTTP semantics (etag, last-modified, conditional GET).

### Privacy posture

Cross-user dedup means *the existence of a feed URL in our cache* implies "at least one Morgenblau user subscribes to it." Subscription lists themselves remain per-user (in PDS). No user can enumerate which other users subscribe to a given feed via Morgenblau's API. To be documented in any future privacy/data policy.

</sync-architecture>

---

<saving-sharing>

## Saving & Sharing

Simple and minimal.

- Users can **save** individual articles to a separate saved-items view — stored as `blue.morgen.feed.save` records
- Users can **share** articles with optional commentary — stored as `blue.morgen.feed.share` records
- No folders, tags, or organization for saved content — just a list

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

### Texture

**Crisp morning.** Clear, sharp, awake, not warm and cozy. The terrace on a clear morning, not the candlelit cafe.

### Core Metaphors

- **The window**: something you choose to look through, then step away from. It never follows you around. What you see through it is finite and tied to today.
- **The newspaper**: not the layout, but the feeling. A finite object with a clear start and end. A ritual, not a habit.

</brand>
