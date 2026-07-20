import { useOptimisticRecord } from '@/hooks/use-optimistic-record';
import { api } from '@/lib/api';
import { toastMutationError } from '@/lib/mutation-toast';

export type SavedToggle = {
    initial: { rkey: string } | null;
    itemUrl: string;
    feedUrl: string | null;
};

export type SaveControl = {
    saved: boolean;
    busy: boolean;
    onToggle: () => void;
};

// Shared by the rail button and the reader's `b` shortcut so they can't drift out of sync.
export function useSaveToggle(toggle: SavedToggle): SaveControl {
    const record = useOptimisticRecord({
        initial: toggle.initial,
        deletePath: (rkey) => `/api/saves/${encodeURIComponent(rkey)}`,
        onDeleteError: (err) =>
            toastMutationError(err, "Couldn't remove this save. Try again."),
    });

    const onToggle = () => {
        if (record.busy) return;
        if (record.active) {
            record.remove();
            return;
        }
        record.setBusy(true);
        record.setActive(true);
        api<{ rkey: string }>('/api/saves', {
            method: 'POST',
            body: {
                itemUrl: toggle.itemUrl,
                feedUrl: toggle.feedUrl ?? undefined,
            },
        })
            .then((payload) => {
                record.setRkey(payload.rkey);
            })
            .catch((err) => {
                record.setActive(false);
                record.setRkey(null);
                toastMutationError(err, "Couldn't save this. Try again.");
            })
            .finally(() => record.setBusy(false));
    };

    return { saved: record.active, busy: record.busy, onToggle };
}
