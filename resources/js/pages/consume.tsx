import {
    BubbleChatIcon,
    Globe02Icon,
    NewsIcon,
    PodcastIcon,
    RefreshIcon,
    Video01Icon,
} from '@hugeicons/core-free-icons';
import { HugeiconsIcon } from '@hugeicons/react';
import { Form, Head, router } from '@inertiajs/react';
import { useCallback, useState } from 'react';
import type { KeyboardEvent, MouseEvent } from 'react';

import { DigestPill } from '@/components/digest-pill';
import TextLink from '@/components/text-link';
import { Button } from '@/components/ui/button';
import { useDigestStatus } from '@/hooks/use-digest-status';
import { LevelContext } from '@/lib/level-context';
import { cn } from '@/lib/utils';
import { discover } from '@/routes';
import { refresh } from '@/routes/feeds';

type FeedEntry = App.Data.Feeds.FeedEntryViewData;
type ContentType = App.Enums.ContentType;

type ConsumeProps = {
    entries?: FeedEntry[];
    has_subscriptions: boolean;
    polling_since: string | null;
};

const TYPE_ICONS: Record<ContentType, typeof NewsIcon> = {
    blogpost: NewsIcon,
    microblog: BubbleChatIcon,
    video: Video01Icon,
    podcast: PodcastIcon,
};

export default function Consume({
    entries,
    has_subscriptions,
    polling_since,
}: ConsumeProps) {
    const isLoading = entries === undefined;
    const isEmpty = entries !== undefined && entries.length === 0;
    const [dismissedSince, setDismissedSince] = useState<string | null>(null);
    const [highlightSince, setHighlightSince] = useState<string | null>(null);

    const pollingSince =
        dismissedSince === polling_since ? null : polling_since;

    const status = useDigestStatus(pollingSince);

    const isPending = status.phase === 'fetching' || status.phase === 'ready';

    const handleReady = useCallback(() => {
        if (!pollingSince) {
            return;
        }

        const since = pollingSince;

        router.reload({
            only: ['entries'],
            onSuccess: () => {
                setHighlightSince(since);
                setDismissedSince(since);
            },
        });
    }, [pollingSince]);

    const handleCaughtUpFade = useCallback(() => {
        setDismissedSince(pollingSince);
    }, [pollingSince]);

    return (
        <>
            <Head title="Consume" />
            <DigestPill
                state={status}
                onReady={handleReady}
                onCaughtUpFade={handleCaughtUpFade}
            />
            <div className="mx-auto w-full max-w-2xl px-4 py-10 sm:px-6">
                {has_subscriptions ? (
                    <div className="mb-6 flex items-center justify-end">
                        <RefreshButton externallyPending={isPending} />
                    </div>
                ) : null}
                {isLoading ? (
                    <DigestSkeleton />
                ) : isEmpty ? (
                    <EmptyState hasSubscriptions={has_subscriptions} />
                ) : (
                    <Newspaper
                        entries={entries}
                        highlightSince={highlightSince}
                    />
                )}
            </div>
        </>
    );
}

function Newspaper({
    entries,
    highlightSince,
}: {
    entries: FeedEntry[];
    highlightSince: string | null;
}) {
    return (
        <LevelContext.Provider value={2}>
            <article className="overflow-hidden rounded-3xl border border-gray-100 bg-card dark:border-gray-700">
                <ul className="flex flex-col">
                    {entries.map((entry, index) => {
                        const isFresh =
                            highlightSince !== null &&
                            entry.first_seen_at > highlightSince;

                        return (
                            <li key={entry.id}>
                                {index > 0 ? (
                                    <div
                                        aria-hidden
                                        className="mx-6 border-t border-gray-100 dark:border-gray-700"
                                    />
                                ) : null}
                                <FreshHighlight active={isFresh}>
                                    {entry.content_type === 'microblog' ? (
                                        <InlineRow entry={entry} />
                                    ) : (
                                        <StandardRow entry={entry} />
                                    )}
                                </FreshHighlight>
                            </li>
                        );
                    })}
                </ul>
            </article>
        </LevelContext.Provider>
    );
}

function FreshHighlight({
    active,
    children,
}: {
    active: boolean;
    children: React.ReactNode;
}) {
    return (
        <div className={cn(active && 'animate-fresh-highlight')}>
            {children}
        </div>
    );
}

function DigestSkeleton() {
    return (
        <article
            aria-busy
            aria-label="Loading digest"
            className="animate-digest-skeleton overflow-hidden rounded-3xl border border-gray-100 bg-card dark:border-gray-700"
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
                                <div className="size-4 rounded-sm bg-gray-100 dark:bg-gray-700" />
                                <div className="h-3 w-32 rounded-sm bg-gray-100 dark:bg-gray-700" />
                            </div>
                            <div className="h-5 w-3/4 rounded-sm bg-gray-100 dark:bg-gray-700" />
                            <div className="h-3 w-11/12 rounded-sm bg-gray-100 dark:bg-gray-700" />
                            <div className="h-3 w-2/3 rounded-sm bg-gray-100 dark:bg-gray-700" />
                        </div>
                    </li>
                ))}
            </ul>
        </article>
    );
}

