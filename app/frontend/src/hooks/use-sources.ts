import { useEffect, useState } from 'react';

import { subscriptionChanges, type GeneratedHealth } from '@/lib/add-source';
import { api, ApiError } from '@/lib/api';

export type FeedSource = {
    rkey: string;
    title?: string;
    feedUrl: string;
    siteUrl?: string;
    faviconUrl?: string;
    primary: boolean;
    tags?: string[];
    muted?: boolean;
    fetchStatus: 'waiting' | 'ready' | 'unavailable';
    lastFetchedAt?: string;
    nextFetchAt?: string;
    generated?: { health: GeneratedHealth; healthReason: string };
};

export type FeedSourceDetail = FeedSource & {
    frequency?: string;
    lastPublishedAt?: string;
    totalEntries: number;
};

export type NewsletterSource = {
    id: string;
    title: string;
    senderAddress: string;
    primary: boolean;
    tags: string[];
};

export type NewsletterSourceDetail = NewsletterSource & {
    senderName: string | null;
    status: 'active' | 'stopped';
    lastReceivedAt: string | null;
    frequency: string;
    issueCount: number;
};

export type SourceEntry = {
    id: number | string;
    entrySlug: string;
    title: string | null;
    url?: string;
    contentType: string;
    publishedAt: string;
};

type State<T> =
    | { status: 'loading' }
    | { status: 'loaded'; data: T }
    | { status: 'error'; notFound?: boolean };

export function useSources<T>(path: string, pollInterval = 0) {
    // Keyed by path so a page reused for another source never shows the previous one's data.
    const [state, setState] = useState<State<T> & { path?: string }>({ status: 'loading' });
    const [attempt, setAttempt] = useState(0);

    useEffect(() => {
        const controller = new AbortController();
        let timer: ReturnType<typeof setTimeout>;
        const refresh = () => setAttempt((value) => value + 1);
        if (path === '/api/subscriptions') {
            subscriptionChanges.addEventListener('change', refresh);
        }
        async function load() {
            try {
                const data = await api<T>(path, { signal: controller.signal });
                if (!controller.signal.aborted) setState({ status: 'loaded', data, path });
            } catch (error) {
                if (!controller.signal.aborted) {
                    const notFound = error instanceof ApiError && error.status === 404;
                    setState((previous) => previous.status === 'loaded' && previous.path === path && !notFound
                        ? previous
                        : { status: 'error', notFound, path });
                }
            } finally {
                if (pollInterval > 0 && !controller.signal.aborted) {
                    timer = setTimeout(load, pollInterval);
                }
            }
        }
        void load();
        return () => {
            controller.abort();
            clearTimeout(timer);
            subscriptionChanges.removeEventListener('change', refresh);
        };
    }, [path, attempt, pollInterval]);

    function retry() {
        setState({ status: 'loading', path });
        refresh();
    }

    function refresh() {
        setAttempt((value) => value + 1);
    }

    const current: State<T> = state.path === path ? state : { status: 'loading' };
    return { ...current, retry, refresh };
}
