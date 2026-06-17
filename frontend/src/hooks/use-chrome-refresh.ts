import { createContext, useContext, useEffect } from 'react';

export type RefreshAction = { onRefresh: () => void; busy: boolean };

export type CalendarNav = {
    selected: Date;
    today: Date;
    onSelect: (date: Date) => void;
};

export type ChromeActionsContextValue = {
    refresh: RefreshAction | null;
    setRefresh: (action: RefreshAction | null) => void;
    calendar: CalendarNav | null;
    setCalendar: (nav: CalendarNav | null) => void;
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

export function useChromeCalendar(): CalendarNav | null {
    return useContext(ChromeActionsContext)?.calendar ?? null;
}

export function useRegisterChromeCalendar(nav: CalendarNav) {
    const setCalendar = useContext(ChromeActionsContext)?.setCalendar;
    const { selected, today, onSelect } = nav;
    useEffect(() => {
        if (!setCalendar) return;
        setCalendar({ selected, today, onSelect });
        return () => setCalendar(null);
    }, [setCalendar, selected, today, onSelect]);
}
