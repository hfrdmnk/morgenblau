export const PATHS = {
    welcome: '/',
    login: '/login',
    consume: '/consume',
    sources: '/sources',
    discover: '/discover',
    create: '/create',
    entry: '/entry',
    oauthLogin: '/oauth/login',
    oauthLogout: '/oauth/logout',
} as const;

export type AppPath = (typeof PATHS)[keyof typeof PATHS];

export function entryHref(slug: string, fromDate?: string): string {
    const base = `${PATHS.entry}/${slug}`;
    return fromDate ? `${base}?from=${encodeURIComponent(fromDate)}` : base;
}

export function consumeHref(date?: string): string {
    return date ? `${PATHS.consume}?date=${encodeURIComponent(date)}` : PATHS.consume;
}
