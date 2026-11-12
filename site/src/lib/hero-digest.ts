import { decodeHTML } from 'entities';
import { XMLParser } from 'fast-xml-parser';
import sharp from 'sharp';

// Dominik's account: the hero shows his digest, read the way Morgenblau reads it, from his PDS and the feeds it names.
const ACCOUNT = 'did:plc:fthx2gjakdj4ynxxu5vysjty';
const SUBSCRIPTION = 'blue.morgen.feed.subscription';
const USER_AGENT = 'Mozilla/5.0 (compatible; Morgenblau/0.1; +https://morgen.blue)';
const ZONE = 'Europe/Zurich';
const FETCH_TIMEOUT_MS = 10_000;
const MAX_ICON_BYTES = 256 * 1024;

export type HeroSource = { title: string | null; siteUrl: string | null; feedUrl: string; favicon: string | null; primary: boolean };
export type HeroEntry = { title: string | null; url: string | null; excerpt: string | null; image: string | null };
export type HeroGroup = { source: HeroSource; lead: HeroEntry; more: number };
// day is the digest's calendar date at UTC midnight, so it formats in UTC.
export type HeroDigest = { day: Date; avatar: string | null; primary: HeroGroup[]; rest: HeroGroup[] };

type Subscription = { feedUrl: string; siteUrl: string | null; title: string | null; primary: boolean };
type FeedItem = { title: string | null; url: string | null; publishedAt: Date; body: string; image: string | null };

const parser = new XMLParser({
    ignoreAttributes: false,
    attributeNamePrefix: '@',
    parseTagValue: false,
    htmlEntities: true,
    isArray: (name) => ['item', 'entry', 'link', 'enclosure', 'media:content'].includes(name),
});

let digest: Promise<HeroDigest> | undefined;

// The build reads it once; the memo keeps astro dev from refetching every feed on each reload.
export function heroDigest(): Promise<HeroDigest> {
    return (digest ??= loadHeroDigest(new Date()));
}

async function loadHeroDigest(now: Date): Promise<HeroDigest> {
    const { day, start, end } = zurichDay(now);
    const pds = await resolvePDS(ACCOUNT);
    const [subscriptions, avatar] = await Promise.all([listSubscriptions(pds), loadAvatar(pds)]);
    const feeds = await mapPool(subscriptions, 8, async (subscription) => {
        const items = await fetchFeed(subscription.feedUrl).catch(() => []);
        return items.filter(({ publishedAt }) => publishedAt >= start && publishedAt < end);
    });
    const followed = subscriptions.map((subscription, index) => ({ subscription, items: feeds[index] })).filter(({ items }) => items.length > 0);
    const sources = await mapPool(followed, 8, async ({ subscription }) => ({ ...subscription, favicon: await loadFavicon(subscription.siteUrl ?? subscription.feedUrl) }));
    // The app lists posts newest first and breaks publish-time ties by row id, newest first. PDS listings start with the newest subscription, and a feed stores its items in document order, so each feed's items go in reversed before the stable sort.
    const posts = followed
        .flatMap(({ items }, index) => items.toReversed().map((item) => ({ source: sources[index], item })))
        .sort((a, b) => b.item.publishedAt.getTime() - a.item.publishedAt.getTime());
    // A busy source keeps its latest post in place and folds the rest beneath it, as in the app.
    const groups = new Map<HeroSource, { source: HeroSource; lead: FeedItem; more: number }>();
    for (const { source, item } of posts) {
        const group = groups.get(source);
        if (group) group.more += 1;
        else groups.set(source, { source, lead: item, more: 0 });
    }
    const all = await mapPool([...groups.values()], 8, async ({ source, lead, more }) => ({
        source,
        more,
        lead: {
            title: lead.title,
            url: lead.url,
            // Only primary posts show an excerpt and an image in the digest.
            excerpt: source.primary ? excerpt(lead.body) || null : null,
            image: source.primary && lead.image ? await loadImage(lead.image) : null,
        },
    }));
    return { day, avatar, primary: all.filter(({ source }) => source.primary), rest: all.filter(({ source }) => !source.primary) };
}

