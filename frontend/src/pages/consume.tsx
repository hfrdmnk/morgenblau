import {
    BubbleChatIcon,
    Globe02Icon,
    LinkSquare01Icon,
    NewsIcon,
    PodcastIcon,
    Video01Icon,
} from '@hugeicons/core-free-icons';
import { HugeiconsIcon } from '@hugeicons/react';
import type { IconSvgElement } from '@hugeicons/react';
import DOMPurify from 'dompurify';
import { useCallback, useEffect, useMemo, useState } from 'react';

import { CalendarStrip } from '@/components/calendar-strip';
import { Skeleton } from '@/components/ui/skeleton';
import { useRegisterChromeRefresh } from '@/hooks/use-chrome-refresh';
import { useDocumentTitle } from '@/hooks/use-document-title';
import { useJobsPoll } from '@/hooks/use-jobs-poll';
import { LevelContext } from '@/hooks/use-surface-level';
import {
    formatISODate,
    isSameDay,
    parseISODate,
    startOfLocalDay,
} from '@/lib/date';
import { entryHref } from '@/lib/paths';
import { subscribeSubscriptionAdded } from '@/lib/subscription-events';
import { safeHref } from '@/lib/utils';

type Source = {
    feedUrl: string;
    title: string | null;
    siteUrl: string | null;
    faviconUrl: string | null;
};

type Entry = {
    id: number;
    entrySlug: string;
    title: string | null;
    url: string;
    contentType: ContentType;
    publishedAt: string;
    source: Source;
    body: string | null;
    metadata?: string | null;
};

type ContentType = 'blogpost' | 'microblog' | 'video' | 'podcast';

type DigestResponse = {
    date: string;
    entries: Entry[];
    hasActiveJob: boolean;
};

type State =
    | { kind: 'loading' }
    | { kind: 'ok'; entries: Entry[]; hasActiveJob: boolean }
    | { kind: 'error' };

const TYPE_ICONS: Record<ContentType, IconSvgElement> = {
    blogpost: NewsIcon,
    microblog: BubbleChatIcon,
    video: Video01Icon,
    podcast: PodcastIcon,
};

