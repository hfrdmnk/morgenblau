import { RefreshIcon } from '@hugeicons/core-free-icons';
import { HugeiconsIcon } from '@hugeicons/react';
import { Form, Head, router } from '@inertiajs/react';
import { useEffect, useRef } from 'react';
import TextLink from '@/components/text-link';
import { Button } from '@/components/ui/button';
import { Spinner } from '@/components/ui/spinner';
import { discover } from '@/routes';
import { refresh } from '@/routes/feeds';

type FeedEntry = App.Data.Feeds.FeedEntryViewData;

type ConsumeProps = {
    entries: FeedEntry[];
    refreshing_feed_ids: number[];
    has_subscriptions: boolean;
};

const POLL_INTERVAL_MS = 2000;
const POLL_MAX_TICKS = 15;

export default function Consume({
    entries,
    refreshing_feed_ids,
    has_subscriptions,
}: ConsumeProps) {
    const isRefreshing = refreshing_feed_ids.length > 0;
    const tickRef = useRef(0);

    useEffect(() => {
        if (!isRefreshing) {
            tickRef.current = 0;

            return;
        }

        tickRef.current = 0;

        const interval = window.setInterval(() => {
            tickRef.current += 1;

            if (tickRef.current > POLL_MAX_TICKS) {
                window.clearInterval(interval);

                return;
            }

            router.reload({
                only: ['entries', 'refreshing_feed_ids'],
                async: true,
            });
        }, POLL_INTERVAL_MS);

        return () => {
            window.clearInterval(interval);
        };
    }, [isRefreshing]);

    const isEmpty = entries.length === 0;

    return (
        <>
            <Head title="Consume" />
            <div className="mx-auto w-full max-w-2xl px-6 py-10">
                {has_subscriptions ? (
                    <div className="mb-8 flex items-center justify-between gap-4">
                        {isRefreshing ? (
                            <RefreshIndicator
                                count={refreshing_feed_ids.length}
                            />
                        ) : (
                            <span aria-hidden />
                        )}
                        <RefreshButton disabled={isRefreshing} />
                    </div>
                ) : null}
                {isEmpty ? (
                    <EmptyState
                        hasSubscriptions={has_subscriptions}
                        isRefreshing={isRefreshing}
                    />
                ) : (
                    <ul className="flex flex-col gap-8">
                        {entries.map((entry) => (
                            <EntryRow key={entry.id} entry={entry} />
                        ))}
                    </ul>
                )}
            </div>
        </>
    );
}

function RefreshButton({ disabled }: { disabled: boolean }) {
    return (
        <Form {...refresh.form()} options={{ preserveScroll: true }}>
            {({ processing }) => (
                <Button
                    type="submit"
                    variant="secondary"
                    size="sm"
                    disabled={disabled || processing}
                >
                    <HugeiconsIcon icon={RefreshIcon} />
                    Refresh now
                </Button>
            )}
        </Form>
    );
}

function EmptyState({
    hasSubscriptions,
    isRefreshing,
}: {
    hasSubscriptions: boolean;
    isRefreshing: boolean;
}) {
    if (!hasSubscriptions) {
        return (
            <div className="flex flex-col gap-2">
                <p>No sources yet.</p>
                <p className="font-handwritten text-sm text-muted-foreground">
                    Pick one over at{' '}
                    <TextLink href={discover().url}>/discover</TextLink>.
                </p>
            </div>
        );
    }

    if (isRefreshing) {
        return null;
    }

    return (
        <div className="flex flex-col gap-2">
            <p>Nothing new this morning.</p>
            <p className="font-handwritten text-sm text-muted-foreground">
                Enjoy your coffee.
            </p>
        </div>
    );
}

function RefreshIndicator({ count }: { count: number }) {
    return (
        <p
            role="status"
            aria-live="polite"
            className="inline-flex items-center gap-[0.5em] font-handwritten text-sm text-muted-foreground"
        >
            <Spinner className="size-3.5" />
            {count === 1 ? 'Refreshing 1 feed…' : `Refreshing ${count} feeds…`}
        </p>
    );
}

function EntryRow({ entry }: { entry: FeedEntry }) {
    const meta = formatMeta(entry);

    return (
        <li className="flex flex-col gap-2">
            <p className="font-handwritten text-sm text-muted-foreground">
                {entry.display_title ?? 'Untitled source'}
            </p>
            {entry.entry_title ? (
                entry.link ? (
                    <a
                        href={entry.link}
                        target="_blank"
                        rel="noreferrer noopener"
                        className="text-lg font-medium tracking-tight text-foreground hover:underline"
                    >
                        {entry.entry_title}
                    </a>
                ) : (
                    <h3 className="text-foreground">{entry.entry_title}</h3>
                )
            ) : null}
            {entry.summary ? (
                <p className="text-sm text-muted-foreground">{entry.summary}</p>
            ) : null}
            {meta ? (
                <p className="font-handwritten text-sm text-muted-foreground">
                    {meta}
                </p>
            ) : null}
        </li>
    );
}

function formatMeta(entry: FeedEntry): string | null {
    const bits: string[] = [];

    if (entry.author) {
        bits.push(entry.author);
    }

    const when = entry.published_at ?? entry.first_seen_at;

    if (when) {
        bits.push(formatRelative(when));
    }

    return bits.length > 0 ? bits.join(' · ') : null;
}

function formatRelative(iso: string): string {
    const then = new Date(iso);
    const now = new Date();
    const diffSeconds = Math.round((now.getTime() - then.getTime()) / 1000);

    if (diffSeconds < 60) {
        return 'just now';
    }

    if (diffSeconds < 3600) {
        const m = Math.round(diffSeconds / 60);

        return `${m} min ago`;
    }

    if (diffSeconds < 86400) {
        const h = Math.round(diffSeconds / 3600);

        return `${h} hr ago`;
    }

    const d = Math.round(diffSeconds / 86400);

    return `${d} day${d === 1 ? '' : 's'} ago`;
}
