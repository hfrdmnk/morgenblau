import { parseDigestDate } from '@/lib/digest-dates';

export const PATHS = {
    login: '/login',
    digest: '/',
    library: '/library',
    sources: '/sources',
    settings: '/settings',
    general: '/settings/general',
    import: '/settings/import',
    export: '/settings/export',
    entry: '/entry',
    oauthLogin: '/oauth/login',
    oauthLogout: '/oauth/logout',
} as const;

export const APP_MODES = [
    { label: 'Digest', href: PATHS.digest, shortcut: 'Shift+D' },
    { label: 'Sources', href: PATHS.sources, shortcut: 'Shift+S' },
    { label: 'Library', href: PATHS.library, shortcut: 'Shift+L' },
] as const;

export function appMode(path: string) {
    return APP_MODES.find(({ href }) =>
        path === href || (href !== PATHS.digest && path.startsWith(`${href}/`)),
    );
}

export function isSettingsPath(path: string): boolean {
    return path === PATHS.settings || path.startsWith(`${PATHS.settings}/`);
}

export function sourceHref(rkey: string): string {
    return `${PATHS.sources}/${encodeURIComponent(rkey)}`;
}

export function newsletterSourceHref(id: string): string {
    return `${PATHS.sources}/newsletters/${encodeURIComponent(id)}`;
}

export function entryHref(slug: string, from: string): string {
    return `${PATHS.entry}/${slug}?from=${encodeURIComponent(from)}`;
}

export function readerSourceHref(source: { kind?: string; id?: string; rkey?: string }): string | null {
    if (source.kind === 'newsletter') return source.id ? newsletterSourceHref(source.id) : null;
    return source.rkey ? sourceHref(source.rkey) : null;
}

const SOURCE_PAGE = /^\/sources\/(?:newsletters\/)?[^/?#\\]+$/;

// `from` arrives in the URL, so only a source page path is trusted as a link target.
export function sourceOrigin(from: string | null): string | null {
    return from && SOURCE_PAGE.test(from) ? from : null;
}

export function digestHref(date?: string): string {
    return date ? `${PATHS.digest}?date=${encodeURIComponent(date)}` : PATHS.digest;
}

const NEWSLETTER_PAGE = /^\/sources\/newsletters\/(.+)$/;

// The API path that returns the list behind an entry's `from`, in the order that list shows it.
export function entryListPath(from: string | null, timezone: string): string | null {
    const source = sourceOrigin(from);
    if (source) {
        const newsletter = NEWSLETTER_PAGE.exec(source);
        return newsletter
            ? `/api/newsletters/${newsletter[1]}/entries`
            : `/api/subscriptions/${source.slice(PATHS.sources.length + 1)}/entries`;
    }
    if (!from || !parseDigestDate(from)) return null;
    return digestApiPath(from, timezone);
}

export function digestApiPath(date: string, timezone: string): string {
    return `/api/digest?${new URLSearchParams({ date, timezone })}`;
}
