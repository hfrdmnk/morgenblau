import {
    ChatIcon,
    CoffeeHotIcon,
    DocumentIcon,
    OpenIcon,
    VideoIcon,
} from '@proicons/react';
import DOMPurify from 'dompurify';
import { useMemo, useRef } from 'react';
import { Link } from 'wouter';

import { CardMasthead } from '@/components/card-masthead';
import { Favicon } from '@/components/favicon';
import { ListHighlight } from '@/components/list-highlight';
import { useAuthedMe } from '@/hooks/use-authed-me';
import type { ListNavigation } from '@/hooks/use-list-navigation';
import { formatEditionDate, isSameDay } from '@/lib/date';
import { readAuthor } from '@/lib/entry-meta';
import { entryActivation } from '@/lib/entry-nav';
import { pickGreeting, pickPastTitle } from '@/lib/greetings';
import { type EntryFrom } from '@/lib/paths';
import { safeHref } from '@/lib/utils';

export type ContentType = 'blogpost' | 'microblog' | 'video';

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

const TYPE_ICONS: Record<ContentType, typeof DocumentIcon> = {
    blogpost: DocumentIcon,
    microblog: ChatIcon,
    video: VideoIcon,
};

const ROW_CLICKABLE_BASE =
    'cursor-pointer transition-colors duration-200 ease-out focus-visible:outline-1 focus-visible:outline-offset-[-2px] focus-visible:outline-solid focus-visible:outline-ring';

const ROW_CLICKABLE_CLASS = `block px-6 py-5 outline-none ${ROW_CLICKABLE_BASE}`;

export function Newspaper({
    entries,
    date,
    today,
    entryFrom,
    nav,
    emptyState,
}: {
    entries: Entry[];
    date?: Date;
    today?: Date;
    entryFrom?: EntryFrom;
    nav: ListNavigation;
    emptyState?: { lead: string; detail: string };
}) {
    const listRef = useRef<HTMLDivElement>(null);
    const isEmpty = entries.length === 0;

    return (
        <article className="overflow-hidden rounded-xl bg-card shadow-card">
            <DigestMastheadSection
                date={date}
                today={today}
                count={entries.length}
            />
            {isEmpty && emptyState ? (
                <EmptyEntries lead={emptyState.lead} detail={emptyState.detail} />
            ) : (
                <div
                    ref={listRef}
                    className="relative"
                    onMouseLeave={nav.clearPointer}
                >
                    <ListHighlight
                        containerRef={listRef}
                        active={nav.active}
                        scrollKey={nav.scrollKey}
                    />
                    <ul className="relative z-10 flex flex-col">
                        {entries.map((entry, index) => (
                            <li key={entry.id}>
                                {index > 0 ? (
                                    <div
                                        aria-hidden
                                        className="mx-6 border-t border-border"
                                    />
                                ) : null}
                                {entry.contentType === 'microblog' ? (
                                    <InlineRow
                                        entry={entry}
                                        index={index}
                                        onActivate={nav.setActive}
                                    />
                                ) : (
                                    <StandardRow
                                        entry={entry}
                                        entryFrom={entryFrom}
                                        index={index}
                                        onActivate={nav.setActive}
                                    />
                                )}
                            </li>
                        ))}
                    </ul>
                </div>
            )}
        </article>
    );
}

// Owns the date-presence check so DigestMasthead's useAuthedMe read only fires when there's a date to show.
function DigestMastheadSection({
    date,
    today,
    count,
}: {
    date?: Date;
    today?: Date;
    count: number;
}) {
    if (!date) return null;
    return (
        <>
            <DigestMasthead
                date={date}
                count={count}
                isToday={today ? isSameDay(date, today) : true}
            />
            <div aria-hidden className="mx-6 border-t border-border" />
        </>
    );
}

function EmptyEntries({ lead, detail }: { lead: string; detail: string }) {
    return (
        <div className="flex flex-col items-center gap-3 px-6 py-16 text-center">
            <CoffeeHotIcon className="size-6 text-muted-foreground" />
            <p>{lead}</p>
            <p className="text-sm font-light text-muted-foreground">{detail}</p>
        </div>
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
        <CardMasthead
            eyebrow={formatEditionDate(date)}
            heading={heading}
            meta={count > 0 ? `${count} ${noun}` : undefined}
        />
    );
}

function StandardRow({
    entry,
    entryFrom,
    index,
    onActivate,
}: {
    entry: Entry;
    entryFrom?: EntryFrom;
    index: number;
    onActivate: (index: number) => void;
}) {
    const cleanedSummary = entry.body
        ? cleanSummary(entry.body, entry.title)
        : null;
    const byline = formatByline(entry);
    const target = entryActivation(entry, entryFrom);

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

    if (!target) {
        return (
            <div
                data-nav-row=""
                onMouseEnter={() => onActivate(index)}
                className="px-6 py-5"
            >
                {content}
            </div>
        );
    }

    if (target.external) {
        return (
            <a
                href={target.href}
                data-nav-row=""
                onMouseEnter={() => onActivate(index)}
                className={ROW_CLICKABLE_CLASS}
                target="_blank"
                rel="noopener noreferrer"
            >
                {content}
            </a>
        );
    }

    return (
        <Link
            href={target.href}
            data-nav-row=""
            onMouseEnter={() => onActivate(index)}
            className={ROW_CLICKABLE_CLASS}
        >
            {content}
        </Link>
    );
}

function InlineRow({
    entry,
    index,
    onActivate,
}: {
    entry: Entry;
    index: number;
    onActivate: (index: number) => void;
}) {
    const meta = formatMicroblogMeta(entry);

    return (
        <div
            data-nav-row=""
            onMouseEnter={() => onActivate(index)}
            className="px-6 py-5"
        >
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

// Body is server-sanitized at ingest (bluemonday UGC); DOMPurify is defense-in-depth in case a sanitizer regression slips through.
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
    const TypeIcon = TYPE_ICONS[entry.contentType] ?? DocumentIcon;
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
                    <OpenIcon className="size-[1.125rem] shrink-0" />
                </a>
            ) : null}
            <TypeIcon className="size-[1.125rem] shrink-0 text-muted-foreground" />
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
