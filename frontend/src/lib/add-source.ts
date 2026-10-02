import { api } from './api';

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

export function discoverSources(input: string, signal: AbortSignal) {
    const trimmed = input.trim();
    const value = /^[a-z][a-z\d+.-]*:/i.test(trimmed)
        ? trimmed
        : `https://${trimmed}`;
    let url: URL;
    try {
        url = new URL(value);
        if (
            !['https:', 'http:'].includes(url.protocol) ||
            url.username ||
            url.password
        )
            throw new Error();
    } catch {
        return Promise.reject(new Error('Enter a website or feed URL.'));
    }
    return api<SourceDiscovery>('/api/subscriptions/resolve', {
        method: 'POST',
        body: { url: url.href },
        signal,
    });
}

export async function addSource(candidate: SourceCandidate) {
    const identity =
        candidate.kind === 'standardfeed'
            ? { publication: candidate.publication }
            : { feedUrl: candidate.feedUrl, title: candidate.title };
    const result = await api('/api/subscriptions', {
        method: 'POST',
        body: {
            subscriptions: [
                {
                    ...identity,
                    siteUrl: candidate.siteUrl,
                },
            ],
        },
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
