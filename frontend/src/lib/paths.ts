export const PATHS = {
    login: '/login',
    digest: '/',
    library: '/library',
    sources: '/sources',
    settings: '/settings',
    import: '/settings/import',
    export: '/settings/export',
    entry: '/entry',
    oauthLogin: '/oauth/login',
    oauthLogout: '/oauth/logout',
} as const;

export function entryHref(slug: string, date: string): string {
    return `${PATHS.entry}/${slug}?from=${encodeURIComponent(date)}`;
}

export function digestHref(date?: string): string {
    return date ? `${PATHS.digest}?date=${encodeURIComponent(date)}` : PATHS.digest;
}
