import type { Dispatch, SetStateAction } from 'react';

import { useSubscribeTarget } from '@/hooks/use-subscribe-target';
import { api } from '@/lib/api';
import { discoverHidePayload, type DiscoverSourceCard } from '@/lib/discover';

export type DiscoverSourceListState<T> =
    | { kind: 'loading' }
    | {
          kind: 'ok';
          sources: T[];
          nextCursor?: string;
          loadingMore: boolean;
      }
    | { kind: 'error' };

function findSourceIndex<T extends { key: string }>(
    state: DiscoverSourceListState<T>,
    key: string,
): number {
    return state.kind === 'ok'
        ? state.sources.findIndex((s) => s.key === key)
        : -1;
}

// Subscribe-dialog and hide wiring shared by discover sources panels.
export function useDiscoverSourceActions<T extends DiscoverSourceCard>(
    state: DiscoverSourceListState<T>,
    setState: Dispatch<SetStateAction<DiscoverSourceListState<T>>>,
) {
    const dialog = useSubscribeTarget<T>();

    // Hides remove the card immediately; on failure it returns to its original position.
    const onHide = async (source: T) => {
        const index = findSourceIndex(state, source.key);
        if (index === -1) return;

        setState((prev) =>
            prev.kind === 'ok'
                ? {
                      ...prev,
                      sources: prev.sources.filter(
                          (s) => s.key !== source.key,
                      ),
                  }
                : prev,
        );

        try {
            await api('/api/discover/hides', {
                method: 'POST',
                body: discoverHidePayload(source),
            });
        } catch {
            setState((prev) => {
                if (prev.kind !== 'ok') return prev;
                const sources = prev.sources.slice();
                sources.splice(index, 0, source);
                return { ...prev, sources };
            });
        }
    };

    return {
        ...dialog,
        isSubscribed: (key: string) => dialog.subscribedKeys.has(key),
        onHide,
    };
}
