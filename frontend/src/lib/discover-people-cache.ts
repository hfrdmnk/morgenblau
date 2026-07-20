import type { DiscoverPerson, PersonPreview } from '@/lib/discover-people';
import type { FollowRecord } from '@/lib/follow';
import type { Profile } from '@/lib/profile';

// Bridges the People tab across mount/unmount within a session; React state stays the source of truth.
const TTL_MS = 60 * 60 * 1000;

export type PeopleCacheEntry = {
    people: DiscoverPerson[];
    nextCursor?: string;
    follows: FollowRecord[];
    profiles: Record<string, Profile | undefined>;
    previews: Record<string, PersonPreview>;
    fetchedAt: number;
};

let entry: PeopleCacheEntry | undefined;

export function readPeopleCache(): PeopleCacheEntry | undefined {
    if (!entry) return undefined;
    if (Date.now() - entry.fetchedAt > TTL_MS) return undefined;
    return entry;
}

export function writePeopleCache(
    people: DiscoverPerson[],
    follows: FollowRecord[],
    profiles: Record<string, Profile | undefined>,
    nextCursor?: string,
): void {
    entry = {
        people,
        nextCursor,
        follows,
        profiles,
        previews: {},
        fetchedAt: Date.now(),
    };
}

export function writeCachedPeople(
    people: DiscoverPerson[],
    nextCursor: string | undefined,
): void {
    if (!entry) return;
    entry.people = people;
    entry.nextCursor = nextCursor;
}

export function writeCachedPeopleProfiles(
    profiles: Record<string, Profile | undefined>,
): void {
    if (!entry) return;
    entry.profiles = { ...entry.profiles, ...profiles };
}

export function writeCachedFollows(follows: FollowRecord[]): void {
    if (!entry) return;
    entry.follows = follows;
}

export function writeCachedPersonPreview(did: string, preview: PersonPreview): void {
    if (!entry) return;
    entry.previews[did] = preview;
}
