import type { ReactNode } from 'react';
import { useEffect } from 'react';

import { MeContext } from '@/hooks/use-authed-me';
import { useMe } from '@/hooks/use-me';
import { PATHS } from '@/lib/paths';

// Children render straight away so a slow /api/profiles/me can't hold the whole app on a blank screen.
export function MeProvider({ children }: { children: ReactNode }) {
    const state = useMe();

    // The Go middleware gates these pages server-side, so anon here means the session died mid-flight.
    useEffect(() => {
        if (state.kind === 'anon') window.location.assign(PATHS.login);
    }, [state.kind]);

    return (
        <MeContext.Provider value={state.kind === 'authed' ? state.me : null}>
            {children}
        </MeContext.Provider>
    );
}