// A digest is one day of its reader's calendar; the hero shows Dominik's, so it is the day in Zurich when the site is built.
function zurichDay(now: Date) {
    const parts = new Intl.DateTimeFormat('en-US', { timeZone: ZONE, year: 'numeric', month: 'numeric', day: 'numeric' }).formatToParts(now);
    const part = (type: string) => Number(parts.find((p) => p.type === type)!.value);
    const [year, month, date] = [part('year'), part('month') - 1, part('day')];
    return { day: new Date(Date.UTC(year, month, date)), start: zurichMidnight(year, month, date), end: zurichMidnight(year, month, date + 1) };
}

// Zurich changes its clocks at 02:00 or 03:00, so the offset in force at midnight UTC is the one at Zurich's own midnight.
function zurichMidnight(year: number, month: number, date: number): Date {
    const wall = Date.UTC(year, month, date);
    const name = new Intl.DateTimeFormat('en-US', { timeZone: ZONE, timeZoneName: 'longOffset' }).formatToParts(wall).find((p) => p.type === 'timeZoneName')!.value;
    const [, sign, hours, minutes] = /GMT([+-])(\d\d):(\d\d)/.exec(name)!;
    return new Date(wall - (sign === '-' ? -1 : 1) * (Number(hours) * 60 + Number(minutes)) * 60_000);
}

async function resolvePDS(did: string): Promise<string> {
    const doc = await (await get(`https://plc.directory/${did}`, 'application/json')).json();
    const service = doc.service?.find((s: { id: string }) => s.id === '#atproto_pds');
    if (!service) throw new Error(`no PDS for ${did}`);
    return service.serviceEndpoint;
}

async function listSubscriptions(pds: string): Promise<Subscription[]> {
    const subscriptions: Subscription[] = [];
    let cursor = '';
    do {
        const page = await (await get(`${pds}/xrpc/com.atproto.repo.listRecords?repo=${ACCOUNT}&collection=${SUBSCRIPTION}&limit=100&cursor=${cursor}`, 'application/json')).json();
        for (const { value } of page.records) {
            // Standardfeed sidecars carry no feed URL; the account follows none, so they are left out.
            if (value.source?.$type !== `${SUBSCRIPTION}#rssFeed`) continue;
            subscriptions.push({ feedUrl: value.source.feedUrl, siteUrl: value.source.siteUrl ?? null, title: value.title || null, primary: value.primary === true });
        }
        cursor = page.cursor ?? '';
    } while (cursor);
    return subscriptions;
}

async function loadAvatar(pds: string): Promise<string | null> {
    const profile = await (await get(`${pds}/xrpc/com.atproto.repo.getRecord?repo=${ACCOUNT}&collection=app.bsky.actor.profile&rkey=self`, 'application/json')).json();
    const cid = profile.value?.avatar?.ref?.$link;
    if (!cid) return null;
    const blob = await bytes(await get(`${pds}/xrpc/com.atproto.sync.getBlob?did=${ACCOUNT}&cid=${cid}`, 'image/*'));
    return dataURI('image/webp', await sharp(blob).resize(80, 80).webp().toBuffer());
}

async function fetchFeed(feedUrl: string): Promise<FeedItem[]> {
    const doc = parser.parse(await (await get(feedUrl, 'application/rss+xml, application/atom+xml, application/xml, text/xml')).text());
    if (doc.feed) return (doc.feed.entry ?? []).flatMap(atomItem);
    return (doc.rss?.channel?.item ?? doc['rdf:RDF']?.item ?? []).flatMap(rssItem);
}

type Node = Record<string, any>;

function text(value: unknown): string {
    if (typeof value === 'string') return value.trim();
    if (value && typeof value === 'object' && '#text' in value) return String(value['#text']).trim();
    return '';
}

