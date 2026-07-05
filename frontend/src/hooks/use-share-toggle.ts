import { useState } from 'react';

export type ShareToggle = {
    initial: { rkey: string } | null;
    entrySlug: string;
    // A path-less Standardfeed document can't take a comment (the server 422s),
    // so the composer hides the note field for it.
    canComment: boolean;
};

export type ShareError = 'reauth' | 'failed';

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

type ShareStatus = 'idle' | 'shared';

// Owns the share state machine. Sharing opens a composer (optional comment)
// then POSTs; un-sharing is a direct optimistic DELETE, mirroring the save
// toggle so the rail button and any future shortcut share one source of truth.
export function useShareToggle(toggle: ShareToggle): ShareControl {
    const [status, setStatus] = useState<ShareStatus>(
        toggle.initial ? 'shared' : 'idle',
    );
    const [rkey, setRkey] = useState<string | null>(
        toggle.initial?.rkey ?? null,
    );
    const [busy, setBusy] = useState(false);
    const [composerOpen, setComposerOpen] = useState(false);
    const [error, setError] = useState<ShareError | null>(null);

    const closeComposer = () => setComposerOpen(false);

    const onToggle = () => {
        if (busy) return;
        if (status === 'shared') {
            if (!rkey) return;
            const previousRkey = rkey;
            setBusy(true);
            setStatus('idle');
            setRkey(null);
            fetch(`/api/shares/${encodeURIComponent(previousRkey)}`, {
                method: 'DELETE',
                credentials: 'same-origin',
            })
                .then((r) => {
                    if (!r.ok && r.status !== 204)
                        throw new Error(String(r.status));
                })
                .catch(() => {
                    setStatus('shared');
                    setRkey(previousRkey);
                })
                .finally(() => setBusy(false));
            return;
        }
        setError(null);
        setComposerOpen(true);
    };

    const submit = (comment: string) => {
        if (busy) return;
        setBusy(true);
        setError(null);
        const trimmed = comment.trim();
        fetch('/api/shares', {
            method: 'POST',
            credentials: 'same-origin',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                entrySlug: toggle.entrySlug,
                comment: trimmed || undefined,
            }),
        })
            .then(async (r) => {
                if (r.status === 403) {
                    const payload = (await r.json().catch(() => null)) as {
                        code?: string;
                    } | null;
                    if (payload?.code === 'reauth_required') {
                        setError('reauth');
                        return;
                    }
                    setError('failed');
                    return;
                }
                if (!r.ok) {
                    setError('failed');
                    return;
                }
                const payload = (await r.json()) as { rkey: string };
                setStatus('shared');
                setRkey(payload.rkey);
                setComposerOpen(false);
            })
            .catch(() => setError('failed'))
            .finally(() => setBusy(false));
    };

    return {
        shared: status === 'shared',
        busy,
        canComment: toggle.canComment,
        composerOpen,
        error,
        onToggle,
        closeComposer,
        submit,
    };
}
