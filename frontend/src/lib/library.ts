import { api } from '@/lib/api';
import { fetchProfile, type Profile } from '@/lib/profile';

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

export type NetworkShareWithProfile = NetworkShare & { profile?: Profile };

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

// Pairs each share with the profile resolved for its sharer, by position against `dids`.
export function hydrateNetworkShares(
    shares: NetworkShare[],
    dids: string[],
    profiles: (Profile | undefined)[],
): NetworkShareWithProfile[] {
    const profileByDID = new Map(dids.map((did, i) => [did, profiles[i]]));
    return shares.map((share) => ({
        ...share,
        profile: profileByDID.get(share.sharerDid),
    }));
}

export async function fetchNetworkShares(): Promise<NetworkShareWithProfile[]> {
    const shares =
        (await api<NetworkShare[] | null>('/api/library/network-shares')) ??
        [];
    const dids = uniqueSharerDIDs(shares);
    const profiles = await Promise.all(dids.map(fetchProfile));
    return hydrateNetworkShares(shares, dids, profiles);
}

export async function fetchShares(): Promise<Share[]> {
    return (await api<Share[] | null>('/api/shares')) ?? [];
}