// Undated posts never reach a digest, so they are dropped here.
function feedItem(title: string, url: string | null, date: string, body: string, image: string | null): FeedItem[] {
    const publishedAt = new Date(date);
    if (!date || Number.isNaN(publishedAt.getTime())) return [];
    return [{ title: title || null, url, publishedAt, body, image: image && URL.canParse(image, url ?? undefined) ? new URL(image, url ?? undefined).href : null }];
}

// The image order follows gofeed's RSS translator, which the app's feed pipeline uses.
function rssItem(item: Node): FeedItem[] {
    const content = text(item['content:encoded']);
    const description = text(item.description);
    const enclosure = item.enclosure?.find((e: Node) => String(e['@type']).startsWith('image/'));
    const media = item['media:content']?.find((m: Node) => /image/.test(`${m['@type']} ${m['@medium']}`));
    const image = item['itunes:image']?.['@href'] ?? media?.['@url'] ?? enclosure?.['@url'] ?? firstImage(content) ?? firstImage(description);
    return feedItem(text(item.title), text(item.link?.[0]) || null, text(item.pubDate) || text(item['dc:date']), content || description, image ?? null);
}

// gofeed gives Atom entries no image, so neither does the app.
function atomItem(entry: Node): FeedItem[] {
    const link = entry.link?.find((l: Node) => !l['@rel'] || l['@rel'] === 'alternate');
    return feedItem(text(entry.title), link?.['@href'] ?? null, text(entry.published) || text(entry.updated), text(entry.content) || text(entry.summary), null);
}

function firstImage(html: string): string | null {
    const src = /<img\b[^>]*?\bsrc\s*=\s*["']([^"']+)["']/i.exec(html)?.[1];
    return src ? decodeHTML(src) : null;
}

// Mirrors entryExcerpt in the app: block elements end in a space, then the text is collapsed and cut at 300 characters.
function excerpt(body: string): string {
    // Some feeds escape their HTML inside CDATA, which the XML parser leaves as entities.
    const html = !body.includes('<') && body.includes('&lt;') ? decodeHTML(body) : body;
    const textOnly = html
        .replace(/<(script|style)\b[\s\S]*?<\/\1>/gi, '')
        .replace(/<\/(p|div|li|h[1-6]|blockquote|figcaption)>|<br\b[^>]*>/gi, '$& ')
        .replace(/<[^>]*>/g, '');
    return decodeHTML(textOnly).replace(/\s+/g, ' ').trim().slice(0, 300);
}

type IconCandidate = { href: string; svg: boolean; size: number; weight: number };

const REL_WEIGHT: Record<string, number> = { 'apple-touch-icon-precomposed': 40, 'apple-touch-icon': 30, icon: 20 };

