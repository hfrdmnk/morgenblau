import { useState } from 'react';

import { useOptimisticRecord } from '@/hooks/use-optimistic-record';
import { api, classifyMutationError, type MutationErrorKind } from '@/lib/api';

export type ShareToggle = {
    initial: { rkey: string } | null;
    entrySlug: string;
    // A path-less Standardfeed document can't take a comment (server 422s), so the composer hides the note field.
    canComment: boolean;
};

// Aliases keep existing callers (library.tsx) on one import site for share errors.
export type ShareError = MutationErrorKind;
export const classifyShareError = classifyMutationError;

export type ShareControl = {
    shared: boolean;
    busy: boolean;
    canComment: boolean;
    composerOpen: boolean;
    error: ShareError | null;
    onToggle: () => void; // shared → unshare (direct DELETE); else open composer
    closeComposer: () => void;
    submit: (comment: string) => void;
};

// Sharing opens a composer then POSTs; unsharing is a direct optimistic DELETE, mirroring the save toggle so all callers share one source of truth.
export function useShareToggle(toggle: ShareToggle): ShareControl {
    const [composerOpen, setComposerOpen] = useState(false);
    const [error, setError] = useState<ShareError | null>(null);
    const record = useOptimisticRecord({
        initial: toggle.initial,
        deletePath: (rkey) => `/api/shares/${encodeURIComponent(rkey)}`,
        onDeleteError: (err) => setError(classifyShareError(err)),
    });

    const closeComposer = () => setComposerOpen(false);

    const onToggle = () => {
        if (record.busy) return;
        if (record.active) {
            record.remove();
            return;
        }
        setError(null);
        setComposerOpen(true);
    };

    const submit = (comment: string) => {
        if (record.busy) return;
        record.setBusy(true);
        setError(null);
        const trimmed = comment.trim();
        api<{ rkey: string }>('/api/shares', {
            method: 'POST',
            body: {
                entrySlug: toggle.entrySlug,
                comment: trimmed || undefined,
            },
        })
            .then((payload) => {
                record.setActive(true);
                record.setRkey(payload.rkey);
                setComposerOpen(false);
            })
            .catch((err) => {
                setError(classifyShareError(err));
            })
            .finally(() => record.setBusy(false));
    };

    return {
        shared: record.active,
        busy: record.busy,
        canComment: toggle.canComment,
        composerOpen,
        error,
        onToggle,
        closeComposer,
        submit,
    };
}
