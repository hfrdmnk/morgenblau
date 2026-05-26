import { useCallback, useEffect, useMemo, useState } from 'react';

import { CalendarStrip } from '@/components/calendar-strip';
import { Newspaper } from '@/components/digest-rows';
import type { Entry } from '@/components/digest-rows';
import { Skeleton } from '@/components/ui/skeleton';
import { useRegisterChromeRefresh } from '@/hooks/use-chrome-refresh';
import { useDocumentTitle } from '@/hooks/use-document-title';
import { useJobsPoll } from '@/hooks/use-jobs-poll';
import {
    formatISODate,
    isSameDay,
    parseISODate,
    startOfLocalDay,
} from '@/lib/date';
import { subscribeSubscriptionAdded } from '@/lib/subscription-events';

type DigestResponse = {
    date: string;
    entries: Entry[];
    hasActiveJob: boolean;
};

type State =
    | { kind: 'loading' }
    | { kind: 'ok'; entries: Entry[]; hasActiveJob: boolean }
    | { kind: 'error' };

export function Digest() {
    useDocumentTitle('Digest');
    const today = useMemo(() => startOfLocalDay(new Date()), []);
    const [selectedDate, setSelectedDate] = useState<Date>(() =>
        readDateFromURL(today),
    );
    const [state, setState] = useState<State>({ kind: 'loading' });
    const [reloadTick, setReloadTick] = useState(0);
    const [refreshing, setRefreshing] = useState(false);

    // Clean up the URL once on mount if the inbound date param was invalid
    // or in the future. readDateFromURL already clamped to today.
    useEffect(() => {
        const raw = new URLSearchParams(window.location.search).get('date');
        if (raw === null) return;
        const parsed = parseISODate(raw);
        if (parsed && parsed.getTime() <= today.getTime()) return;
        const url = new URL(window.location.href);
        url.searchParams.delete('date');
        window.history.replaceState(null, '', url.toString());
    }, [today]);

    useEffect(() => {
        const onPopState = () => {
            setSelectedDate(readDateFromURL(today));
        };
        window.addEventListener('popstate', onPopState);
        return () => window.removeEventListener('popstate', onPopState);
    }, [today]);

    useEffect(() => {
        let cancelled = false;
        const load = async () => {
            try {
                const url = `/api/digest?date=${encodeURIComponent(formatISODate(selectedDate))}`;
                const r = await fetch(url, { credentials: 'same-origin' });
                if (!r.ok) throw new Error(String(r.status));
                const data = (await r.json()) as DigestResponse;
                if (cancelled) return;
                setState({
                    kind: 'ok',
                    entries: data.entries,
                    hasActiveJob: data.hasActiveJob,
                });
            } catch {
                if (!cancelled) setState({ kind: 'error' });
            } finally {
                if (!cancelled) setRefreshing(false);
            }
        };
        load();
        return () => {
            cancelled = true;
        };
    }, [reloadTick, selectedDate]);

    const handleSelectDate = useCallback(
        (date: Date) => {
            setSelectedDate(date);
            const url = new URL(window.location.href);
            if (isSameDay(date, today)) {
                url.searchParams.delete('date');
            } else {
                url.searchParams.set('date', formatISODate(date));
            }
            window.history.pushState(null, '', url.toString());
        },
        [today],
    );

    const onRefresh = useCallback(async () => {
        setRefreshing(true);
        try {
            await fetch('/api/digest/refresh', {
                method: 'POST',
                credentials: 'same-origin',
            });
        } catch {
            // The poll/reload below will surface fetch outcomes; ignore here.
        }
        setReloadTick((tick) => tick + 1);
    }, []);

    useEffect(() => {
        return subscribeSubscriptionAdded(() => {
            setReloadTick((tick) => tick + 1);
        });
    }, []);

    const hasActiveJob = state.kind === 'ok' && state.hasActiveJob;
    const onQuiet = useCallback(() => {
        setReloadTick((tick) => tick + 1);
    }, []);
    useJobsPoll(hasActiveJob, onQuiet);

    const isBusy = state.kind === 'loading' || refreshing || hasActiveJob;
    useRegisterChromeRefresh(onRefresh, isBusy);

    return (
        <>
            <CalendarStrip
                selected={selectedDate}
                today={today}
                onSelect={handleSelectDate}
            />
            <div className="mx-auto w-full max-w-2xl px-4 pb-12 sm:px-6">
                {isBusy ? (
                    <DigestSkeleton />
                ) : state.kind === 'error' ? (
                    <EmptyMessage
                        lead="Couldn't load the digest."
                        detail="Try refreshing in a moment."
                    />
                ) : state.entries.length === 0 ? (
                    <EmptyMessage
                        lead={
                            state.hasActiveJob
                                ? 'Brewing your first edition…'
                                : 'Nothing new this morning.'
                        }
                        detail={
                            state.hasActiveJob
                                ? "This won't take long."
                                : 'Enjoy your coffee.'
                        }
                    />
                ) : (
                    <Newspaper
                        entries={state.entries}
                        fromDate={
                            isSameDay(selectedDate, today)
                                ? undefined
                                : formatISODate(selectedDate)
                        }
                    />
                )}
            </div>
        </>
    );
}

function readDateFromURL(today: Date): Date {
    const raw = new URLSearchParams(window.location.search).get('date');
    if (!raw) return today;
    const parsed = parseISODate(raw);
    if (!parsed) return today;
    if (parsed.getTime() > today.getTime()) return today;
    return parsed;
}

function DigestSkeleton() {
    return (
        <article
            aria-busy
            aria-label="Loading digest"
            className="overflow-hidden rounded-3xl border border-gray-100 bg-card dark:border-gray-700"
        >
            <ul className="flex flex-col">
                {Array.from({ length: 6 }).map((_, index) => (
                    <li key={index}>
                        {index > 0 ? (
                            <div
                                aria-hidden
                                className="mx-6 border-t border-gray-100 dark:border-gray-700"
                            />
                        ) : null}
                        <div className="flex flex-col gap-2 px-6 py-5">
                            <div className="flex items-center gap-2">
                                <Skeleton className="size-4 rounded-sm" />
                                <Skeleton className="h-3 w-32" />
                            </div>
                            <Skeleton className="h-5 w-3/4" />
                            <Skeleton className="h-3 w-11/12" />
                            <Skeleton className="h-3 w-2/3" />
                        </div>
                    </li>
                ))}
            </ul>
        </article>
    );
}

function EmptyMessage({ lead, detail }: { lead: string; detail: string }) {
    return (
        <div className="flex flex-col gap-2">
            <p>{lead}</p>
            <p className="text-sm font-light text-muted-foreground">{detail}</p>
        </div>
    );
}
