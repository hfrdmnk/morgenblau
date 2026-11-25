import { useReducer, useRef } from 'react';
import { toast } from 'sonner';

import { api, describeMutationError } from '@/lib/api';
import { saveKey, saveTagsPath, withState, type LibrarySave, type SaveState } from '@/lib/library';

const FILED: Record<SaveState, string> = { queue: 'Back in To read', archived: 'Archived' };

// Filing shows at once and its request joins the save's own queue, so an undo pressed mid-flight lands after the change it undoes.
// A failure puts back the tags the server last confirmed.
export function useLibraryFiling(refresh: () => void) {
    const current = useRef(new Map<string, string[]>());
    const removed = useRef(new Set<string>());
    const confirmed = useRef(new Map<string, string[]>());
    const queues = useRef(new Map<string, Promise<void>>());
    const [, rerender] = useReducer((count: number) => count + 1, 0);

    const tagsOf = (save: LibrarySave) => current.current.get(saveKey(save)) ?? save.tags ?? [];

    function set(key: string, tags: string[]) {
        current.current.set(key, tags);
        rerender();
    }

    function apply(save: LibrarySave, tags: string[]) {
        const key = saveKey(save);
        if (!confirmed.current.has(key)) confirmed.current.set(key, save.tags ?? []);
        set(key, tags);
        const done = queued(key).then(async () => {
            const updated = await api<{ tags?: string[] }>(saveTagsPath(save), { method: 'PATCH', body: { tags } });
            confirmed.current.set(key, updated.tags ?? []);
        });
        queues.current.set(
            key,
            done.catch((error: unknown) => {
                set(key, confirmed.current.get(key)!);
                toast.error(describeMutationError(error, 'Couldn’t file this save. Try again.'));
            }),
        );
    }

    const queued = (key: string) => queues.current.get(key) ?? Promise.resolve();

    // Undo puts back the exact tags from before, not only the state.
    function file(save: LibrarySave, state: SaveState) {
        const before = tagsOf(save);
        apply(save, withState(before, state));
        toast(FILED[state], {
            id: 'library-filed',
            description: save.title,
            action: { label: 'Undo', onClick: () => apply(save, before) },
            classNames: {
                actionButton:
                    '-my-1 ml-auto shrink-0 cursor-pointer rounded-sm px-2 py-1 text-sm font-medium hover:bg-secondary',
            },
        });
    }

    // Undo saves the post again, which gives it a new record, so the list reloads instead of putting the old row back.
    function remove(save: LibrarySave) {
        const key = saveKey(save);
        const tags = tagsOf(save);
        removed.current.add(key);
        rerender();
        const done = api(saveTagsPath(save), { method: 'DELETE' });
        done.catch((error: unknown) => {
            removed.current.delete(key);
            rerender();
            toast.error(describeMutationError(error, 'Couldn’t remove this save. Try again.'));
        });
        toast('Removed from Library', {
            id: 'library-filed',
            description: save.title,
            action: {
                label: 'Undo',
                onClick: () =>
                    void done
                        .then(() => saveAgain(save, tags))
                        .then(() => {
                            removed.current.delete(key);
                            refresh();
                        })
                        .catch((error: unknown) =>
                            toast.error(describeMutationError(error, 'Couldn’t save this again. Try again.')),
                        ),
            },
            classNames: {
                actionButton:
                    '-my-1 ml-auto shrink-0 cursor-pointer rounded-sm px-2 py-1 text-sm font-medium hover:bg-secondary',
            },
        });
    }

    return { tagsOf, file, remove, isRemoved: (save: LibrarySave) => removed.current.has(saveKey(save)) };
}

async function saveAgain(save: LibrarySave, tags: string[]) {
    let path: string;
    if (save.kind === 'newsletter') {
        const created = await api<{ id: string }>('/api/newsletter-saves', {
            method: 'POST',
            body: { messageId: save.messageId },
        });
        path = saveTagsPath({ ...save, id: created.id });
    } else {
        const created = await api<{ rkey: string }>('/api/saves', {
            method: 'POST',
            body: { itemUrl: save.itemUrl, feedUrl: save.feedUrl },
        });
        path = saveTagsPath({ ...save, rkey: created.rkey });
    }
    if (tags.length) await api(path, { method: 'PATCH', body: { tags } });
}
