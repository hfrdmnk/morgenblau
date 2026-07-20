import { api } from '@/lib/api';

export type Profile = {
    did: string;
    handle: string;
    displayName?: string | null;
    avatar?: string | null;
};

// Profile lookups decorate lists best-effort; a miss renders as a bare DID, never an error.
export async function fetchProfile(did: string): Promise<Profile | undefined> {
    try {
        return await api<Profile>(`/api/profiles/${encodeURIComponent(did)}`);
    } catch {
        return undefined;
    }
}

// One deduped lookup pass for every DID a list can reference.
export async function fetchProfiles(
    dids: string[],
): Promise<Record<string, Profile | undefined>> {
    const unique = Array.from(new Set(dids));
    const resolved = await Promise.all(unique.map(fetchProfile));
    return Object.fromEntries(unique.map((did, i) => [did, resolved[i]]));
}