function StandardRow({ entry }: { entry: FeedEntry }) {
    const cleanedSummary = entry.summary
        ? cleanSummary(entry.summary, entry.entry_title)
        : null;
    const byline = formatByline(entry);

    const content = (
        <div className="flex flex-col gap-1.5">
            <RowHeader
                entry={entry}
                lead={entry.display_title ?? 'Unknown source'}
            />
            <h3 className="line-clamp-3 text-lg font-medium tracking-tight text-foreground">
                {entry.entry_title ?? (
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

    if (!entry.link) {
        return <div className="px-6 py-5">{content}</div>;
    }

    return (
        <a
            href={entry.link}
            target="_blank"
            rel="noopener noreferrer"
            className={ROW_CLICKABLE_CLASS}
        >
            {content}
        </a>
    );
}

function InlineRow({ entry }: { entry: FeedEntry }) {
    const meta = formatMicroblogMeta(entry);

    const handleClick = (e: MouseEvent<HTMLDivElement>) => {
        if (!entry.link) {
            return;
        }

        if ((e.target as HTMLElement).closest('a')) {
            return;
        }

        window.open(entry.link, '_blank', 'noopener,noreferrer');
    };

    const handleKey = (e: KeyboardEvent<HTMLDivElement>) => {
        if (!entry.link) {
            return;
        }

        if (e.key !== 'Enter' && e.key !== ' ') {
            return;
        }

        if ((e.target as HTMLElement).closest('a')) {
            return;
        }

        e.preventDefault();
        window.open(entry.link, '_blank', 'noopener,noreferrer');
    };

    return (
        <div
            role={entry.link ? 'link' : undefined}
            tabIndex={entry.link ? 0 : undefined}
            onClick={entry.link ? handleClick : undefined}
            onKeyDown={entry.link ? handleKey : undefined}
            className={cn(
                'px-6 py-5 outline-none',
                entry.link && ROW_CLICKABLE_BASE,
            )}
        >
            <div className="flex flex-col gap-3">
                <RowHeader entry={entry} lead={meta ?? 'Unknown source'} />
                {entry.summary ? (
                    // Summary is sanitized at ingest by HtmlSanitizer (Purify).
                    <div
                        className="text-base text-foreground [&_a]:text-primary [&_a]:underline-offset-4 [&_a:hover]:underline"
                        dangerouslySetInnerHTML={{ __html: entry.summary }}
                    />
                ) : null}
            </div>
        </div>
    );
}

function RowHeader({ entry, lead }: { entry: FeedEntry; lead: string }) {
    const TypeIcon = TYPE_ICONS[entry.content_type];

    return (
        <div className="flex items-center gap-2">
            <Favicon src={entry.favicon_url} />
            <p className="line-clamp-1 min-w-0 flex-1 text-sm font-light text-muted-foreground">
                {lead}
            </p>
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

const ROW_CLICKABLE_BASE =
    'cursor-pointer transition-colors duration-200 ease-out hover:bg-gray-50 focus-visible:outline-1 focus-visible:outline-offset-[-2px] focus-visible:outline-solid focus-visible:outline-ring dark:hover:bg-gray-900';

const ROW_CLICKABLE_CLASS = `block px-6 py-5 outline-none ${ROW_CLICKABLE_BASE}`;

function cleanSummary(summary: string, title: string | null): string | null {
    const stripped = summary
        .replace(/<[^>]*>/g, '')
        .replace(/\s+/g, ' ')
        .trim();

    if (stripped.length < 40) {
        return null;
    }

    if (title) {
        const t = title.toLowerCase();
        const s = stripped.toLowerCase();

        if (s === t) {
            return null;
        }

        if (s.includes(t) || t.includes(s)) {
            return null;
        }
    }

    if (/^(read|continue|view|visit|see)\s+(more|the|on)\b/i.test(stripped)) {
        return null;
    }

    if (/^read\s+full\b/i.test(stripped)) {
        return null;
    }

    if (/^by\s+\S+/i.test(stripped) && stripped.length < 60) {
        return null;
    }

    if (/^https?:\/\/\S+$/.test(stripped)) {
        return null;
    }

    if (stripped.endsWith('…') && stripped.length < 30) {
        return null;
    }

    return stripped;
}

function formatByline(entry: FeedEntry): string | null {
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

function formatMicroblogMeta(entry: FeedEntry): string | null {
    const bits: string[] = [];
    const source = entry.display_title ?? null;
    const author = entry.author ?? null;

    if (source) {
        bits.push(source);
    }

    if (author && author !== source) {
        bits.push(author);
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

function RefreshButton({ externallyPending }: { externallyPending: boolean }) {
    return (
        <Form {...refresh.form()} options={{ preserveScroll: true }}>
            {({ processing }) => (
                <Button
                    type="submit"
                    variant="secondary"
                    size="sm"
                    disabled={processing || externallyPending}
                >
                    <HugeiconsIcon icon={RefreshIcon} />
                    Refresh now
                </Button>
            )}
        </Form>
    );
}

function EmptyState({ hasSubscriptions }: { hasSubscriptions: boolean }) {
    if (!hasSubscriptions) {
        return (
            <div className="flex flex-col gap-2">
                <p>No sources yet.</p>
                <p className="text-sm font-light text-muted-foreground">
                    Pick one over at{' '}
                    <TextLink href={discover().url}>/discover</TextLink>.
                </p>
            </div>
        );
    }

    return (
        <div className="flex flex-col gap-2">
            <p>Nothing new this morning.</p>
            <p className="text-sm font-light text-muted-foreground">
                Enjoy your coffee.
            </p>
        </div>
    );
}
