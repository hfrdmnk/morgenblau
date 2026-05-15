import { useEffect, useState } from 'react';

import { useDocumentTitle } from '@/hooks/use-document-title';
import { safeHref } from '@/lib/utils';

type Subscription = {
    uri: string;
    cid: string;
    value: {
        title?: string;
        feedUrl?: string;
        siteUrl?: string;
        [k: string]: unknown;
    };
};

type State =
    | { kind: 'loading' }
    | { kind: 'ok'; records: Subscription[] }
    | { kind: 'error' };

export function Sources() {
    useDocumentTitle('Sources');
    const [state, setState] = useState<State>({ kind: 'loading' });

    useEffect(() => {
        let cancelled = false;
        fetch('/api/subscriptions')
            .then((r) => {
                if (!r.ok) throw new Error(String(r.status));
                return r.json();
            })
            .then((records: Subscription[]) => {
                if (!cancelled) setState({ kind: 'ok', records });
            })
            .catch(() => {
                if (!cancelled) setState({ kind: 'error' });
            });
        return () => {
            cancelled = true;
        };
    }, []);

    if (state.kind === 'loading') {
        return (
            <main className="p-8">
                <p className="text-muted-foreground">Loading…</p>
            </main>
        );
    }
    if (state.kind === 'error') {
        return (
            <main className="p-8">
                <p className="text-muted-foreground">
                    Could not load your sources.
                </p>
            </main>
        );
    }
    if (state.records.length === 0) {
        return (
            <main className="p-8">
                <p className="text-muted-foreground">Add your first source.</p>
            </main>
        );
    }
    return (
        <main className="p-8">
            <ul className="space-y-2">
                {state.records.map((r) => {
                    const href = safeHref(r.value.siteUrl);
                    const label =
                        r.value.title || r.value.feedUrl || r.uri;

                    return (
                        <li key={r.uri}>
                            {href ? (
                                <a
                                    href={href}
                                    target="_blank"
                                    rel="noopener noreferrer"
                                    className="text-primary underline-offset-4 hover:underline"
                                >
                                    {label}
                                </a>
                            ) : (
                                <span>{label}</span>
                            )}
                        </li>
                    );
                })}
            </ul>
        </main>
    );
}
