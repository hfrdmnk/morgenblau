import {
    ArrowLeft01Icon,
    HourglassIcon,
    Pulse01Icon,
} from '@hugeicons/core-free-icons';
import { HugeiconsIcon } from '@hugeicons/react';
import { useEffect, useState } from 'react';

import { Newspaper } from '@/components/digest-rows';
import type { Entry } from '@/components/digest-rows';
import { Favicon } from '@/components/favicon';
import { Skeleton } from '@/components/ui/skeleton';
import { shortTimeAgo } from '@/lib/date';
import { useDocumentTitle } from '@/hooks/use-document-title';
import { PATHS } from '@/lib/paths';

type Frequency =
    | 'new'
    | 'daily'
    | 'weekly'
    | 'biweekly'
    | 'monthly'
    | 'irregular'
    | 'noPosts';

const FREQUENCY_LABEL: Record<Frequency, string> = {
    new: 'New',
    daily: 'Daily',
    weekly: 'Weekly',
    biweekly: 'Biweekly',
    monthly: 'Monthly',
    irregular: 'Irregular',
    noPosts: 'No posts',
};

type SourceDetail = {
    rkey: string;
    feedUrl: string;
    title?: string;
    siteUrl?: string;
    frequency?: Frequency;
    lastPublishedAt?: string;
    totalEntries: number;
    savedByYou: number;
};

type State =
    | { kind: 'loading' }
    | { kind: 'ok'; detail: SourceDetail; entries: Entry[] }
    | { kind: 'error' };

function rkeyFromLocation(): string | null {
    const prefix = `${PATHS.sources}/`;
    const path = window.location.pathname;
    if (!path.startsWith(prefix)) return null;
    const rkey = path.slice(prefix.length);
    return rkey.length > 0 ? rkey : null;
}

export function Source() {
    const rkey = rkeyFromLocation();
    const [state, setState] = useState<State>(
        rkey ? { kind: 'loading' } : { kind: 'error' },
    );

    useDocumentTitle(
        state.kind === 'ok' ? state.detail.title ?? 'Source' : 'Source',
    );

    useEffect(() => {
        if (!rkey) return;
        let cancelled = false;
        const load = async () => {
            try {
                const [detailRes, entriesRes] = await Promise.all([
                    fetch(`/api/subscriptions/${rkey}`, {
                        credentials: 'same-origin',
                    }),
                    fetch(`/api/subscriptions/${rkey}/entries`, {
                        credentials: 'same-origin',
                    }),
                ]);
                if (!detailRes.ok) throw new Error(String(detailRes.status));
                if (!entriesRes.ok) throw new Error(String(entriesRes.status));
                const detail = (await detailRes.json()) as SourceDetail;
                const entries = (await entriesRes.json()) as Entry[];
                if (!cancelled) setState({ kind: 'ok', detail, entries });
            } catch {
                if (!cancelled) setState({ kind: 'error' });
            }
        };
        load();
        return () => {
            cancelled = true;
        };
    }, [rkey]);

    if (state.kind === 'loading') {
        return <SourceSkeleton />;
    }
    if (state.kind === 'error') {
        return (
            <main className="mx-auto w-full max-w-2xl px-4 pt-16 pb-12 sm:px-6">
                <p className="text-sm font-light text-muted-foreground">
                    Couldn't load this source.
                </p>
            </main>
        );
    }

    return <SourceView detail={state.detail} entries={state.entries} />;
}

