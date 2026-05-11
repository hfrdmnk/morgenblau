import { useEffect, useState } from 'react';

import { status as digestStatus } from '@/routes/digest';

type DigestStatus = App.Data.Digest.DigestStatusData;

const POLL_INTERVAL_MS = 3000;
const MAX_DURATION_MS = 5 * 60 * 1000;

export type DigestStatusState =
    | { phase: 'idle' }
    | { phase: 'fetching' }
    | { phase: 'ready'; newCount: number; latestEntryAt: string | null }
    | { phase: 'caught_up' };

type InternalResult =
    | { phase: 'fetching' }
    | { phase: 'ready'; newCount: number; latestEntryAt: string | null }
    | { phase: 'caught_up' };

type Snapshot = { since: string; result: InternalResult };

export function useDigestStatus(
    pollingSince: string | null,
): DigestStatusState {
    const [snapshot, setSnapshot] = useState<Snapshot | null>(null);

    useEffect(() => {
        if (!pollingSince) {
            return;
        }

        const startedAt = Date.now();
        let timer: ReturnType<typeof setTimeout> | null = null;
        let cancelled = false;

        const record = (result: InternalResult) => {
            setSnapshot({ since: pollingSince, result });
        };

        const tick = async () => {
            if (cancelled || Date.now() - startedAt > MAX_DURATION_MS) {
                return;
            }

            try {
                const res = await fetch(
                    digestStatus({ query: { since: pollingSince } }).url,
                    {
                        headers: { Accept: 'application/json' },
                        credentials: 'same-origin',
                    },
                );

                if (cancelled) {
                    return;
                }

                if (!res.ok) {
                    timer = setTimeout(tick, POLL_INTERVAL_MS);

                    return;
                }

                const data: DigestStatus = await res.json();

                if (cancelled) {
                    return;
                }

                if (data.pending) {
                    record({ phase: 'fetching' });
                    timer = setTimeout(tick, POLL_INTERVAL_MS);

                    return;
                }

                if (data.new_count > 0) {
                    record({
                        phase: 'ready',
                        newCount: data.new_count,
                        latestEntryAt: data.latest_entry_at,
                    });
                } else {
                    record({ phase: 'caught_up' });
                }
            } catch {
                if (!cancelled) {
                    timer = setTimeout(tick, POLL_INTERVAL_MS);
                }
            }
        };

        tick();

        return () => {
            cancelled = true;

            if (timer) {
                clearTimeout(timer);
            }
        };
    }, [pollingSince]);

    if (!pollingSince) {
        return { phase: 'idle' };
    }

    if (snapshot && snapshot.since === pollingSince) {
        return snapshot.result;
    }

    return { phase: 'fetching' };
}
