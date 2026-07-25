import { api } from '@/lib/api';

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

export type Share = {
    rkey: string;
    kind: 'rss' | 'standardfeed';
    itemUrl?: string;
    document?: string;
    comment?: string;
    createdAt: string;
    title?: string;
    targetUrl?: string;
    entrySlug?: string;
};

export type Save = {
    rkey: string;
    uri?: string;
    cid?: string;
    itemUrl: string;
    feedUrl?: string;
    createdAt: string;
    title?: string;
    targetUrl?: string;
    entrySlug?: string;
};

// First-seen order so the caller resolves each identity once in the batch profile lookup.
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

// Rows only: the caller hydrates sharer profiles separately so shares render before identities resolve.
export async function fetchNetworkShares(): Promise<NetworkShare[]> {
    return (
        (await api<NetworkShare[] | null>('/api/library/network-shares')) ?? []
    );
}

export async function fetchShares(): Promise<Share[]> {
    return (await api<Share[] | null>('/api/shares')) ?? [];
}

export async function fetchSaves(): Promise<Save[]> {
    return (await api<Save[] | null>('/api/saves')) ?? [];
}