export function Consume() {
    useDocumentTitle('Consume');
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

function Newspaper({
    entries,
    fromDate,
}: {
    entries: Entry[];
    fromDate?: string;
}) {
    return (
        <LevelContext.Provider value={2}>
            <article className="overflow-hidden rounded-3xl border border-gray-100 bg-card dark:border-gray-700">
                <ul className="flex flex-col">
                    {entries.map((entry, index) => (
                        <li key={entry.id}>
                            {index > 0 ? (
                                <div
                                    aria-hidden
                                    className="mx-6 border-t border-gray-100 dark:border-gray-700"
                                />
                            ) : null}
                            {entry.contentType === 'microblog' ? (
                                <InlineRow entry={entry} />
                            ) : (
                                <StandardRow
                                    entry={entry}
                                    fromDate={fromDate}
                                />
                            )}
                        </li>
                    ))}
                </ul>
            </article>
        </LevelContext.Provider>
    );
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

function StandardRow({
    entry,
    fromDate,
}: {
    entry: Entry;
    fromDate?: string;
}) {
    const cleanedSummary = entry.body
        ? cleanSummary(entry.body, entry.title)
        : null;
    const byline = formatByline(entry);
    const opensInReader =
        entry.contentType === 'blogpost' || entry.contentType === 'video';

    const content = (
        <div className="flex flex-col gap-1.5">
            <RowHeader
                entry={entry}
                lead={entry.source.title ?? entry.source.feedUrl}
            />
            <h3 className="line-clamp-3 text-lg font-medium tracking-tight text-foreground">
                {entry.title ?? (
                    <em className="text-muted-foreground">Untitled</em>
                )}
            </h3>
            {cleanedSummary ? (
                <p className="line-clamp-2 text-sm text-muted-foreground">
                    {cleanedSummary}
                </p>
            ) : null}
            {byline ? (
                <p className="text-sm font-light text-muted-foreground">
                    {byline}
                </p>
            ) : null}
        </div>
    );

    if (opensInReader) {
        return (
            <a
                href={entryHref(entry.entrySlug, fromDate)}
                className={ROW_CLICKABLE_CLASS}
            >
                {content}
            </a>
        );
    }

    const link = safeHref(entry.url);
    if (!link) {
        return <div className="px-6 py-5">{content}</div>;
    }

    return (
        <a
            href={link}
            target="_blank"
            rel="noopener noreferrer"
            className={ROW_CLICKABLE_CLASS}
        >
            {content}
        </a>
    );
}

function InlineRow({ entry }: { entry: Entry }) {
    const meta = formatMicroblogMeta(entry);

    return (
        <div className="px-6 py-5">
            <div className="flex flex-col gap-3">
                <RowHeader
                    entry={entry}
                    lead={meta ?? entry.source.title ?? entry.source.feedUrl}
                    linkHref={entry.url}
                />
                {entry.body ? <MicroblogBody html={entry.body} /> : null}
            </div>
        </div>
    );
}

// Body is server-sanitized at ingest (bluemonday UGC). DOMPurify runs as
// client-side defense-in-depth in case a sanitizer regression slips through.
function MicroblogBody({ html }: { html: string }) {
    const clean = useMemo(() => DOMPurify.sanitize(html), [html]);
    return (
        <div
            className="text-base text-foreground [&_a]:text-primary [&_a]:underline-offset-4 [&_a:hover]:underline"
            dangerouslySetInnerHTML={{ __html: clean }}
        />
    );
}

function RowHeader({
    entry,
    lead,
    linkHref,
}: {
    entry: Entry;
    lead: string;
    linkHref?: string | null;
}) {
    const TypeIcon = TYPE_ICONS[entry.contentType] ?? NewsIcon;
    const link = safeHref(linkHref);

    return (
        <div className="flex items-center gap-2">
            <Favicon src={entry.source.faviconUrl} />
            <p className="line-clamp-1 min-w-0 flex-1 text-sm font-light text-muted-foreground">
                {lead}
            </p>
            {link ? (
                <a
                    href={link}
                    target="_blank"
                    rel="noopener noreferrer"
                    aria-label="Open source post"
                    className="rounded-sm text-muted-foreground transition-colors duration-200 ease-out hover:text-foreground focus-visible:outline-1 focus-visible:outline-ring"
                >
                    <HugeiconsIcon
                        icon={LinkSquare01Icon}
                        className="size-[1.125rem] shrink-0"
                    />
                </a>
            ) : null}
            <HugeiconsIcon
                icon={TypeIcon}
                className="size-[1.125rem] shrink-0 text-muted-foreground"
            />
        </div>
    );
}

function Favicon({ src }: { src: string | null }) {
    const [errored, setErrored] = useState(false);

    if (!src || errored) {
        return (
            <HugeiconsIcon
                icon={Globe02Icon}
                className="size-4 text-muted-foreground"
            />
        );
    }

    return (
        <img
            src={src}
            alt=""
            className="size-4 rounded-sm"
            onError={() => setErrored(true)}
            loading="lazy"
        />
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

const ROW_CLICKABLE_BASE =
    'cursor-pointer transition-colors duration-200 ease-out hover:bg-gray-50 focus-visible:outline-1 focus-visible:outline-offset-[-2px] focus-visible:outline-solid focus-visible:outline-ring dark:hover:bg-gray-900';

const ROW_CLICKABLE_CLASS = `block px-6 py-5 outline-none ${ROW_CLICKABLE_BASE}`;

function cleanSummary(summary: string, title: string | null): string | null {
    // Parse as HTML so the browser decodes entities (&#39; → ') and strips
    // tags in one pass. textContent yields safe plain text.
    const doc = new DOMParser().parseFromString(summary, 'text/html');
    const stripped = (doc.body.textContent ?? '').replace(/\s+/g, ' ').trim();

    if (stripped === '') return null;

    if (title) {
        const t = title.toLowerCase();
        const s = stripped.toLowerCase();
        if (s === t) return null;
        if (s.includes(t) || t.includes(s)) return null;
    }

    if (/^(read|continue|view|visit|see)\s+(more|the|on)\b/i.test(stripped)) {
        return null;
    }
    if (/^read\s+full\b/i.test(stripped)) return null;
    if (/^by\s+\S+/i.test(stripped) && stripped.length < 60) return null;
    if (/^https?:\/\/\S+$/.test(stripped)) return null;
    if (stripped.endsWith('…') && stripped.length < 30) return null;

    return stripped;
}

function formatByline(entry: Entry): string | null {
    const bits: string[] = [];
    const author = readAuthor(entry.metadata);
    if (author) bits.push(author);
    if (entry.publishedAt) bits.push(formatRelative(entry.publishedAt));
    return bits.length > 0 ? bits.join(' · ') : null;
}

function formatMicroblogMeta(entry: Entry): string | null {
    const bits: string[] = [];
    const source = entry.source.title;
    const author = readAuthor(entry.metadata);
    if (source) bits.push(source);
    if (author && author !== source) bits.push(author);
    if (entry.publishedAt) bits.push(formatRelative(entry.publishedAt));
    return bits.length > 0 ? bits.join(' · ') : null;
}

function readAuthor(metadata: string | null | undefined): string | null {
    if (!metadata) return null;
    try {
        const parsed = JSON.parse(metadata) as { author?: unknown };
        return typeof parsed.author === 'string' ? parsed.author : null;
    } catch {
        return null;
    }
}

function formatRelative(iso: string): string {
    const then = new Date(iso);
    const now = new Date();
    const diffSeconds = Math.round((now.getTime() - then.getTime()) / 1000);

    if (diffSeconds < 60) return 'just now';
    if (diffSeconds < 3600) return `${Math.round(diffSeconds / 60)} min ago`;
    if (diffSeconds < 86400) return `${Math.round(diffSeconds / 3600)} hr ago`;

    const d = Math.round(diffSeconds / 86400);
    return `${d} day${d === 1 ? '' : 's'} ago`;
}
