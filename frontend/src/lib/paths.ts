export const PATHS = {
    welcome: '/',
    login: '/login',
    consume: '/consume',
    sources: '/sources',
    entry: '/entry',
    oauthLogin: '/oauth/login',
    oauthLogout: '/oauth/logout',
} as const;

export type AppPath = (typeof PATHS)[keyof typeof PATHS];
