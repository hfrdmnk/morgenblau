import { useCallback } from 'react';
import { useLocation } from 'wouter';

import type { Entry } from '@/components/digest-rows';
import type { KeyMap } from '@/hooks/use-keyboard';
import { useListNavKeyboard } from '@/hooks/use-list-nav-keyboard';
import {
    useListNavigation,
    type ListNavigation,
} from '@/hooks/use-list-navigation';
import { entryActivation } from '@/lib/entry-nav';
import type { EntryFrom } from '@/lib/paths';

// Shared by digest and source list views so row-open behavior and keyboard wiring can't diverge; callers layer extra keys on top.
export function useEntryNavigation(
    entries: Entry[],
    entryFrom: EntryFrom | undefined,
    extraKeys?: KeyMap,
): ListNavigation {
    const [, navigate] = useLocation();
    const onOpen = useCallback(
        (entry: Entry) => {
            const target = entryActivation(entry, entryFrom);
            if (!target) return;
            if (target.external) {
                window.open(target.href, '_blank', 'noopener,noreferrer');
            } else {
                navigate(target.href);
            }
        },
        [entryFrom, navigate],
    );
    const nav = useListNavigation(entries, onOpen);
    useListNavKeyboard(nav, extraKeys);

    return nav;
}
