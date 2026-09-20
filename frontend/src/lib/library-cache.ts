import type { Save } from '@/lib/library';
import { subscribeLibraryMutation } from '@/lib/library-events';

// Bridges the Library tabs across mount/unmount within a session; React state stays the source of truth.
const TTL_MS = 60 * 60 * 1000;

type SavedEntry = { saves: Save[]; fetchedAt: number };

let saved: SavedEntry | undefined;

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

function clearSavedCache(): void {
    saved = undefined;
}

// Mutation responses carry no list fields, so a save invalidates the list rather than patching it.
subscribeLibraryMutation(clearSavedCache);
