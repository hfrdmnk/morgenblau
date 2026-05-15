import { useEffect, useState } from 'react';

import { safeHref } from '@/lib/utils';

export type Me = {
    did: string;
    handle: string;
    avatar: string | null;
    displayName: string | null;
};

export type MeState =
    | { kind: 'loading' }
    | { kind: 'authed'; me: Me }
    | { kind: 'anon' };

type MeResponse = {
    did: string;
    handle: string;
    avatar?: string | null;
    displayName?: string | null;
};

export function useMe(): MeState {
    const [state, setState] = useState<MeState>({ kind: 'loading' });

    useEffect(() => {
        let cancelled = false;
        fetch('/api/me')
            .then((r) => (r.ok ? (r.json() as Promise<MeResponse>) : null))
            .then((data) => {
                if (cancelled) return;
                setState(
                    data
                        ? {
                              kind: 'authed',
                              me: {
                                  did: data.did,
                                  handle: data.handle,
                                  avatar: safeHref(data.avatar) ?? null,
                                  displayName: data.displayName ?? null,
                              },
                          }
                        : { kind: 'anon' },
                );
            })
            .catch(() => {
                if (!cancelled) setState({ kind: 'anon' });
            });
        return () => {
            cancelled = true;
        };
    }, []);

    return state;
}
