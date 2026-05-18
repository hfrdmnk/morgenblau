import { useMemo, useState, type ReactNode } from 'react';

import {
    ChromeActionsContext,
    type RefreshAction,
} from '@/hooks/use-chrome-refresh';

export function ChromeActionsProvider({ children }: { children: ReactNode }) {
    const [refresh, setRefresh] = useState<RefreshAction | null>(null);
    const value = useMemo(() => ({ refresh, setRefresh }), [refresh]);
    return (
        <ChromeActionsContext.Provider value={value}>
            {children}
        </ChromeActionsContext.Provider>
    );
}
