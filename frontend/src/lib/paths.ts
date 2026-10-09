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
    { label: 'Digest', href: PATHS.digest },
    { label: 'Sources', href: PATHS.sources },
    { label: 'Library', href: PATHS.library },
] as const;

export function appMode(path: string) {
    return APP_MODES.find(({ href }) =>
        path === href || (href !== PATHS.digest && path.startsWith(`${href}/`)),
    );
}

export function isSettingsPath(path: string): boolean {
    return path === PATHS.settings || path.startsWith(`${PATHS.settings}/`);
}

export function entryHref(slug: string, date: string): string {
    return `${PATHS.entry}/${slug}?from=${encodeURIComponent(date)}`;
}

export function digestHref(date?: string): string {
    return date ? `${PATHS.digest}?date=${encodeURIComponent(date)}` : PATHS.digest;
}