function SourceView({
    detail,
    entries,
}: {
    detail: SourceDetail;
    entries: Entry[];
}) {
    const domain = hostnameOf(detail.siteUrl ?? detail.feedUrl);
    const title = detail.title ?? detail.feedUrl;
    const proxyFavicon = `/api/favicon?feed=${encodeURIComponent(detail.feedUrl)}`;
    const frequency = detail.frequency ?? 'noPosts';

    return (
        <main className="mx-auto w-full max-w-2xl px-4 pt-16 pb-12 sm:px-6">
            <header className="mb-10 flex flex-col gap-4">
                <div className="relative flex items-center gap-3 font-sans">
                    <BackButton />
                    <Favicon src={proxyFavicon} className="size-10 rounded-lg" />
                    <div className="min-w-0 flex-1">
                        {domain ? (
                            <p className="truncate text-xs font-light text-muted-foreground">
                                {domain}
                            </p>
                        ) : null}
                        <h1 className="truncate text-2xl font-medium tracking-tight text-balance text-foreground">
                            {title}
                        </h1>
                    </div>
                </div>
                <StatRow detail={detail} frequency={frequency} />
            </header>

            {entries.length === 0 ? (
                <p className="text-sm font-light text-muted-foreground">
                    No posts yet.
                </p>
            ) : (
                <Newspaper
                    entries={entries}
                    entryFrom={{ sourceRkey: detail.rkey }}
                />
            )}
        </main>
    );
}

function StatRow({
    detail,
    frequency,
}: {
    detail: SourceDetail;
    frequency: Frequency;
}) {
    return (
        <dl className="grid grid-cols-4 gap-3 font-sans text-sm">
            <Stat label="Frequency">
                <span className="inline-flex items-center gap-1.5">
                    <HugeiconsIcon
                        icon={Pulse01Icon}
                        className="size-3.5 shrink-0 text-muted-foreground"
                    />
                    {FREQUENCY_LABEL[frequency]}
                </span>
            </Stat>
            <Stat label="Last post">
                {detail.lastPublishedAt ? (
                    <span className="inline-flex items-center gap-1.5">
                        <HugeiconsIcon
                            icon={HourglassIcon}
                            className="size-3.5 shrink-0 text-muted-foreground"
                        />
                        {shortTimeAgo(detail.lastPublishedAt)}
                    </span>
                ) : (
                    <span className="text-muted-foreground">—</span>
                )}
            </Stat>
            <Stat label="Total posts">{detail.totalEntries}</Stat>
            <Stat label="Saved by you">{detail.savedByYou}</Stat>
        </dl>
    );
}

function Stat({
    label,
    children,
}: {
    label: string;
    children: React.ReactNode;
}) {
    return (
        <div className="flex flex-col gap-0.5">
            <dt className="text-[0.6875rem] font-light tracking-wide text-muted-foreground uppercase">
                {label}
            </dt>
            <dd className="truncate font-medium tracking-tight">{children}</dd>
        </div>
    );
}

function SourceSkeleton() {
    return (
        <main
            aria-busy
            aria-label="Loading source"
            className="mx-auto w-full max-w-2xl px-4 pt-16 pb-12 sm:px-6"
        >
            <header className="mb-10 flex flex-col gap-4">
                <div className="relative flex items-center gap-3">
                    <BackButton />
                    <Skeleton className="size-10 rounded-lg" />
                    <div className="flex min-w-0 flex-1 flex-col gap-1.5">
                        <Skeleton className="h-3 w-32" />
                        <Skeleton className="h-6 w-3/4" />
                    </div>
                </div>
                <div className="grid grid-cols-4 gap-3">
                    {Array.from({ length: 4 }).map((_, i) => (
                        <div key={i} className="flex flex-col gap-1.5">
                            <Skeleton className="h-2 w-16" />
                            <Skeleton className="h-4 w-20" />
                        </div>
                    ))}
                </div>
            </header>

            <article className="overflow-hidden rounded-xl bg-card">
                <ul className="flex flex-col">
                    {Array.from({ length: 4 }).map((_, index) => (
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
        </main>
    );
}

function BackButton() {
    return (
        <a
            href={PATHS.sources}
            aria-label="Back to sources"
            className="absolute top-1/2 right-full mr-2 inline-flex size-9 -translate-y-1/2 items-center justify-center rounded-xl text-muted-foreground transition-colors duration-200 ease-out outline-none hover:text-foreground focus-visible:outline-1 focus-visible:outline-offset-2 focus-visible:outline-ring focus-visible:outline-solid"
        >
            <HugeiconsIcon icon={ArrowLeft01Icon} className="size-5" />
        </a>
    );
}

function hostnameOf(url: string | undefined): string {
    if (!url) return '';
    try {
        return new URL(url).hostname.replace(/^www\./, '');
    } catch {
        return '';
    }
}
