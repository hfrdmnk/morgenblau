import type { ReactNode } from 'react';

import { MeContext } from '@/hooks/use-authed-me';
import { useMe } from '@/hooks/use-me';
import { PATHS } from '@/lib/paths';

// The Go middleware gates the page server-side, so by the time we mount here
// we expect to be authed. The 'anon' branch only fires if the session died
// between the server gate and the /api/profiles/me fetch, so defensively reload.
export function MeProvider({ children }: { children: ReactNode }) {
    const state = useMe();

    if (state.kind === 'loading') {
        return <div className="min-h-svh bg-background" />;
    }
    if (state.kind === 'anon') {
        window.location.assign(PATHS.login);
        return null;
    }

    return (
        <MeContext.Provider value={state.me}>{children}</MeContext.Provider>
    );
}
