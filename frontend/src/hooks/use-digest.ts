import { useEffect, useRef, useState } from 'react';

import { subscriptionChanges } from '@/lib/add-source';
import { api } from '@/lib/api';

type Source = {
    title: string | null;
    feedUrl?: string;
    siteUrl: string | null;
    faviconUrl: string | null;
};

export type DigestEntry = {
    id: number | string;
    entrySlug: string;
    title: string | null;
    url?: string;
    source: Source;
};

type DigestResponse = {
    date: string;
    entries: DigestEntry[];
    hasActiveJob: boolean;
};

type LoadState<T> =
    | { status: 'loading' }
    | { status: 'loaded'; data: T }
    | { status: 'error' };

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
        const timezone = Intl.DateTimeFormat().resolvedOptions().timeZone;
        const params = new URLSearchParams({ date, timezone });

        async function load() {
            try {
                const data = await api<DigestResponse>(`/api/digest?${params}`, { signal: controller.signal });
                if (controller.signal.aborted) return;
                setState({ status: 'loaded', data, date });
                collecting = data.hasActiveJob;
                collection.current.active = collecting;
                if (collecting) timer = setTimeout(load, 1000);
            } catch {
                if (controller.signal.aborted) return;
                if (collecting) timer = setTimeout(load, 1000);
                else setState({ status: 'error', date });
            }
        }
        void load();

        return () => {
            controller.abort();
            clearTimeout(timer);
        };
    }, [date, attempt]);

    return state.date === date ? state : { status: 'loading' as const };
}
