import { createContext, useContext } from 'react';

import type { Me } from '@/hooks/use-me';

// undefined means no provider above; null means the provider is mounted and me is still resolving.
export const MeContext = createContext<Me | null | undefined>(undefined);

export function useAuthedMe(): Me | null {
    const me = useContext(MeContext);
    if (me === undefined) {
        throw new Error('useAuthedMe must be used inside MeProvider');
    }
    return me;
}
