# Morgenblau Spec

> Single source of truth for product vision, content model, and guardrails.
> Keep high-level. Update when core decisions change, not for every feature.

---

<vision>

## What is Morgenblau?

A calm content platform powered by RSS and ATProto. Not a classic RSS reader — a window into the Atmosphere that organizes content into finite daily digests instead of infinite feeds.

**Core emotional promise:** Intentionality without deprivation. You still get the good stuff, but on your terms.

**Target users:** People who want to consume content (blogs, microblogs, videos, podcasts) without the anxiety of unread counts or the pull of endless scrolling. They value the open web and the ATProto ecosystem.

**What makes it different:**

- Daily digests instead of an unread inbox
- Social layer via ATProto backlinks — RSS becomes interactive
- Four first-class content types with dedicated UIs
- The "editor of your own publication" identity — you curate sources, not manage subscriptions

</vision>

---

<modes>

## Three Modes (Product Roadmap)

| Mode     | Status | Description                                                    |
| -------- | ------ | -------------------------------------------------------------- |
| Consume  | v1     | Daily digests, four content types, social layer, custom player |
| Discover | Future | Find new sources via ATProto social graph, link extraction     |
| Create   | Future | Post to Bluesky (and later long-form via standard.site)        |

**v1 is Consume only.** Discovery and Creation are future modes.

</modes>

---

<platform>

## Platform

**v1 is web-only.** No native apps, no PWA. Browser-first experience.

</platform>

---

<authentication>

## Authentication

ATProto OAuth is the only auth mechanism — no passwords, email, or registration. The user row stores `did` (primary key), encrypted `refresh_token`, and `iss` (auth server URL). Handle is resolved at login and lives in the Laravel session, never the DB. Profile data (avatar, display name) is re-fetched live from the PDS when needed.

