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
    existingSubscriptions: { feedUrl: string; title: string | null }[];
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

const RATE_LIMITED = 'Too many generated feeds this hour. Try again later.';

// Server messages are terse logs, so every failure reads in the dialog's own words.
export function generationError(error: unknown) {
    if (error instanceof ApiError) {
        if (error.status === 429 || error.code === 'rate_limited')
            return RATE_LIMITED;
        if (error.status === 400) return 'Enter a website URL.';
        if (error.status === 502) return 'Couldn’t read that page.';
        if (error.status === 503)
            return 'Feed generation isn’t available right now.';
        if (error.status < 500) return error.message;
    }
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

// A limit, an unavailable generator or a bad address answers the same way at once.
export function canRetryGeneration(error: unknown) {
    if (error instanceof ApiError)
        return error.status >= 500 && error.status !== 503;
    return error instanceof TypeError;
}

export function generatedVerdict(source: GeneratedSource) {
    const blocked = source.health === 'blocked_by_site';
    const found = source.health !== 'broken' && source.itemCount > 0;
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

export async function addSource(candidate: SourceCandidate) {
    const identity =
        candidate.kind === 'standardfeed'
            ? { publication: candidate.publication }
            : { feedUrl: candidate.feedUrl, title: candidate.title };
    const result = await api('/api/subscriptions', {
        method: 'POST',
        body: { ...identity, siteUrl: candidate.siteUrl },
    });
    subscriptionChanges.dispatchEvent(new Event('change'));
    return result;
}

export async function saveSources(
    candidates: SourceCandidate[],
    onAdded: (candidate: SourceCandidate) => void,
) {
    for (const candidate of candidates) {
        await addSource(candidate);
        onAdded(candidate);
    }
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
