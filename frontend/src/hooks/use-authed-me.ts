import { createContext, useContext } from 'react';

import type { Me } from '@/hooks/use-me';

export const MeContext = createContext<Me | null>(null);

export function useAuthedMe(): Me {
    const me = useContext(MeContext);
    if (!me) throw new Error('useAuthedMe must be used inside MeProvider');
    return me;
}
