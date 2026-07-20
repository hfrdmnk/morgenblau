import { useEffect } from 'react';

import { api } from '@/lib/api';

// Fires `onQuiet` on each "job exists" to "no jobs" transition; caller must flip `active` off (e.g. by re-fetching) to tear down the loop.
export function useJobsPoll(
    active: boolean,
    onQuiet: () => void,
    intervalMs = 1500,
): void {
    useEffect(() => {
        if (!active) return;
        let cancelled = false;
        let timer: ReturnType<typeof setTimeout> | null = null;
        let firedQuiet = false;

        const tick = async () => {
            try {
                const body = await api<unknown>('/api/jobs/active');
                if (cancelled) return;
                if (body === null) {
                    if (!firedQuiet) {
                        firedQuiet = true;
                        onQuiet();
                    }
                } else {
                    firedQuiet = false;
                }
                timer = setTimeout(tick, intervalMs);
            } catch {
                if (!cancelled) timer = setTimeout(tick, intervalMs);
            }
        };

        tick();
        return () => {
            cancelled = true;
            if (timer) clearTimeout(timer);
        };
    }, [active, onQuiet, intervalMs]);
}
