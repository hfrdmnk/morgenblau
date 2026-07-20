export const PATHS = {
    welcome: '/',
    login: '/login',
    digest: '/digest',
    library: '/library',
    sources: '/sources',
    discover: '/discover',
    profile: '/profile',
    entry: '/entry',
    oauthLogin: '/oauth/login',
    oauthLogout: '/oauth/logout',
} as const;

export type EntryFrom = { date?: string; sourceRkey?: string };

export function entryHref(slug: string, from?: EntryFrom): string {
    const base = `${PATHS.entry}/${slug}`;
    if (from?.sourceRkey) {
        return `${base}?fromSource=${encodeURIComponent(from.sourceRkey)}`;
    }
    if (from?.date) {
        return `${base}?from=${encodeURIComponent(from.date)}`;
    }
    return base;
}

export function digestHref(date?: string): string {
    return date ? `${PATHS.digest}?date=${encodeURIComponent(date)}` : PATHS.digest;
}

export function sourceHref(rkey: string): string {
    return `${PATHS.sources}/${rkey}`;
}

function profileHref(handleOrDid: string): string {
    return `${PATHS.profile}/${handleOrDid}`;
}

// SPEC <discovery> Profile page: handle preferred in links, DID the stable fallback.
export function personHref(did: string, handle?: string): string {
    return profileHref(handle ?? did);
}
