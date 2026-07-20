import type { DiscoverSource, DiscoverSourcePost } from '@/lib/discover';
import type { Profile } from '@/lib/profile';

// Bridges the Discover page across mount/unmount within a session; React state stays the source of truth.
const TTL_MS = 60 * 60 * 1000;

export type CacheEntry = {
    sources: DiscoverSource[];
    nextCursor?: string;
    profiles: Record<string, Profile | undefined>;
    posts: Record<string, DiscoverSourcePost[]>;
    fetchedAt: number;
};

let entry: CacheEntry | undefined;

export function readDiscoverCache(): CacheEntry | undefined {
    if (!entry) return undefined;
    if (Date.now() - entry.fetchedAt > TTL_MS) return undefined;
    return entry;
}

export function writeDiscoverCache(
    sources: DiscoverSource[],
    profiles: Record<string, Profile | undefined>,
    nextCursor?: string,
): void {
    entry = { sources, nextCursor, profiles, posts: {}, fetchedAt: Date.now() };
}

export function writeCachedSources(
    sources: DiscoverSource[],
    nextCursor: string | undefined,
): void {
    if (!entry) return;
    entry.sources = sources;
    entry.nextCursor = nextCursor;
}

export function writeCachedProfiles(
    profiles: Record<string, Profile | undefined>,
): void {
    if (!entry) return;
    entry.profiles = { ...entry.profiles, ...profiles };
}

export function writeCachedPost(key: string, posts: DiscoverSourcePost[]): void {
    if (!entry) return;
    entry.posts[key] = posts;
}
