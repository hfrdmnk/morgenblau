import { useEffect } from 'react';

import { api } from '@/lib/api';

type JobStatus = 'pending' | 'running' | 'done' | 'failed';

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

export function useJobCompletion(
    jobId: string | null,
    onComplete: (status: 'done' | 'failed' | 'unknown') => void,
    intervalMs = 1500,
): void {
    useEffect(() => {
        if (!jobId) return;
        let cancelled = false;
        let timer: ReturnType<typeof setTimeout> | null = null;

        const tick = async () => {
            try {
                const job = await api<{ status: JobStatus }>(`/api/jobs/${jobId}`);
                if (cancelled) return;
                if (job.status === 'done' || job.status === 'failed') {
                    onComplete(job.status);
                    return;
                }
                timer = setTimeout(tick, intervalMs);
            } catch {
                if (!cancelled) onComplete('unknown');
            }
        };

        tick();
        return () => {
            cancelled = true;
            if (timer) clearTimeout(timer);
        };
    }, [jobId, onComplete, intervalMs]);
}
