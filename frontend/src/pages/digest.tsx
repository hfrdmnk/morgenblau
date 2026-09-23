import { useCallback, useEffect, useMemo, useState } from 'react';
import { toast } from 'sonner';

import { Newspaper } from '@/components/digest-rows';
import type { Entry } from '@/components/digest-rows';
import { Skeleton } from '@/components/ui/skeleton';
import {
    useRegisterChromeCalendar,
    useRegisterChromeRefresh,
} from '@/hooks/use-chrome-refresh';
import { useDocumentTitle } from '@/hooks/use-document-title';
import { useEntryNavigation } from '@/hooks/use-entry-navigation';
import { useJobCompletion } from '@/hooks/use-jobs-poll';
import { api } from '@/lib/api';
import {
    addDays,
    formatISODate,
    isSameDay,
    parseISODate,
    startOfLocalDay,
} from '@/lib/date';
import { digestRequestPath } from '@/lib/digest';
import { emitLibraryMutation } from '@/lib/library-events';
import { toastMutationError } from '@/lib/mutation-toast';
import { subscribeSubscriptionAdded } from '@/lib/subscription-events';

type DigestResponse = {
    date: string;
    entries: Entry[];
};

// Stable empty list so list navigation doesn't reset every render while loading.
const EMPTY_ENTRIES: Entry[] = [];

type State =
    | { kind: 'loading' }
    | { kind: 'ok'; entries: Entry[] }
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
    const [manualJobId, setManualJobId] = useState<string | null>(null);

    // Clean up the URL once on mount if the date param was invalid or in the future; readDateFromURL already clamped it.
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
                const url = digestRequestPath(selectedDate);
                const data = await api<DigestResponse>(url);
                if (cancelled) return;
                setState({
                    kind: 'ok',
                    entries: data.entries,
                });
            } catch {
                if (!cancelled) setState({ kind: 'error' });
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
            const { jobId } = await api<{ jobId: string }>(
                '/api/digest/refresh',
                { method: 'POST' },
            );
            setManualJobId(jobId);
        } catch (error) {
            toastMutationError(error, "Couldn't start the refresh. Try again.");
            setRefreshing(false);
        }
    }, []);

    const onManualComplete = useCallback((status: 'done' | 'failed' | 'unknown') => {
        setManualJobId(null);
        setRefreshing(false);
        if (status === 'unknown') {
            toast.error("Couldn't check the refresh. Try again in a moment.");
            return;
        }
        if (status === 'failed') {
            toast.error("Couldn't finish the refresh. Try again.");
            return;
        }
        emitLibraryMutation();
        setReloadTick((tick) => tick + 1);
    }, []);
    useJobCompletion(manualJobId, onManualComplete);

    useEffect(() => {
        return subscribeSubscriptionAdded(() => {
            setReloadTick((tick) => tick + 1);
        });
    }, []);

    const isBusy = refreshing || manualJobId !== null;
    useRegisterChromeRefresh(onRefresh, isBusy || state.kind === 'loading');
    useRegisterChromeCalendar({
        selected: selectedDate,
        today,
        onSelect: handleSelectDate,
    });

    const entries = state.kind === 'ok' ? state.entries : EMPTY_ENTRIES;
    const entryFrom = useMemo(
        () =>
            isSameDay(selectedDate, today)
                ? undefined
                : { date: formatISODate(selectedDate) },
        [selectedDate, today],
    );
    const nav = useEntryNavigation(entries, entryFrom, {
        ArrowLeft: () => handleSelectDate(addDays(selectedDate, -1)),
        ArrowRight: () => {
            const next = addDays(selectedDate, 1);
            if (startOfLocalDay(next).getTime() > today.getTime()) return;
            handleSelectDate(next);
        },
        t: () => handleSelectDate(today),
        r: () => {
            if (!isBusy) onRefresh();
        },
    });

    return (
        <div className="mx-auto w-full max-w-2xl px-4 pt-10 pb-12 sm:px-6">
            {state.kind === 'loading' ? (
                <DigestSkeleton />
            ) : state.kind === 'error' ? (
                <EmptyMessage
                    lead="Couldn't load the digest."
                    detail="Try refreshing in a moment."
                />
            ) : (
                <Newspaper
                    entries={entries}
                    date={selectedDate}
                    today={today}
                    entryFrom={entryFrom}
                    nav={nav}
                    emptyState={{
                        lead: 'Nothing new this morning.',
                        detail: 'Enjoy your coffee.',
                    }}
                />
            )}
        </div>
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
            className="overflow-hidden rounded-xl bg-card shadow-card"
        >
            <ul className="flex flex-col">
                {Array.from({ length: 6 }).map((_, index) => (
                    <li key={index}>
                        {index > 0 ? (
                            <div
                                aria-hidden
                                className="mx-6 border-t border-border"
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
