import { parseDigestDate, toDateKey } from '@/lib/digest-dates';
import type { SaveState } from '@/lib/library';

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
    admin: '/admin',
    oauthLogin: '/oauth/login',
    oauthLogout: '/oauth/logout',
} as const;

export const APP_MODES = [
    { label: 'Digest', href: PATHS.digest, shortcut: 'Shift+D' },
    { label: 'Sources', href: PATHS.sources, shortcut: 'Shift+S' },
    { label: 'Library', href: PATHS.library, shortcut: 'Shift+L' },
] as const;

export function appMode(path: string) {
    return APP_MODES.find(({ href }) => path === href || (href !== PATHS.digest && path.startsWith(`${href}/`)));
}

export function isSettingsPath(path: string): boolean {
    return path === PATHS.settings || path.startsWith(`${PATHS.settings}/`);
}

export function isAdminPath(path: string): boolean {
    return path === PATHS.admin || path.startsWith(`${PATHS.admin}/`);
}

// Pages opened from the account menu, which offer a way back to the mode they were opened from.
export function isAccountMenuPath(path: string): boolean {
    return isSettingsPath(path) || isAdminPath(path);
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

export function libraryHref(tab: SaveState = 'queue'): string {
    return tab === 'queue' ? PATHS.library : `${PATHS.library}?tab=${tab}`;
}

const LIBRARY_PAGE = /^\/library(?:\?tab=archived)?$/;

// Like sourceOrigin: only a Library tab the app itself links to is trusted as the way back.
function libraryOrigin(from: string | null): string | null {
    return from && LIBRARY_PAGE.test(from) ? from : null;
}

// The reader returns to the source page or Library tab it was opened from, and otherwise to the digest day `from` names.
export function readerBack(from: string | null): { href: string; label: string } {
    const source = sourceOrigin(from);
    if (source) return { href: source, label: 'Back to source' };
    const library = libraryOrigin(from);
    if (library) return { href: library, label: 'Back to library' };
    const date = parseDigestDate(from);
    return { href: digestHref(date ? toDateKey(date) : undefined), label: 'Back to digest' };
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
