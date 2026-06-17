import { useMemo, useState, type ReactNode } from 'react';

import {
    ChromeActionsContext,
    type CalendarNav,
    type RefreshAction,
} from '@/hooks/use-chrome-refresh';

export function ChromeActionsProvider({ children }: { children: ReactNode }) {
    const [refresh, setRefresh] = useState<RefreshAction | null>(null);
    const [calendar, setCalendar] = useState<CalendarNav | null>(null);
    const value = useMemo(
        () => ({ refresh, setRefresh, calendar, setCalendar }),
        [refresh, calendar],
    );
    return (
        <ChromeActionsContext.Provider value={value}>
            {children}
        </ChromeActionsContext.Provider>
    );
}
