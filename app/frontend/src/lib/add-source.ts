import { api, ApiError } from './api';

export const subscriptionChanges = new EventTarget();

export type SourceCandidate = {
    title?: string;
    siteUrl?: string;
    subscribedVia?: { kind: string; title?: string };
} & (
    | { kind?: '' | 'rss'; feedUrl: string; publication?: never }
    | { kind: 'standardfeed'; publication: string; feedUrl?: never }
);

export type SourceDiscovery = {
    candidates: SourceCandidate[];
    existingSubscriptions: { feedUrl: string; rkey: string; title: string | null }[];
};

export function sourceKey(candidate: SourceCandidate) {
    return candidate.publication ?? candidate.feedUrl;
}

export function sourceTitle(candidate: SourceCandidate) {
    return candidate.title || candidate.siteUrl || sourceKey(candidate);
}

export function sourceLocation(candidate: SourceCandidate) {
    return candidate.feedUrl || candidate.siteUrl || sourceKey(candidate);
}

const YOUTUBE_FEED = 'https://www.youtube.com/feeds/videos.xml';

// Mirrors SameYouTubeFeedURLs in internal/feedfinder: a channel's full feed and its UULF uploads playlist, which leaves out Shorts.
export function youtubeFeeds(feedUrl: string) {
    let url: URL;
    try {
        url = new URL(feedUrl);
    } catch {
        return undefined;
    }
    if (!['youtube.com', 'www.youtube.com'].includes(url.hostname) || url.pathname !== '/feeds/videos.xml') return undefined;
    const id =
        /^UC([\w-]{22})$/.exec(url.searchParams.get('channel_id') ?? '')?.[1] ??
        /^UULF([\w-]{22})$/.exec(url.searchParams.get('playlist_id') ?? '')?.[1];
    if (!id) return undefined;
    return { all: `${YOUTUBE_FEED}?channel_id=UC${id}`, longForm: `${YOUTUBE_FEED}?playlist_id=UULF${id}` };
}

export function discoveryError(error: unknown) {
    return error instanceof Error && !(error instanceof TypeError)
        ? error.message
        : 'Couldn’t look for feeds. Try again.';
}

function webURL(input: string) {
    const trimmed = input.trim();
    let url: URL;
    try {
        url = new URL(
            /^[a-z][a-z\d+.-]*:/i.test(trimmed) ? trimmed : `https://${trimmed}`,
        );
    } catch {
        return undefined;
    }
    return ['https:', 'http:'].includes(url.protocol) &&
        !url.username &&
        !url.password
        ? url.href
        : undefined;
}

export function discoverSources(input: string, signal: AbortSignal) {
    const url = webURL(input);
    if (!url) return Promise.reject(new Error('Enter a website or feed URL.'));
    return api<SourceDiscovery>('/api/subscriptions/resolve', {
        method: 'POST',
        body: { url },
        signal,
    });
}

export type GeneratedHealth =
    | 'ok'
    | 'needs_attention'
    | 'blocked_by_site'
    | 'broken';

export type AnchorRejection = 'not_on_page' | 'not_confirmed' | 'no_better';

export type GeneratedSource = {
    id: string;
    feedUrl: string;
    title: string;
    siteUrl: string;
    health: GeneratedHealth;
    healthReason: string;
    needsAnchor: boolean;
    items: { url: string; title: string; date: string | null }[];
    itemCount: number;
    anchorRejected?: AnchorRejection;
};

export function generateSource(
    input: string,
    anchor: string | undefined,
    signal: AbortSignal,
) {
    const url = webURL(input);
    if (!url) return Promise.reject(new Error('Enter a website URL.'));
    const anchorUrl = anchor === undefined ? undefined : webURL(anchor);
    if (anchor !== undefined && !anchorUrl)
        return Promise.reject(
            new Error('Enter the address of one post on that page.'),
        );
    return api<GeneratedSource>('/api/web-sources', {
        method: 'POST',
        body: anchorUrl ? { url, anchorUrl } : { url },
        signal,
    });
}