Scopes: granular `repo:app.skyreader.*` per-collection, following [Dan Abramov's guidance](https://underreacted.leaflet.pub/3mjfozhlhys2z). Avoid `transition:generic`.

Client metadata is served at `/oauth-client-metadata.json`, JWKS at `/oauth-jwks.json`, callback at `/oauth/callback` (route name `bluesky.oauth.redirect`, package convention).

References:
- [ATProto OAuth](https://atproto.com/specs/oauth), [ATProto permissions](https://atproto.com/specs/permission)

</authentication>

---

<daily-digests>

## Daily Digests

The core consumption model. Content is **grouped by day** into daily editions rather than presented as a continuous feed. "Daily" describes the *grouping*, not the *fetch cadence* — fetch cadence is governed by [Refresh Cadence](#refresh-cadence) under Feed Sources.

### Empty Editions

An empty edition is a feature, not a bug. Display a simple, calm message. No nudges, no guilt. Example: _"Nothing new this morning. Enjoy your coffee."_

### History

Rolling window of past digests (exact retention TBD, roughly 30 days). Older content fades away — reinforces the daily mindset.

### No Read Tracking

No read state. No progress indicators. No "you've seen 8 of 12." Each edition simply exists. Content is not marked, dimmed, or tracked.

</daily-digests>

---

<content-types>

## Four Content Types

All four are first-class citizens in v1, each with a UI optimized for its format.

| Type      | Description                        | Playback                 |
| --------- | ---------------------------------- | ------------------------ |
| Blogpost  | Articles with titles and body text | In-app reader + link out |
| Microblog | Short posts without a title        | Inline in digest         |
| Video     | YouTube, Vimeo, etc.               | Custom player            |
| Podcast   | Audio feeds                        | Custom player            |

### Reading Mode

In-app reader by default — fetch and render article content directly. Users can always open the original URL. Both options available.

### Media Playback

Custom video and audio player UI that matches Morgenblau's design language. Not YouTube iframes or bare HTML audio elements.

### Classification & Sanitization

Content type is **classified at fetch time and persisted** — entries land in storage with a `content_type` column already set, not derived at render. Same applies to HTML sanitization for the in-app reader: sanitize once during the fetch pipeline, store the safe form, never sanitize at render.

Type-specific metadata (reading-time, YouTube video id, podcast enclosure + duration, etc.) lives in a `metadata` JSON column on `feed_entries`. Fields get promoted to typed columns only when their content-type UI ships and the access patterns are known.

</content-types>

---

<views>

## Views

Views are filters of content, different lenses to look through.

### Default Views (Predefined)

The app provides default views based on content type (e.g., Blogposts, Videos, Podcasts, Microblogs).

### Custom Views (User-Created)

Users can create their own views with custom filter criteria — by tags, sources, content types, or combinations.

### Default Landing

When a user opens Morgenblau, they land on **today's digest** — a unified view of the current day's content across all sources. Views are available for filtering from there.

</views>

---

<feed-sources>

## Feed Sources

### Adding Sources

Users add sources by pasting a URL. Morgenblau resolves the URL into one or more feeds: it follows `<link rel="alternate" type="application/rss+xml">` (and Atom equivalents) on HTML pages, maps YouTube channel / `@handle` / `/c/` / `/user/` URLs to the corresponding `feeds/videos.xml`, and resolves Apple Podcasts URLs to the show's RSS feed via the iTunes lookup API. Each subscription is stored as an `app.skyreader.feed.subscription` record in the user's ATProto repo.

### Organization

Flat list of subscriptions. Windows handle the filtering/viewing.

### Primary Sources

Users can mark feeds as **primary sources**. These receive prominent placement in the digest — front-page treatment.

### Refresh Cadence

Refresh has **exactly three triggers**:

- **Auto-refresh** every 30 minutes for all active subscriptions.
- **Manual refresh** is available on the digest view.
- **On subscription add**, the new subscription is fetched immediately (only that feed, not the whole set).

Notably absent: refresh on digest visit. The window metaphor is "step away, come back, content has accumulated on its own clock" — opening the digest does not trigger fetches.

Architecture must permit evolving toward finer real-time refresh per feed (HTTP caching headers, per-feed `next_check_at`, exponential backoff on errors). The 30-minute default is a product choice, not an architectural ceiling.

### Failure Handling

Failed fetches back off exponentially (5min → 15min → 1h → 6h → 24h cap) until success or auto-disable. After **20 consecutive failures** the feed is silently muted — no toasts, no banners, no digest-side nudges. Muted feeds still auto-retry once per day; on the first success they silently re-enable, no user action required.

Failure state is **visible only in the subscription list** as quiet metadata (last successful fetch time, muted state). The digest itself never surfaces feed errors — the calm-brand promise extends to "no apologies for missing content."

</feed-sources>

---

<social-layer>

## Social Layer (ATProto)

The core differentiator. For each piece of content, the app checks for ATProto backlinks and displays social context alongside it.

### v1 Scope

- **Read:** Show Bluesky likes, reposts, and reply threads found via backlinks
- **Like:** Users can like content from within Morgenblau
- **Follow:** In-app follows stored as `app.skyreader.social.follow` records (separate from Bluesky social graph follows)
- No reposting, replying, or other interactions in v1

### UX Principle

Social context is available but not forced. The reading experience comes first. Reactions are opt-in per article — shown only if the user wants to see them.

</social-layer>

---

<atproto-lexicons>

## ATProto Lexicons

Morgenblau uses [Skyreader's](https://github.com/disnet/skyreader) lexicons (`app.skyreader.*`) for all user data stored in ATProto repos. This enables interoperability — data written by Morgenblau can be read by Skyreader and vice versa.

Vendored lexicon schemas live in `lexicons/app/skyreader/`.

Any local database tables that mirror PDS-resident data (e.g. a local `subscriptions` index for fast joins) are **derived indexes only** — reconciled from PDS reads, never authoritative. User-owned state always belongs in a lexicon record, not a local column.

| Feature            | NSID                              | Schema                                          |
| ------------------ | --------------------------------- | ----------------------------------------------- |
| Feed subscriptions | `app.skyreader.feed.subscription` | `lexicons/app/skyreader/feed/subscription.json` |
| Saved articles     | `app.skyreader.feed.saved`        | `lexicons/app/skyreader/feed/saved.json`        |
| Shared articles    | `app.skyreader.social.share`      | `lexicons/app/skyreader/social/share.json`      |
| In-app follows     | `app.skyreader.social.follow`     | `lexicons/app/skyreader/social/follow.json`     |

</atproto-lexicons>

---

<saving-sharing>

## Saving & Sharing

Simple and minimal.

- Users can **save** individual articles to a separate saved-items view — stored as `app.skyreader.feed.saved` records
- Users can **share** articles with optional commentary — stored as `app.skyreader.social.share` records
- No folders, tags, or organization for saved content — just a list

</saving-sharing>

---

<navigation>

## Navigation

### Day Navigation

**Calendar strip** — horizontal strip of days. Tap a day to see its digest.

### Digest Layout

**Vertical feed** — simple top-to-bottom scroll of cards. Clean and predictable. Primary sources may receive larger or more prominent cards.

</navigation>

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

**Crisp morning.** Clear, sharp, awake — not warm and cozy. The terrace on a clear morning, not the candlelit cafe.

### Core Metaphors

- **The window** — something you choose to look through, then step away from. It never follows you around. What you see through it is finite and tied to today.
- **The newspaper** — not the layout, but the feeling. A finite object with a clear start and end. A ritual, not a habit.

### Identity

Users aren't "managing subscriptions." They're the **editor of their own daily publication** — choosing sources, deciding who gets the front page.

</brand>
