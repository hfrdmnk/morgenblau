import { useEffect, useState } from 'react';

import { api } from '@/lib/api';

export type FeedSource = {
    rkey: string;
    title?: string;
    feedUrl: string;
    siteUrl?: string;
    faviconUrl?: string;
    primary: boolean;
    muted?: boolean;
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

export function useSources<T>(path: string) {
    const [state, setState] = useState<State<T>>({ status: 'loading' });
    const [attempt, setAttempt] = useState(0);

    useEffect(() => {
        const controller = new AbortController();
        api<T>(path, { signal: controller.signal })
            .then((data) => {
                if (!controller.signal.aborted) setState({ status: 'loaded', data });
            })
            .catch(() => {
                if (!controller.signal.aborted) setState({ status: 'error' });
            });
        return () => controller.abort();
    }, [path, attempt]);

    function retry() {
        setState({ status: 'loading' });
        setAttempt((value) => value + 1);
    }

    return { ...state, retry };
}