const RATE_LIMITED = 'You’ve generated a lot of feeds recently. Try again later.';
const SETUP_IN_FLIGHT =
    'Your last feed is still being generated. Try again in a moment.';

// The reader's own earlier setup is still spending, so this clears in seconds unlike a limit.
function setupInFlight(error: ApiError) {
    return error.code === 'setup_in_flight';
}

function limitMessage(error: ApiError) {
    return setupInFlight(error) ? SETUP_IN_FLIGHT : RATE_LIMITED;
}

const CODE_MESSAGES: Record<string, string> = {
    browser_built:
        'This site builds its post list in the browser, so Morgenblau can’t follow it yet.',
};

const STATUS_MESSAGES: Record<number, string> = {
    400: 'Enter a website URL.',
    502: 'Couldn’t read that page.',
    503: 'Feed generation isn’t available right now.',
};

function apiGenerationError(error: ApiError) {
    if (error.status === 429 || error.code === 'rate_limited')
        return limitMessage(error);
    const known = CODE_MESSAGES[error.code ?? ''] ?? STATUS_MESSAGES[error.status];
    if (known) return known;
    return error.status < 500 ? error.message : undefined;
}

// Server messages are terse logs, so every failure reads in the dialog's own words.
export function generationError(error: unknown) {
    const api = error instanceof ApiError ? apiGenerationError(error) : undefined;
    if (api) return api;
    return error instanceof Error &&
        !(error instanceof TypeError) &&
        !(error instanceof ApiError)
        ? error.message
        : 'Couldn’t generate a feed. Try again.';
}

// Only a bad address is the reader's to fix; a busy or unreachable server is not.
export function anchorError(error: unknown) {
    const invalid =
        error instanceof ApiError
            ? error.status === 400
            : !(error instanceof TypeError);
    return {
        message:
            error instanceof ApiError && invalid
                ? 'Enter the address of one post on that page.'
                : generationError(error),
        invalid,
    };
}

// The post was a fair try, so this reads as an outcome rather than the reader's mistake.
export function anchorRejection(why: AnchorRejection) {
    return why === 'not_on_page'
        ? 'That post isn’t linked on this page. The feed stays as it was.'
        : 'That post didn’t help. The feed stays as it was.';
}

// A long limit, an unavailable generator or a bad address answers the same way at once.
export function canRetryGeneration(error: unknown) {
    if (error instanceof ApiError)
        return error.status === 429
            ? setupInFlight(error)
            : error.status >= 500 && error.status !== 503;
    return error instanceof TypeError;
}

export function generatedVerdict(source: GeneratedSource) {
    const blocked = source.health === 'blocked_by_site';
    const found = source.itemCount > 0;
    return {
        subscribable: !blocked && found,
        askAnchor:
            !blocked &&
            (!found || source.needsAnchor || source.health !== 'ok'),
        blocked,
    };
}

export function generatedCandidate(source: GeneratedSource): SourceCandidate {
    return {
        feedUrl: source.feedUrl,
        title: source.title,
        siteUrl: source.siteUrl,
    };
}

export type SourceOptions = { tags: string[]; primary: boolean; title?: string };

// Options travel only when set: a native publication with none needs no sidecar record.
export async function addSource(
    candidate: SourceCandidate,
    options: SourceOptions = { tags: [], primary: false },
) {
    // A publication's own name needs no sidecar, so only a name the reader typed travels with it.
    const identity =
        candidate.kind === 'standardfeed'
            ? { publication: candidate.publication, title: options.title }
            : { feedUrl: candidate.feedUrl, title: options.title ?? candidate.title };
    const result = await api<{ rkey: string }>('/api/subscriptions', {
        method: 'POST',
        body: {
            ...identity,
            siteUrl: candidate.siteUrl,
            ...(options.tags.length && { tags: options.tags }),
            ...(options.primary && { primary: true }),
        },
    });
    subscriptionChanges.dispatchEvent(new Event('change'));
    return result;
}

