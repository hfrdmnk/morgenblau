import { api } from '@/lib/api';

export type Profile = {
    did: string;
    handle: string;
    displayName?: string | null;
    avatar?: string | null;
    description?: string | null;
};

type ProfilesResponse = { profiles: Record<string, Profile | null> };

// Matches the server's per-request DID cap on /api/profiles.
const PROFILE_CHUNK_SIZE = 50;

export function chunked<T>(items: T[], size: number): T[][] {
    const chunks: T[][] = [];
    for (let i = 0; i < items.length; i += size) {
        chunks.push(items.slice(i, i + size));
    }
    return chunks;
}

// Profile lookups decorate lists best-effort; a failed chunk renders as bare DIDs, never an error.
async function fetchProfileChunk(
    dids: string[],
): Promise<ProfilesResponse | null> {
    const query = dids.map((did) => encodeURIComponent(did)).join(',');
    return api<ProfilesResponse>(`/api/profiles?dids=${query}`).catch(
        () => null,
    );
}

// One deduped lookup pass for every DID a list can reference.
export async function fetchProfiles(
    dids: string[],
): Promise<Record<string, Profile | undefined>> {
    const unique = Array.from(new Set(dids));
    const responses = await Promise.all(
        chunked(unique, PROFILE_CHUNK_SIZE).map(fetchProfileChunk),
    );
    return Object.fromEntries(
        responses
            .flatMap((response) => Object.entries(response?.profiles ?? {}))
            .map(([did, profile]): [string, Profile | undefined] => [
                did,
                profile ?? undefined,
            ]),
    );
}
