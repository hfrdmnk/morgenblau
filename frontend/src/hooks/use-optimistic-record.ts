import { useState } from 'react';

import { optimisticDelete } from '@/lib/optimistic-delete';

type OptimisticRecordOptions = {
    initial: { rkey: string } | null;
    deletePath: (rkey: string) => string;
    onDeleteError?: (error: unknown) => void;
};

// The rkey-backed record state shared by the save and share toggles: remove() flips state before the DELETE lands and rolls back on failure.
export function useOptimisticRecord(options: OptimisticRecordOptions) {
    const [active, setActive] = useState(Boolean(options.initial));
    const [rkey, setRkey] = useState<string | null>(
        options.initial?.rkey ?? null,
    );
    const [busy, setBusy] = useState(false);

    const remove = () => {
        if (!rkey) return;
        const previousRkey = rkey;
        setBusy(true);
        optimisticDelete({
            path: options.deletePath(previousRkey),
            clear: () => {
                setActive(false);
                setRkey(null);
            },
            restore: () => {
                setActive(true);
                setRkey(previousRkey);
            },
            onError: options.onDeleteError,
            settle: () => setBusy(false),
        });
    };

    return { active, busy, setActive, setRkey, setBusy, remove };
}
