import { useEffect, useState } from 'react';

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

    useEffect(() => {
        const controller = new AbortController();
        const timezone = Intl.DateTimeFormat().resolvedOptions().timeZone;
        const params = new URLSearchParams({ date, timezone });

        api<DigestResponse>(`/api/digest?${params}`, { signal: controller.signal })
            .then((data) => setState({ status: 'loaded', data, date }))
            .catch((error: unknown) => {
                if (!(error instanceof DOMException && error.name === 'AbortError')) {
                    setState({ status: 'error', date });
                }
            });

        return () => controller.abort();
    }, [date]);

    return state.date === date ? state : { status: 'loading' as const };
}
