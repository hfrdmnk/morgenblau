import { createContext, useContext, useEffect } from 'react';

export type RefreshAction = { onRefresh: () => void; busy: boolean };

export type ChromeActionsContextValue = {
    refresh: RefreshAction | null;
    setRefresh: (action: RefreshAction | null) => void;
};

export const ChromeActionsContext =
    createContext<ChromeActionsContextValue | null>(null);

export function useChromeRefresh(): RefreshAction | null {
    return useContext(ChromeActionsContext)?.refresh ?? null;
}

export function useRegisterChromeRefresh(
    onRefresh: () => void,
    busy: boolean,
) {
    const setRefresh = useContext(ChromeActionsContext)?.setRefresh;
    useEffect(() => {
        if (!setRefresh) return;
        setRefresh({ onRefresh, busy });
        return () => setRefresh(null);
    }, [setRefresh, onRefresh, busy]);
}
