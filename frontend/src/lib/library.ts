export type NetworkShare = {
    sharerDid: string;
    kind: 'rss' | 'standardfeed' | 'skyreader';
    itemUrl?: string;
    document?: string;
    title?: string;
    targetUrl?: string;
    entrySlug?: string;
    comment?: string;
    createdAt: string;
};

// First-seen order so the caller resolves each identity once before the /api/profiles/{did} fan-out.
export function uniqueSharerDIDs(shares: NetworkShare[]): string[] {
    const seen = new Set<string>();
    const out: string[] = [];
    for (const share of shares) {
        if (!seen.has(share.sharerDid)) {
            seen.add(share.sharerDid);
            out.push(share.sharerDid);
        }
    }
    return out;
}
