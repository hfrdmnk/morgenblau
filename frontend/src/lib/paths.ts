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

export function entryHref(slug: string): string {
    return `${PATHS.entry}/${slug}`;
}
