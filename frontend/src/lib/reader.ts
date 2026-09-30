export type ReaderEntry = {
    entrySlug: string;
    title: string | null;
    url?: string | null;
    contentType: string;
    publishedAt: string;
    body: string | null;
    metadata?: string | null;
    source: { title: string | null; siteUrl: string | null; feedUrl?: string };
    newsletter?: {
        messageId: string;
        senderName: string | null;
        senderAddress: string;
        sentAt: string | null;
        hasBlockedRemoteImages: boolean;
        remoteImagesAllowed: boolean;
    };
};

export function hasNewReaderContent(current: string | null, updated: string | null): boolean {
    return Boolean(updated?.trim()) && updated?.trim() !== current?.trim();
}

export function readerContentURL(value: string, base: string): string {
    if (value.trim().startsWith('#')) return value;
    try {
        return new URL(value, base).href;
    } catch {
        return value;
    }
}

export function readerBody(entry: ReaderEntry): string {
    const body = entry.body || '';
    const base = httpURL(entry.url);
    if (entry.newsletter || entry.contentType === 'newsletter' || !base) return body;

    // A template keeps image requests inert until publisher-relative URLs are resolved.
    const template = document.createElement('template');
    template.innerHTML = body;
    for (const attribute of ['href', 'src', 'cite']) {
        for (const element of template.content.querySelectorAll(`[${attribute}]`)) {
            element.setAttribute(attribute, readerContentURL(element.getAttribute(attribute)!, base));
        }
    }
    return template.innerHTML;
}

export function httpURL(value?: string | null): string | null {
    if (!value) return null;
    try {
        const url = new URL(value);
        return url.protocol === 'https:' || url.protocol === 'http:' ? url.href : null;
    } catch {
        return null;
    }
}

export function youtubeID(value?: string | null): string | null {
    const safe = httpURL(value);
    if (!safe) return null;
    const url = new URL(safe);
    const host = url.hostname;
    let id: string | null = null;
    if (host === 'youtu.be') id = url.pathname.slice(1);
    if (['youtube.com', 'www.youtube.com', 'm.youtube.com', 'www.youtube-nocookie.com'].includes(host)) {
        id = url.pathname === '/watch'
            ? url.searchParams.get('v')
            : /^\/(?:shorts|embed|live)\/([^/]+)\/?$/.exec(url.pathname)?.[1] ?? null;
    }
    return id && /^[\w-]{11}$/.test(id) ? id : null;
}

export function readerMetadata(raw?: string | null): {
    author?: string;
    video?: { url: string; type: string };
} {
    try {
        const data = JSON.parse(raw || '{}');
        const url = httpURL(typeof data?.enclosure?.url === 'string' ? data.enclosure.url : null);
        const type = typeof data?.enclosure?.type === 'string' ? data.enclosure.type.toLowerCase() : '';
        return {
            ...(typeof data?.author === 'string' && { author: data.author }),
            ...(url && type.startsWith('video/') && {
                video: { url, type },
            }),
        };
    } catch {
        return {};
    }
}
