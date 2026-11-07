import { toast } from 'sonner';

import { api, describeMutationError } from '@/lib/api';
import type { ReaderEntry } from '@/lib/reader';

type SaveState = { saved: boolean; tags: string[] };
export type EntrySave = NonNullable<ReturnType<typeof createEntrySave>>;

type SaveRoutes = {
    create: () => Promise<{ id?: string; rkey?: string }>;
    path: (id: string) => string;
};

function saveRoutes(entry: ReaderEntry): SaveRoutes | null {
    if (entry.newsletter) {
        const { messageId } = entry.newsletter;
        return {
            create: () => api('/api/newsletter-saves', { method: 'POST', body: { messageId } }),
            path: (id) => `/api/newsletter-saves/${encodeURIComponent(id)}`,
        };
    }
    // A path-less Standardfeed document has no canonical URL to key a save on.
    if (!entry.url) return null;
    const body = { itemUrl: entry.url, feedUrl: entry.source.feedUrl };
    return {
        create: () => api('/api/saves', { method: 'POST', body }),
        path: (rkey) => `/api/saves/${encodeURIComponent(rkey)}`,
    };
}

// Every change shows at once and its request joins a queue, so a tag typed while the save is still in flight lands after it.
// A failed request puts the state back to what the server last confirmed.
export function createEntrySave(entry: ReaderEntry) {
    const found = saveRoutes(entry);
    if (!found) return null;
    const routes = found;
    let id = entry.savedState?.id ?? entry.savedState?.rkey ?? null;
    let confirmed: SaveState = { saved: Boolean(id), tags: entry.savedState?.tags ?? [] };
    let state = confirmed;
    let queue = Promise.resolve();
    const listeners = new Set<() => void>();

    function set(next: SaveState) {
        state = next;
        listeners.forEach((listener) => listener());
    }

    // A failure resets the view to the server's state, so changes queued behind it are dropped rather than applied unseen.
    let generation = 0;

    // Resolves to whether the change reached the server; a dropped or failed one resolves false.
    function enqueue(request: () => Promise<void>, failure: string): Promise<boolean> {
        const queuedIn = generation;
        const done = queue.then(async () => {
            if (queuedIn !== generation) return false;
            await request();
            return true;
        }).catch((error: unknown) => {
            generation += 1;
            set(confirmed);
            toast.error(describeMutationError(error, failure));
            return false;
        });
        queue = done.then(() => undefined);
        return done;
    }

    async function save() {
        if (state.saved) return true;
        set({ saved: true, tags: [] });
        return enqueue(async () => {
            if (id) return;
            const created = await routes.create();
            id = created.id ?? created.rkey ?? null;
            confirmed = { saved: true, tags: [] };
        }, 'Couldn’t save this. Try again.');
    }

    async function setTags(tags: string[]) {
        if (!state.saved) void save();
        set({ saved: true, tags });
        return enqueue(async () => {
            if (!id) return;
            const updated = await api<{ tags?: string[] }>(routes.path(id), { method: 'PATCH', body: { tags } });
            confirmed = { saved: true, tags: updated.tags ?? [] };
        }, 'Couldn’t save these tags. Try again.');
    }

    async function remove() {
        if (!state.saved) return true;
        set({ saved: false, tags: [] });
        return enqueue(async () => {
            if (!id) return;
            await api(routes.path(id), { method: 'DELETE' });
            id = null;
            confirmed = { saved: false, tags: [] };
        }, 'Couldn’t remove this save. Try again.');
    }

    return {
        get: () => state,
        subscribe(listener: () => void) {
            listeners.add(listener);
            return () => { listeners.delete(listener); };
        },
        save,
        setTags,
        remove,
    };
}
