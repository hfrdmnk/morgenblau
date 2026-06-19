import {
    BubbleChatIcon,
    LinkSquare01Icon,
    NewsIcon,
    PodcastIcon,
    Video01Icon,
} from '@hugeicons/core-free-icons';
import { HugeiconsIcon } from '@hugeicons/react';
import type { IconSvgElement } from '@hugeicons/react';
import DOMPurify from 'dompurify';
import { useMemo } from 'react';

import { Favicon } from '@/components/favicon';
import { useAuthedMe } from '@/hooks/use-authed-me';
import { formatEditionDate, isSameDay } from '@/lib/date';
import { pickGreeting, pickPastTitle } from '@/lib/greetings';
import { entryHref, type EntryFrom } from '@/lib/paths';
import { safeHref } from '@/lib/utils';

export type ContentType = 'blogpost' | 'microblog' | 'video' | 'podcast';

export type Source = {
    feedUrl: string;
    title: string | null;
    siteUrl: string | null;
    faviconUrl: string | null;
};

export type Entry = {
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

const TYPE_ICONS: Record<ContentType, IconSvgElement> = {
    blogpost: NewsIcon,
    microblog: BubbleChatIcon,
    video: Video01Icon,
    podcast: PodcastIcon,
};

const ROW_CLICKABLE_BASE =
    'cursor-pointer transition-colors duration-200 ease-out hover:bg-overlay-1 focus-visible:outline-1 focus-visible:outline-offset-[-2px] focus-visible:outline-solid focus-visible:outline-ring';

const ROW_CLICKABLE_CLASS = `block px-6 py-5 outline-none ${ROW_CLICKABLE_BASE}`;

export function Newspaper({
    entries,
    date,
    today,
    entryFrom,
}: {
    entries: Entry[];
    date?: Date;
    today?: Date;
    entryFrom?: EntryFrom;
}) {
    return (
        <article className="overflow-hidden rounded-xl border border-border bg-card">
            {date ? (
                <>
                    <DigestMasthead
                        date={date}
                        count={entries.length}
                        isToday={today ? isSameDay(date, today) : true}
                    />
                    <div aria-hidden className="mx-6 border-t border-border" />
                </>
            ) : null}
            <ul className="flex flex-col">
                {entries.map((entry, index) => (
                    <li key={entry.id}>
                        {index > 0 ? (
                            <div
                                aria-hidden
                                className="mx-6 border-t border-border"
                            />
                        ) : null}
                        {entry.contentType === 'microblog' ? (
                            <InlineRow entry={entry} />
                        ) : (
                            <StandardRow entry={entry} entryFrom={entryFrom} />
                        )}
                    </li>
                ))}
            </ul>
        </article>
    );
}

function DigestMasthead({
    date,
    count,
    isToday,
}: {
    date: Date;
    count: number;
    isToday: boolean;
}) {
    const me = useAuthedMe();
    const name = me.displayName?.trim().split(/\s+/)[0] ?? null;
    // date is stable per selection, so the phrase re-picks only on navigation.
    const heading = useMemo(
        () => (isToday ? pickGreeting(name, date) : pickPastTitle(date)),
        [name, date, isToday],
    );
    const noun = count === 1 ? 'piece' : 'pieces';

    return (
        <div className="flex flex-col gap-1 px-6 pt-6 pb-5">
            <p className="text-sm font-light text-muted-foreground">
                {formatEditionDate(date)}
            </p>
            <div className="flex items-baseline justify-between gap-4">
                <h2 className="text-xl font-medium">{heading}</h2>
                <p className="shrink-0 text-sm text-muted-foreground">
                    {count} {noun}
                </p>
            </div>
        </div>
    );
}

function StandardRow({
    entry,
    entryFrom,
}: {
    entry: Entry;
    entryFrom?: EntryFrom;
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
                href={entryHref(entry.entrySlug, entryFrom)}
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
            className="text-sm text-muted-foreground [&_a]:text-primary [&_a]:underline-offset-4 [&_a:hover]:underline"
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

function cleanSummary(summary: string, title: string | null): string | null {
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
