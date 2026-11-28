import { useReducer, useRef } from 'react';
import { toast } from 'sonner';

import { api, describeMutationError } from '@/lib/api';
import { saveKey, saveTagsPath, saveTitle, type LibrarySave } from '@/lib/library';
export function useSaveRemoval(refresh: () => void) {
    const removed = useRef(new Set<string>());
    const [, rerender] = useReducer((count: number) => count + 1, 0);

    function remove(save: LibrarySave) {
        const key = saveKey(save);
        removed.current.add(key);
        rerender();
        const done = api(saveTagsPath(save), { method: 'DELETE' });
        done.catch((error: unknown) => {
            removed.current.delete(key);
            rerender();
            toast.dismiss('library-removed');
            toast.error(describeMutationError(error, 'Couldn’t remove this save. Try again.'));
        });
        toast('Removed from Library', {
            id: 'library-removed',
            description: saveTitle(save),
            action: {
                label: 'Undo',
                onClick: () =>
                    void done
                        .then(() => saveAgain(save))
                        .then(() => {
                            removed.current.delete(key);
                            refresh();
                        })
                        .catch((error: unknown) =>
                            toast.error(describeMutationError(error, 'Couldn’t put this back in your Library.')),
                        ),
            },
            classNames: {
                actionButton:
                    '-my-1 ml-auto shrink-0 cursor-pointer rounded-sm px-2 py-1 text-sm font-medium hover:bg-secondary',
            },
        });
    }

    return { remove, isRemoved: (save: LibrarySave) => removed.current.has(saveKey(save)) };
}

async function saveAgain(save: LibrarySave) {
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
    if (save.tags?.length) await api(path, { method: 'PATCH', body: { tags: save.tags } });
}
