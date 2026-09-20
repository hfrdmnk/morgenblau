import { useState } from 'react';

import { optimisticDelete } from '@/lib/optimistic-delete';

type OptimisticRecordOptions = {
    initial: { id: string } | null;
    deletePath: (id: string) => string;
    onDeleteError?: (error: unknown) => void;
};

// remove() flips local record state before the DELETE lands and rolls back on failure.
export function useOptimisticRecord(options: OptimisticRecordOptions) {
    const [active, setActive] = useState(Boolean(options.initial));
    const [key, setKey] = useState<string | null>(
        options.initial?.id ?? null,
    );
    const [busy, setBusy] = useState(false);

    const remove = () => {
        if (!key) return;
        const previousKey = key;
        setBusy(true);
        optimisticDelete({
            path: options.deletePath(previousKey),
            clear: () => {
                setActive(false);
                setKey(null);
            },
            restore: () => {
                setActive(true);
                setKey(previousKey);
            },
            onError: options.onDeleteError,
            settle: () => setBusy(false),
        });
    };

    return { active, busy, setActive, setKey, setBusy, remove };
}
