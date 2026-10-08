import { useEffect, useState } from 'react';

import { subscriptionChanges } from '@/lib/add-source';
import { api } from '@/lib/api';

export type FeedSource = {
    rkey: string;
    title?: string;
    feedUrl: string;
    siteUrl?: string;
    faviconUrl?: string;
    primary: boolean;
    muted?: boolean;
    fetchStatus: 'waiting' | 'ready' | 'unavailable';
    lastFetchedAt?: string;
    nextFetchAt?: string;
};

export type NewsletterSource = {
    id: string;
    title: string;
    senderAddress: string;
    primary: boolean;
};

type State<T> =
    | { status: 'loading' }
    | { status: 'loaded'; data: T }
    | { status: 'error' };

export function useSources<T>(path: string, pollInterval = 0) {
    const [state, setState] = useState<State<T>>({ status: 'loading' });
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
                if (!controller.signal.aborted) setState({ status: 'loaded', data });
            } catch {
                if (!controller.signal.aborted) {
                    setState((previous) => previous.status === 'loaded' ? previous : { status: 'error' });
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
        setState({ status: 'loading' });
        setAttempt((value) => value + 1);
    }

    return { ...state, retry };
}
