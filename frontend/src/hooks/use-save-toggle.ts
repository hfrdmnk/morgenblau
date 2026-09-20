import { useOptimisticRecord } from '@/hooks/use-optimistic-record';
import { api } from '@/lib/api';
import { emitLibraryMutation } from '@/lib/library-events';
import { toastMutationError } from '@/lib/mutation-toast';

type FeedSavedToggle = {
    kind: 'feed';
    initial: { rkey: string } | null;
    itemUrl: string;
    feedUrl: string | null;
};

type NewsletterSavedToggle = {
    kind: 'newsletter';
    initial: { kind: 'newsletter'; id: string } | null;
    messageId: string;
};

export type SavedToggle = FeedSavedToggle | NewsletterSavedToggle;

export type SaveControl = {
    saved: boolean;
    busy: boolean;
    onToggle: () => void;
};

type OptimisticRecord = ReturnType<typeof useOptimisticRecord>;
type SaveResponse = { id?: string; rkey?: string };

function initialRecord(toggle: SavedToggle): { id: string } | null {
    if (toggle.kind === 'newsletter') return toggle.initial;
    return toggle.initial ? { id: toggle.initial.rkey } : null;
}

function deletePath(toggle: SavedToggle, id: string): string {
    const encodedId = encodeURIComponent(id);
    return toggle.kind === 'newsletter'
        ? `/api/newsletter-saves/${encodedId}`
        : `/api/saves/${encodedId}`;
}

function createSave(toggle: SavedToggle): Promise<SaveResponse> {
    if (toggle.kind === 'newsletter') {
        return api('/api/newsletter-saves', {
            method: 'POST',
            body: { messageId: toggle.messageId },
        });
    }
    return api('/api/saves', {
        method: 'POST',
        body: {
            itemUrl: toggle.itemUrl,
            feedUrl: toggle.feedUrl ?? undefined,
        },
    });
}

function applyCreatedSave(payload: SaveResponse, record: OptimisticRecord) {
    const id = payload.id ?? payload.rkey;
    if (!id) throw new Error('Save response did not include an id');
    record.setKey(id);
    emitLibraryMutation();
}

function rollbackCreatedSave(error: unknown, record: OptimisticRecord) {
    record.setActive(false);
    record.setKey(null);
    toastMutationError(error, "Couldn't save this. Try again.");
}

function startCreate(toggle: SavedToggle, record: OptimisticRecord) {
    record.setBusy(true);
    record.setActive(true);
    createSave(toggle)
        .then((payload) => applyCreatedSave(payload, record))
        .catch((error) => rollbackCreatedSave(error, record))
        .finally(() => record.setBusy(false));
}

// Shared by the rail button and the reader's `b` shortcut so they can't drift out of sync.
export function useSaveToggle(toggle: SavedToggle): SaveControl {
    const record = useOptimisticRecord({
        initial: initialRecord(toggle),
        deletePath: (id) => deletePath(toggle, id),
        onDeleteError: (err) =>
            toastMutationError(err, "Couldn't remove this save. Try again."),
    });

    const onToggle = () => {
        if (record.busy) return;
        if (record.active) {
            record.remove();
            emitLibraryMutation();
            return;
        }
        startCreate(toggle, record);
    };

    return { saved: record.active, busy: record.busy, onToggle };
}
