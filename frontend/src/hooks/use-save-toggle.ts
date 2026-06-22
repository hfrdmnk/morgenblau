import { useState } from 'react';

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

type SaveStatus = 'idle' | 'saved';

// Owns the save state machine so the rail button and the reader's `b` shortcut
// share one source of truth and can't drift out of sync.
export function useSaveToggle(toggle: SavedToggle): SaveControl {
    const [status, setStatus] = useState<SaveStatus>(
        toggle.initial ? 'saved' : 'idle',
    );
    const [rkey, setRkey] = useState<string | null>(
        toggle.initial?.rkey ?? null,
    );
    const [busy, setBusy] = useState(false);

    const onToggle = () => {
        if (busy) return;
        if (status === 'saved') {
            if (!rkey) return;
            const previousRkey = rkey;
            setBusy(true);
            setStatus('idle');
            setRkey(null);
            fetch(`/api/saves/${encodeURIComponent(previousRkey)}`, {
                method: 'DELETE',
                credentials: 'same-origin',
            })
                .then((r) => {
                    if (!r.ok && r.status !== 204)
                        throw new Error(String(r.status));
                })
                .catch(() => {
                    setStatus('saved');
                    setRkey(previousRkey);
                })
                .finally(() => setBusy(false));
            return;
        }
        setBusy(true);
        setStatus('saved');
        fetch('/api/saves', {
            method: 'POST',
            credentials: 'same-origin',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                itemUrl: toggle.itemUrl,
                feedUrl: toggle.feedUrl ?? undefined,
            }),
        })
            .then(async (r) => {
                if (!r.ok) throw new Error(String(r.status));
                const payload = (await r.json()) as { rkey: string };
                setRkey(payload.rkey);
            })
            .catch(() => {
                setStatus('idle');
                setRkey(null);
            })
            .finally(() => setBusy(false));
    };

    return { saved: status === 'saved', busy, onToggle };
}
