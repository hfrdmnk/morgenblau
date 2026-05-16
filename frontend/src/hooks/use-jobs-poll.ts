import { useEffect } from 'react';

// Polls /api/jobs/active while `active` is true. Fires `onQuiet` on every
// transition from "a job exists" to "no jobs"; the caller is responsible for
// flipping `active` off (typically by re-fetching state that derives it) so
// the loop tears down.
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
                const r = await fetch('/api/jobs/active', {
                    credentials: 'same-origin',
                });
                if (cancelled) return;
                if (!r.ok) {
                    timer = setTimeout(tick, intervalMs);
                    return;
                }
                const body = (await r.json().catch(() => null)) as unknown;
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