// The ranking follows favicon.Discover in the app: SVG first, then the largest declared size, then the rel.
async function loadFavicon(siteUrl: string): Promise<string | null> {
    const candidates: IconCandidate[] = [];
    let base = new URL(siteUrl);
    try {
        const page = await get(siteUrl, 'text/html,application/xhtml+xml');
        base = new URL(page.url);
        for (const tag of (await page.text()).slice(0, MAX_ICON_BYTES).match(/<link\b[^>]*>/gi) ?? []) {
            const attrs = Object.fromEntries([...tag.matchAll(/([\w-]+)\s*=\s*(?:"([^"]*)"|'([^']*)'|([^\s>]+))/g)].map((m) => [m[1].toLowerCase(), m[2] ?? m[3] ?? m[4]]));
            const rel = String(attrs.rel ?? '').toLowerCase().split(/\s+/).find((token) => token in REL_WEIGHT);
            if (!rel || !attrs.href || attrs.href.startsWith('data:') || !URL.canParse(decodeHTML(attrs.href), base)) continue;
            const href = new URL(decodeHTML(attrs.href), base).href;
            const size = Math.max(0, ...String(attrs.sizes ?? '').split(/\s+/).flatMap((s) => s.toLowerCase().split('x').map(Number)).filter(Number.isFinite));
            candidates.push({ href, svg: attrs.type === 'image/svg+xml' || /\.svg($|\?)/i.test(href), size, weight: REL_WEIGHT[rel] });
        }
    } catch {
        // A site that does not answer still gets its /favicon.ico tried.
    }
    candidates.sort((a, b) => Number(b.svg) - Number(a.svg) || b.size - a.size || b.weight - a.weight);
    candidates.push({ href: new URL('/favicon.ico', base).href, svg: false, size: 0, weight: 0 });
    for (const candidate of candidates) {
        try {
            const body = await bytes(await get(candidate.href, 'image/*'), MAX_ICON_BYTES);
            const icon = isICO(body) ? icoFrame(body) : body;
            if (isICO(icon)) return dataURI('image/x-icon', icon);
            return dataURI('image/png', await sharp(icon).resize(32, 32).png().toBuffer());
        } catch {
            continue;
        }
    }
    return null;
}

function isICO(body: Buffer): boolean {
    return body.length > 6 && body.readUInt32BE(0) === 0x00000100;
}

// An ICO bundles many sizes and sharp reads none of them, so this keeps the frame nearest 32px: its PNG as is, or a bitmap frame as a one-frame ICO.
function icoFrame(ico: Buffer): Buffer {
    const frames = Array.from({ length: ico.readUInt16LE(4) }, (_, index) => {
        const at = 6 + index * 16;
        return { at, width: ico[at] || 256, length: ico.readUInt32LE(at + 8), offset: ico.readUInt32LE(at + 12) };
    });
    const frame = frames.sort((a, b) => Math.abs(a.width - 32) - Math.abs(b.width - 32) || b.width - a.width)[0];
    const data = ico.subarray(frame.offset, frame.offset + frame.length);
    if (data.subarray(1, 4).toString() === 'PNG') return data;
    const header = Buffer.alloc(22);
    header.writeUInt32BE(0x00000100, 0);
    header.writeUInt16LE(1, 4);
    ico.copy(header, 6, frame.at, frame.at + 12);
    header.writeUInt32LE(22, 18);
    return Buffer.concat([header, data]);
}

async function loadImage(url: string): Promise<string | null> {
    try {
        const body = await bytes(await get(url, 'image/*'));
        return dataURI('image/webp', await sharp(body).resize({ width: 480, withoutEnlargement: true }).webp({ quality: 72 }).toBuffer());
    } catch {
        return null;
    }
}

// Feed content names these URLs, so only public web hosts are fetched, never this machine's own network.
function isPublicURL(url: URL): boolean {
    const host = url.hostname;
    return /^https?:$/.test(url.protocol) && host.includes('.') && !host.endsWith('.internal') && !host.endsWith('.local') && !/^[\d.]+$|:/.test(host);
}

async function get(url: string, accept: string): Promise<Response> {
    if (!isPublicURL(new URL(url))) throw new Error(`refusing ${url}`);
    const response = await fetch(url, { headers: { 'user-agent': USER_AGENT, accept }, signal: AbortSignal.timeout(FETCH_TIMEOUT_MS) });
    if (!response.ok || !isPublicURL(new URL(response.url))) throw new Error(`${url}: ${response.status}`);
    return response;
}

async function bytes(response: Response, max = 8 * 1024 * 1024): Promise<Buffer> {
    const body = Buffer.from(await response.arrayBuffer());
    if (body.length > max) throw new Error(`${response.url}: ${body.length} bytes`);
    return body;
}

function dataURI(type: string, body: Buffer): string {
    return `data:${type};base64,${body.toString('base64')}`;
}

async function mapPool<T, R>(items: T[], limit: number, run: (item: T, index: number) => Promise<R>): Promise<R[]> {
    const results: R[] = new Array(items.length);
    let next = 0;
    await Promise.all(Array.from({ length: limit }, async () => {
        while (next < items.length) {
            const index = next++;
            results[index] = await run(items[index], index);
        }
    }));
    return results;
}
