import { useEffect, useRef, useState } from 'react';

import { subscriptionChanges } from '@/lib/add-source';
import { api, connection } from '@/lib/api';
import { digestApiPath } from '@/lib/paths';

type Source = {
    kind?: string;
    id?: string;
    rkey?: string;
    title: string | null;
    feedUrl?: string;
    siteUrl: string | null;
    faviconUrl: string | null;
    primary?: boolean;
};

export type DigestEntry = {
    id: number | string;
    entrySlug: string;
    title: string | null;
    url?: string;
    body: string | null;
    image?: string;
    source: Source;
};

type DigestResponse = {
    date: string;
    entries: DigestEntry[];
    hasActiveJob: boolean;
};

type LoadState<T> = { status: 'loading' } | { status: 'loaded'; data: T } | { status: 'error' };

export function useDigest(date: string) {
    const [state, setState] = useState<LoadState<DigestResponse> & { date?: string }>({
        status: 'loading',
    });
    const [attempt, setAttempt] = useState(0);
    const collection = useRef({ date, active: false });

    useEffect(() => {
        const refresh = () => setAttempt((value) => value + 1);
        subscriptionChanges.addEventListener('change', refresh);
        return () => subscriptionChanges.removeEventListener('change', refresh);
    }, []);

    useEffect(() => {
        const controller = new AbortController();
        let timer: ReturnType<typeof setTimeout>;
        if (collection.current.date !== date) collection.current = { date, active: false };
        let collecting = collection.current.active;
        // Collection retries every second, doubling while Morgenblau is out of reach so an outage is not polled every second.
        let misses = 0;
        const retryDelay = () => Math.min(1000 * 2 ** Math.max(misses - 1, 0), 30_000);
        const path = digestApiPath(date, Intl.DateTimeFormat().resolvedOptions().timeZone);

        async function load() {
            try {
                const data = await api<DigestResponse>(path, { signal: controller.signal, offline: true });
                if (controller.signal.aborted) return;
                misses = connection.reachable ? 0 : misses + 1;
                setState({ status: 'loaded', data, date });
                collecting = data.hasActiveJob;
                collection.current.active = collecting;
                if (collecting) timer = setTimeout(load, retryDelay());
            } catch {
                if (controller.signal.aborted) return;
                misses++;
                if (collecting) timer = setTimeout(load, retryDelay());
                else setState({ status: 'error', date });
            }
        }
        void load();

        return () => {
            controller.abort();
            clearTimeout(timer);
        };
    }, [date, attempt]);

    function retry() {
        setState({ status: 'loading', date });
        setAttempt((value) => value + 1);
    }

    const current = state.date === date ? state : { status: 'loading' as const };
    return { ...current, retry };
}
