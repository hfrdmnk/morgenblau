import { useCallback, useState } from 'react';

import type { Entry } from '@/components/digest-rows';
import { entryActivation } from '@/lib/entry-nav';
import type { EntryFrom } from '@/lib/paths';

export type ListNavigation = {
    selected: number | null;
    move: (delta: number) => void;
    open: () => void;
    clear: () => void;
};

export function useListNavigation(
    entries: Entry[],
    from?: EntryFrom,
): ListNavigation {
    const [selected, setSelected] = useState<number | null>(null);

    // Drop the selection when the list changes (day switch, refetch). Adjusting
    // during render avoids a setState-in-effect cascade.
    const [prevEntries, setPrevEntries] = useState(entries);
    if (entries !== prevEntries) {
        setPrevEntries(entries);
        setSelected(null);
    }

    const move = useCallback(
        (delta: number) => {
            setSelected((current) => {
                if (entries.length === 0) return null;
                if (current === null) {
                    return delta > 0 ? 0 : entries.length - 1;
                }
                const next = current + delta;
                if (next < 0) return 0;
                if (next > entries.length - 1) return entries.length - 1;
                return next;
            });
        },
        [entries.length],
    );

    const open = useCallback(() => {
        if (selected === null) return;
        const entry = entries[selected];
        if (!entry) return;
        const target = entryActivation(entry, from);
        if (!target) return;
        if (target.external) {
            window.open(target.href, '_blank', 'noopener,noreferrer');
        } else {
            window.location.href = target.href;
        }
    }, [entries, selected, from]);

    const clear = useCallback(() => setSelected(null), []);

    return { selected, move, open, clear };
}