export type SourceChange = { title?: string; tags?: string[]; primary?: boolean; feedUrl?: string };

// The PATCH route rewrites only the fields it is sent, so the change carries only what the reader edited.
export async function editSource(rkey: string, change: SourceChange) {
    const { jobId } = await api<{ jobId?: string }>(`/api/subscriptions/${encodeURIComponent(rkey)}`, { method: 'PATCH', body: change });
    subscriptionChanges.dispatchEvent(new Event('change'));
    // A re-pointed feed has no posts until its fetch lands, so views read it again once the job settles.
    if (jobId) void settled(jobId).then(() => subscriptionChanges.dispatchEvent(new Event('change')));
}

const JOB_POLL_MS = 750;
const JOB_POLLS = 80;

// Resolves once the job has stopped running; a job that is unreadable or outlives the polls ends the wait the same way.
async function settled(jobId: string) {
    for (let poll = 0; poll < JOB_POLLS; poll++) {
        await new Promise((resolve) => setTimeout(resolve, JOB_POLL_MS));
        const job = await api<{ status: string }>(`/api/jobs/${encodeURIComponent(jobId)}`).catch(() => null);
        if (job?.status !== 'running') return;
    }
}

export async function unsubscribe(rkey: string) {
    await api(`/api/subscriptions/${encodeURIComponent(rkey)}`, { method: 'DELETE' });
    subscriptionChanges.dispatchEvent(new Event('change'));
}

export type PreviewPost = { title: string; publishedAt?: string; url?: string; contentHtml?: string };

export async function previewSource(candidate: SourceCandidate, signal: AbortSignal) {
    const { items } = await api<{ items: PreviewPost[] }>('/api/subscriptions/preview', {
        method: 'POST',
        body: candidate.kind === 'standardfeed' ? { publication: candidate.publication } : { feedUrl: candidate.feedUrl },
        signal,
    });
    return items;
}

export async function previewArticle(url: string, signal?: AbortSignal) {
    const { contentHtml } = await api<{ contentHtml: string }>('/api/subscriptions/preview/article', {
        method: 'POST',
        body: { url },
        signal,
    });
    return contentHtml;
}

export type SiteFollow = { title: string; feedUrl: string; siteUrl?: string; rkey?: string };

export type SiteProfile = {
    name?: string;
    summary?: string;
    aboutHtml?: string;
    contact?: { kind: 'email' | 'url'; value: string; preferred?: boolean }[];
    following?: SiteFollow[];
};

// A site without a frontmatter summary still gets a short bio: its about page's first paragraph.
export function profileSummary(profile: SiteProfile): string | undefined {
    if (profile.summary) return profile.summary;
    const template = document.createElement('template');
    template.innerHTML = profile.aboutHtml ?? '';
    return template.content.querySelector('p')?.textContent?.trim() || undefined;
}

export type SiteResult = SourceDiscovery & { url: string; profile: SiteProfile | null };

// The profile is optional decoration, so its failure never fails the lookup.
export async function findSite(input: string, signal: AbortSignal): Promise<SiteResult> {
    const url = webURL(input);
    const profile = url
        ? api<{ profile: SiteProfile | null }>('/api/subscriptions/profile', { method: 'POST', body: { url }, signal }).then(
              (result) => result.profile ?? null,
              () => null,
          )
        : Promise.resolve(null);
    const discovery = await discoverSources(input, signal);
    return { ...discovery, url: input, profile: await profile };
}

export async function newsletterAddress(
    signal: AbortSignal,
): Promise<string | undefined> {
    const existing = await api<{ address?: string }>(
        '/api/newsletters/address',
        { signal },
    );
    return existing.address;
}
