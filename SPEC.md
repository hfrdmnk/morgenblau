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

**ATProto OAuth only.** Users must log in with an ATProto account. There is no anonymous or email-based access. This unlocks the social layer from day one.

</authentication>

---

<daily-digests>

## Daily Digests

The core consumption model. Content is organized into daily editions rather than a continuous feed.

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

</content-types>

---

<windows>

## Windows

Windows are filtered views onto your content — different lenses to look through.

### Default Windows (Predefined)

The app provides default windows based on content type (e.g., Blogposts, Videos, Podcasts, Microblogs).

### Custom Windows (User-Created)

Users can create their own windows with custom filter criteria — by tags, sources, content types, or combinations.

### Default Landing

When a user opens Morgenblau, they land on **today's digest** — a unified view of the current day's content across all sources. Windows are available for filtering from there.

</windows>

---

<feed-sources>

## Feed Sources

### Adding Sources

Users manually add RSS/Atom feed URLs. No auto-discovery in v1. Each subscription is stored as an `app.skyreader.feed.subscription` record in the user's ATProto repo.

### Organization

Flat list of subscriptions. No folders or categories — Windows handle the filtering/viewing.

### Primary Sources

Users can mark feeds as **primary sources**. These receive prominent placement in the digest — front-page treatment.

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

### Open for Future (Tasteful Only)

- **Notifications** — could see optional edition-ready notifications ("Your morning edition is ready"), but never content-level push notifications
- **Smart ranking** — could consider light curation in the future, but never engagement-based algorithmic sorting

</anti-features>

---

<brand>

## Brand

### Texture

**Crisp morning.** Clear, sharp, awake — not warm and cozy. The terrace on a clear morning, not the candlelit cafe.

- Clean sans-serifs
- Cool blues
- Precise transitions

### Core Metaphors

- **The window** — something you choose to look through, then step away from. It never follows you around. What you see through it is finite and tied to today.
- **The newspaper** — not the layout, but the feeling. A finite object with a clear start and end. A ritual, not a habit.

### Identity

Users aren't "managing subscriptions." They're the **editor of their own daily publication** — choosing sources, deciding who gets the front page.

</brand>

---

<tech-stack>

## Tech Stack (Quick Reference)

| Layer           | Technology                          |
| --------------- | ----------------------------------- |
| Framework       | React 19, TypeScript                |
| Router          | TanStack React Router + React Start |
| Styling         | Tailwind CSS v4                     |
| Animation       | Motion, Three.js (shaders)          |
| Components      | Base UI                             |
| Build           | Vite                                |
| Package Manager | bun                                 |

</tech-stack>
