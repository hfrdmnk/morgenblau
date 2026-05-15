import { Refresh01Icon } from '@hugeicons/core-free-icons';
import { HugeiconsIcon } from '@hugeicons/react';
import { useEffect, useState } from 'react';

import { RefreshPill } from '@/components/refresh-pill';
import { Button } from '@/components/ui/button';
import { useDocumentTitle } from '@/hooks/use-document-title';

type Entry = {
    id: number;
    title: string | null;
    url: string;
    contentType: string;
    publishedAt: string;
    source: {
        feedUrl: string;
        title: string | null;
        siteUrl: string | null;
    };
    body: string | null;
};

type DigestResponse = {
    date: string;
    entries: Entry[];
    hasActiveJob: boolean;
};

type State =
    | { kind: 'loading' }
    | { kind: 'ok'; data: DigestResponse }
    | { kind: 'error' };

export function Consume() {
    useDocumentTitle('Consume');

    const [state, setState] = useState<State>({ kind: 'loading' });
    const [triggerKey, setTriggerKey] = useState(0);
    const [refreshing, setRefreshing] = useState(false);
    const [jobInFlight, setJobInFlight] = useState(false);

    useEffect(() => {
        let cancelled = false;
        const load = async () => {
            try {
                const r = await fetch('/api/digest', {
                    credentials: 'same-origin',
                });
                if (!r.ok) throw new Error(String(r.status));
                const data = (await r.json()) as DigestResponse;
                if (!cancelled) setState({ kind: 'ok', data });
            } catch {
                if (!cancelled) setState({ kind: 'error' });
            }
        };
        load();
        return () => {
            cancelled = true;
        };
    }, [triggerKey, jobInFlight]);

    const onRefresh = async () => {
        if (refreshing) return;
        setRefreshing(true);
        try {
            await fetch('/api/digest/refresh', {
                method: 'POST',
                credentials: 'same-origin',
            });
            setTriggerKey((k) => k + 1);
        } catch {
            // Pill stays silent — calm-brand promise.
        } finally {
            setRefreshing(false);
        }
    };

    const hasEntries = state.kind === 'ok' && state.data.entries.length > 0;
    const serverFlaggedActive =
        state.kind === 'ok' && state.data.hasActiveJob;
    const inFlight = jobInFlight || serverFlaggedActive;

    return (
        <main className="relative flex min-h-full flex-col">
            <div className="flex items-center justify-end px-6 pt-4">
                <Button
                    variant="ghost"
                    size="icon-sm"
                    aria-label="Refresh"
                    onClick={onRefresh}
                    disabled={refreshing}
                >
                    <HugeiconsIcon icon={Refresh01Icon} className="size-5" />
                </Button>
            </div>

            <div className="flex-1 px-6 pb-12">
                {state.kind === 'loading' && (
                    <p className="pt-8 text-center text-muted-foreground">
                        Loading…
                    </p>
                )}
                {state.kind === 'error' && (
                    <p className="pt-8 text-center text-muted-foreground">
                        Couldn’t load the digest.
                    </p>
                )}
                {state.kind === 'ok' && hasEntries && (
                    <ul className="mx-auto flex max-w-2xl flex-col gap-4 pt-4">
                        {state.data.entries.map((entry) => (
                            <EntryCard key={entry.id} entry={entry} />
                        ))}
                    </ul>
                )}
                {state.kind === 'ok' && !hasEntries && (
                    <div className="flex min-h-[40dvh] items-center justify-center">
                        <p className="text-lg text-muted-foreground">
                            {inFlight
                                ? 'Brewing your first edition…'
                                : 'Nothing new this morning. Enjoy your coffee.'}
                        </p>
                    </div>
                )}
            </div>

            <RefreshPill
                triggerKey={triggerKey}
                onActiveChange={setJobInFlight}
            />
        </main>
    );
}

function EntryCard({ entry }: { entry: Entry }) {
    const sourceTitle = entry.source.title ?? entry.source.feedUrl;
    const published = formatDate(entry.publishedAt);
    const readerHref = `/entry?id=${entry.id}`;

    return (
        <li className="rounded-xl border border-border bg-card p-4 hover:bg-muted/40">
            <a href={readerHref}>
                <div className="flex items-baseline justify-between gap-3">
                    <span className="truncate text-xs uppercase tracking-wide text-muted-foreground">
                        {sourceTitle}
                    </span>
                    <span className="shrink-0 text-xs text-muted-foreground">
                        {published}
                    </span>
                </div>
                <h2 className="mt-1 text-base font-medium text-foreground">
                    {entry.title || entry.url}
                </h2>
            </a>
        </li>
    );
}

function formatDate(iso: string): string {
    try {
        const d = new Date(iso);
        return d.toLocaleTimeString(undefined, {
            hour: '2-digit',
            minute: '2-digit',
        });
    } catch {
        return iso;
    }
}
