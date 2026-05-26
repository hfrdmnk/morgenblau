export const PATHS = {
    welcome: '/',
    login: '/login',
    digest: '/digest',
    library: '/library',
    sources: '/sources',
    discover: '/discover',
    entry: '/entry',
    oauthLogin: '/oauth/login',
    oauthLogout: '/oauth/logout',
} as const;

export type AppPath = (typeof PATHS)[keyof typeof PATHS];

export function entryHref(slug: string, fromDate?: string): string {
    const base = `${PATHS.entry}/${slug}`;
    return fromDate ? `${base}?from=${encodeURIComponent(fromDate)}` : base;
}

export function digestHref(date?: string): string {
    return date ? `${PATHS.digest}?date=${encodeURIComponent(date)}` : PATHS.digest;
}

export function sourceHref(rkey: string): string {
    return `${PATHS.sources}/${rkey}`;
}
