import { parseDigestDate, toDateKey } from '@/lib/digest-dates';
import { librarySort, libraryView, type LibrarySave, type LibrarySort, type LibraryView } from '@/lib/library';

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

export function sourceOrigin(from: string | null): string | null {
    return from && SOURCE_PAGE.test(from) ? from : null;
}

export function libraryHref(
    view: LibraryView = 'recent',
    { sort = 'newest', query = '' }: { sort?: LibrarySort; query?: string } = {},
): string {
    if (view === 'recent') return PATHS.library;
    const params = new URLSearchParams({ view });
    if (sort !== 'newest') params.set('sort', sort);
    if (query.trim()) params.set('q', query);
    return `${PATHS.library}?${params}`;
}

const LIBRARY_PARAMS = new Set(['view', 'sort', 'q']);

function libraryOrigin(from: string | null): string | null {
    if (!from?.startsWith(`${PATHS.library}?`) && from !== PATHS.library) return null;
    const url = new URL(from, 'https://app.invalid');
    if (url.pathname !== PATHS.library || [...url.searchParams.keys()].some((key) => !LIBRARY_PARAMS.has(key)))
        return null;
    return libraryHref(libraryView(url.search), {
        sort: librarySort(url.search),
        query: url.searchParams.get('q') ?? '',
    });
}

export function saveHref(save: LibrarySave, from: string): string {
    if (save.entrySlug) return entryHref(save.entrySlug, from);
    return save.kind === 'newsletter' ? from : save.itemUrl;
}

export function readerBack(from: string | null): { href: string; label: string } {
    const source = sourceOrigin(from);
    if (source) return { href: source, label: 'Back to source' };
    const library = libraryOrigin(from);
    if (library) return { href: library, label: 'Back to Library' };
    const date = parseDigestDate(from);
    return { href: digestHref(date ? toDateKey(date) : undefined), label: 'Back to digest' };
}

export function digestHref(date?: string): string {
    return date ? `${PATHS.digest}?date=${encodeURIComponent(date)}` : PATHS.digest;
}

const NEWSLETTER_PAGE = /^\/sources\/newsletters\/(.+)$/;

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
