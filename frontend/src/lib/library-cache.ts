import type { NetworkShare, Save, Share } from '@/lib/library';
import { subscribeLibraryMutation } from '@/lib/library-events';
import type { Profile } from '@/lib/profile';

// Bridges the Library tabs across mount/unmount within a session; React state stays the source of truth.
const TTL_MS = 60 * 60 * 1000;

type SavedEntry = { saves: Save[]; fetchedAt: number };
type SharedEntry = { shares: Share[]; fetchedAt: number };
type NetworkEntry = {
    shares: NetworkShare[];
    profiles: Record<string, Profile | undefined>;
    fetchedAt: number;
};

let saved: SavedEntry | undefined;
let shared: SharedEntry | undefined;
let network: NetworkEntry | undefined;

function unexpired<T extends { fetchedAt: number }>(
    entry: T | undefined,
): T | undefined {
    if (!entry) return undefined;
    if (Date.now() - entry.fetchedAt > TTL_MS) return undefined;
    return entry;
}

export function readSavedCache(): SavedEntry | undefined {
    return unexpired(saved);
}

export function writeSavedCache(saves: Save[]): void {
    saved = { saves, fetchedAt: Date.now() };
}

export function writeCachedSaves(saves: Save[]): void {
    if (!saved) return;
    saved.saves = saves;
}

export function readSharedCache(): SharedEntry | undefined {
    return unexpired(shared);
}

export function writeSharedCache(shares: Share[]): void {
    shared = { shares, fetchedAt: Date.now() };
}

export function writeCachedShares(shares: Share[]): void {
    if (!shared) return;
    shared.shares = shares;
}

export function readNetworkCache(): NetworkEntry | undefined {
    return unexpired(network);
}

export function writeNetworkCache(shares: NetworkShare[]): void {
    network = { shares, profiles: {}, fetchedAt: Date.now() };
}

export function writeCachedNetworkShares(shares: NetworkShare[]): void {
    if (!network) return;
    network.shares = shares;
}

export function writeCachedNetworkProfiles(
    profiles: Record<string, Profile | undefined>,
): void {
    if (!network) return;
    network.profiles = { ...network.profiles, ...profiles };
}

function clearSavedCache(): void {
    saved = undefined;
}

function clearSharedCache(): void {
    shared = undefined;
}

function clearNetworkCache(): void {
    network = undefined;
}

// Mutation responses carry no list fields, so a mutation invalidates the lists it lands in rather than patching them.
subscribeLibraryMutation((event) => {
    if (event.kind === 'save') {
        clearSavedCache();
        return;
    }
    clearSharedCache();
    clearNetworkCache();
});
