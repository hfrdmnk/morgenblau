import { useEffect, useState } from 'react';

type ActiveJob = {
    id: string;
    kind: string;
    status: 'pending' | 'running' | 'done' | 'failed';
} | null;

type Props = {
    // Bumped by the page when it kicks off a refresh; forces an immediate
    // poll so the pill appears without waiting for the next tick.
    triggerKey?: number;
    onActiveChange?: (active: boolean) => void;
    pollIntervalMs?: number;
};

// RefreshPill polls /api/jobs/active and renders a transient pill while a
// user-initiated job is in flight. Per SPEC <feed-sources> it never quantifies
// pending entries with a count.
export function RefreshPill({
    triggerKey = 0,
    onActiveChange,
    pollIntervalMs = 1500,
}: Props) {
    const [active, setActive] = useState<ActiveJob>(null);

    useEffect(() => {
        let cancelled = false;
        let timer: number | undefined;

        const tick = async () => {
            try {
                const r = await fetch('/api/jobs/active', {
                    credentials: 'same-origin',
                });
                if (!r.ok) {
                    if (!cancelled) setActive(null);
                    return;
                }
                const data = (await r.json()) as ActiveJob;
                if (!cancelled) setActive(data);
            } catch {
                if (!cancelled) setActive(null);
            }
        };

        // Poll immediately on mount and on each triggerKey change.
        tick();
        timer = window.setInterval(tick, pollIntervalMs);
        return () => {
            cancelled = true;
            if (timer) window.clearInterval(timer);
        };
    }, [triggerKey, pollIntervalMs]);

    useEffect(() => {
        onActiveChange?.(active !== null);
    }, [active, onActiveChange]);

    if (!active) return null;

    return (
        <div
            role="status"
            aria-live="polite"
            className="pointer-events-none fixed bottom-6 left-1/2 z-50 -translate-x-1/2 select-none rounded-full border border-border bg-muted/90 px-4 py-1.5 text-sm text-foreground shadow-sm backdrop-blur"
        >
            Refreshing your sources…
        </div>
    );
}
