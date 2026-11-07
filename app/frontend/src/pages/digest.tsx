import { AnimatePresence, motion, useReducedMotion } from 'motion/react';
import { useEffect, useId, useMemo, useState } from 'react';
import { Skeleton } from 'skullet/react';
import { Link, useLocation } from 'wouter';
import { useSearch } from 'wouter/use-browser-location';

import { AddSourceDialog } from '@/components/add-source-dialog';
import { ChevronDownIcon } from '@/components/icons';
import { ItemList, ListError, ListItem, SourceIcon } from '@/components/item-list';
import { EmptyState } from '@/components/status-state';
import { Button } from '@/components/ui/button';
import { useCommands } from '@/hooks/use-commands';
import { useDigest, type DigestEntry } from '@/hooks/use-digest';
import { useDocumentTitle } from '@/hooks/use-document-title';
import { useShortcuts } from '@/hooks/use-shortcuts';
import { useSources, type FeedSource } from '@/hooks/use-sources';
import { subscriptionChanges } from '@/lib/add-source';
import { addCalendarDays, selectedDigestDate, startOfToday, toDateKey } from '@/lib/digest-dates';
import { digestHref, entryHref, readerSourceHref } from '@/lib/paths';
import { entryExcerpt } from '@/lib/reader';
import { showShortcutNotice } from '@/lib/shortcuts';
import { cn } from '@/lib/utils';

const dayName = new Intl.DateTimeFormat(undefined, { weekday: 'long' });
export function Digest() {
    const search = useSearch();
    const today = startOfToday();
    const selectedDate = selectedDigestDate(search, today);
    const selectedKey = toDateKey(selectedDate);
    const digest = useDigest(selectedKey);
    useDayShortcuts(selectedDate, today, digest.status === 'loaded' ? digest.data.entries : []);
    useDocumentTitle(dayName.format(selectedDate));

    return (
        <main className="mx-auto w-full max-w-3xl px-5 pb-20 pt-20 sm:px-8 md:pt-24" id="main">
            <h1 className="mb-8 text-base font-book text-muted-foreground">
                {dayName.format(selectedDate)}
            </h1>
            <DigestContent date={selectedKey} state={digest} />
        </main>
    );
}

function useDayShortcuts(selected: Date, today: Date, entries: DigestEntry[]) {
    const [, navigate] = useLocation();
    const group = dayName.format(selected);
    const todayKey = toDateKey(today);
    const isToday = toDateKey(selected) === todayKey;
    const show = (date: Date) => {
        const key = toDateKey(date);
        navigate(digestHref(key === todayKey ? undefined : key));
    };
    const earlier = () => show(addCalendarDays(selected, -1));
    const later = () => isToday ? showShortcutNotice('Today is the latest digest') : show(addCalendarDays(selected, 1));
    useShortcuts({
        'Shift+ArrowLeft': earlier,
        h: earlier,
        'Shift+ArrowRight': later,
        l: later,
        'Shift+T': () => isToday ? showShortcutNotice('Already showing today') : show(today),
    });
    useCommands([
        { id: 'day:previous', group, label: 'Previous day', shortcut: 'h', keywords: ['earlier', 'back'], run: earlier },
        ...(isToday ? [] : [
            { id: 'day:next', group, label: 'Next day', shortcut: 'l', keywords: ['later', 'forward'], run: later },
            { id: 'day:today', group, label: 'Today', shortcut: 'Shift+T', run: () => show(today) },
        ]),
        ...entries.map((entry) => ({
            id: `headline:${entry.entrySlug}`,
            group: 'Headlines',
            label: entry.title || 'Untitled',
            detail: entry.source.title ?? undefined,
            icon: entry.source.faviconUrl,
            searchOnly: true,
            run: () => navigate(entryHref(entry.entrySlug, toDateKey(selected))),
        })),
    ]);
}

type DigestData = Extract<ReturnType<typeof useDigest>, { status: 'loaded' }>['data'];

const SAMPLE_SOURCE = { title: 'Example Publication', siteUrl: 'https://example.com', faviconUrl: null };
const SAMPLE_DIGEST: DigestData = {
    date: '2026-01-01',
    hasActiveJob: false,
    entries: [
        { id: 1, entrySlug: 'a', title: 'A sample headline that runs about as long as most do', body: 'An opening sentence or two of the post, long enough to fill the two lines an excerpt shows here.', source: { ...SAMPLE_SOURCE, primary: true } },
        ...['A shorter sample headline', 'Another sample headline of a typical length', 'A sample headline'].map((title, index) => (
            { id: index + 2, entrySlug: `${index}`, title, body: null, source: SAMPLE_SOURCE }
        )),
    ],
};

function DigestContent({ date, state }: { date: string; state: ReturnType<typeof useDigest> }) {
    // An empty day while a sync runs is still loading: entries are on their way, so it keeps the skeleton until they land or the job ends.
    const collecting = state.status === 'loaded' && state.data.hasActiveJob && state.data.entries.length === 0;
    return (
        <Skeleton
            data={state.status === 'loaded' && !collecting ? state.data : null}
            error={state.status === 'error' ? <ListError label="this digest" retry={state.retry} /> : undefined}
            id="digest"
            label="Loading digest"
            sample={SAMPLE_DIGEST}
        >
            {(digest) => <LoadedDigest date={date} digest={digest} key={date} />}
        </Skeleton>
    );
}

// Kept outside the page so going back from a folded post finds its group still open.
const openSources = new Map<string, Set<string>>();

function LoadedDigest({ date, digest }: { date: string; digest: DigestData }) {
    const [open, setOpen] = useState(() => new Set(openSources.get(date)));
    if (digest.entries.length === 0) return <EmptyDigest digest={digest} />;
    const toggle = (key: string) => {
        const next = new Set(open);
        if (!next.delete(key)) next.add(key);
        openSources.set(date, next);
        setOpen(next);
    };
    const primary = groupBySource(digest.entries.filter((entry) => entry.source.primary));
    const rest = groupBySource(digest.entries.filter((entry) => !entry.source.primary));
    return (
        <div className="flex flex-col gap-20">
            {primary.length > 0 ? (
                <ol aria-label="Primary sources" className="flex flex-col gap-12">
                    {primary.map((group) => (
                        <PrimaryItem date={date} group={group} key={group.key} onToggle={() => toggle(group.key)} open={open.has(group.key)} />
                    ))}
                </ol>
            ) : null}
            {rest.length > 0 ? (
                <ItemList ordered>
                    {rest.map((group) => (
                        <DigestItem date={date} group={group} key={group.key} onToggle={() => toggle(group.key)} open={open.has(group.key)} />
                    ))}
                </ItemList>
            ) : null}
        </div>
    );
}

type SourceGroup = { key: string; lead: DigestEntry; more: DigestEntry[] };

// Entries arrive newest first, so a busy source keeps its latest post in place and folds the rest beneath it.
function groupBySource(entries: DigestEntry[]): SourceGroup[] {
    const groups = new Map<string, SourceGroup>();
    for (const entry of entries) {
        const { source } = entry;
        const key = source.kind === 'newsletter' ? `newsletter:${source.id}` : source.feedUrl ? `feed:${source.feedUrl}` : `entry:${entry.id}`;
        const group = groups.get(key);
        if (group) group.more.push(entry);
        else groups.set(key, { key, lead: entry, more: [] });
    }
    return [...groups.values()];
}

// An empty day reads differently for a reader who has nothing to follow yet, so it checks before choosing the copy.
function EmptyDigest({ digest }: { digest: DigestData }) {
    const feeds = useSources<FeedSource[]>('/api/subscriptions');
    const newsletters = useSources<{ active: unknown[]; stopped: unknown[] }>('/api/newsletters');
    // Servers without newsletter delivery answer 404 here, which means the reader has no newsletters.
    const noSources = feeds.status === 'loaded' && newsletters.status !== 'loading'
        ? feeds.data.length === 0 && (newsletters.status === 'error' || newsletters.data.active.length === 0)
        : null;
    // After a source change the copy waits for the digest's own answer, which may turn this day into collecting instead.
    const [changedAt, setChangedAt] = useState<DigestData | null>(null);
    useEffect(() => {
        const change = () => setChangedAt(digest);
        subscriptionChanges.addEventListener('change', change);
        return () => subscriptionChanges.removeEventListener('change', change);
    }, [digest]);
    const waiting = changedAt === digest;
    const [held, setHeld] = useState(noSources);
    if (!waiting && held !== noSources) setHeld(noSources);
    return (
        <Skeleton
            data={waiting ? held : noSources}
            error={feeds.status === 'error' ? <ListError label="your sources" retry={feeds.retry} /> : undefined}
            label="Loading digest"
            sample
        >
            {(empty) => <EmptyDigestState noSources={empty} />}
        </Skeleton>
    );
}

function EmptyDigestState({ noSources }: { noSources: boolean }) {
    if (!noSources) {
        return <EmptyState title="A quiet day." description="None of your sources published anything on this day." />;
    }
    return (
        <EmptyState
            title={`${greeting(new Date())}. Nothing to read yet.`}
            description="Tell Morgenblau what you like to read, and new posts will gather here as they’re published."
            action={<AddSourceDialog trigger={<Button>Add your first source</Button>} />}
        />
    );
}

function greeting(now: Date): string {
    const hour = now.getHours();
    if (hour >= 5 && hour < 12) return 'Good morning';
    if (hour >= 12 && hour < 18) return 'Good afternoon';
    return 'Good evening';
}

// Primary posts lead with type and whitespace; the image appears only when the post has one that loads.
type GroupProps = { date: string; group: SourceGroup; onToggle: () => void; open: boolean };

function PrimaryItem({ date, group, onToggle, open }: GroupProps) {
    const entry = group.lead;
    // The digest re-polls every second while a sync runs; memoizing on the body skips reparsing unchanged posts.
    const excerpt = useMemo(() => entryExcerpt(entry.body), [entry.body]);
    const [failedImage, setFailedImage] = useState<string | null>(null);
    const image = entry.image && entry.image !== failedImage ? entry.image : null;
    const href = entryHref(entry.entrySlug, date);
    return (
        <li className="flex flex-col gap-3">
            <div className={image ? 'flex flex-col gap-3 sm:grid sm:grid-cols-[1fr_auto] sm:grid-rows-[auto_auto_1fr] sm:gap-x-8' : 'flex flex-col gap-3'}>
                <PrimarySource entry={entry} />
                <h2 className="text-2xl font-medium tracking-[-0.015em] text-balance break-words sm:col-start-1 sm:row-start-2 sm:text-3xl">
                    <Link className={`rounded-sm hover:text-foreground/75 ${entry.title ? 'text-foreground' : 'text-muted-foreground'}`} href={href}>{entry.title || 'Untitled'}</Link>
                </h2>
                {excerpt ? (
                    <p className="line-clamp-2 max-w-[60ch] self-start text-base text-muted-foreground text-pretty sm:col-start-1 sm:row-start-3">{excerpt}</p>
                ) : null}
                {image ? <PrimaryImage href={href} onError={() => setFailedImage(image)} src={image} /> : null}
            </div>
            {group.more.length > 0 ? <MoreFromSource className="text-xl" date={date} group={group} onToggle={onToggle} open={open} /> : null}
        </li>
    );
}

function PrimarySource({ entry }: { entry: DigestEntry }) {
    const sourceHref = readerSourceHref(entry.source);
    const className = 'flex min-w-0 items-center gap-2 self-start rounded-sm text-sm font-medium text-foreground sm:col-start-1 sm:row-start-1';
    const source = (
        <>
            <SourceIcon src={entry.source.faviconUrl} />
            <span className="truncate">{sourceLabel(entry)}</span>
        </>
    );
    if (!sourceHref) return <div className={className}>{source}</div>;
    return <Link className={`${className} hover:text-foreground/75`} href={sourceHref}>{source}</Link>;
}

// The title already carries the link for assistive tech; the image is a second, pointer-only way in.
function PrimaryImage({ href, onError, src }: { href: string; onError: () => void; src: string }) {
    return (
        <Link
            aria-hidden
            className="order-first mb-1 block aspect-[2/1] w-full overflow-hidden rounded-lg bg-muted ring-1 ring-foreground/5 sm:order-none sm:col-start-2 sm:row-span-2 sm:row-start-2 sm:mb-0 sm:aspect-[4/3] sm:w-44 sm:self-start"
            href={href}
            tabIndex={-1}
        >
            <img alt="" className="size-full object-cover" decoding="async" loading="lazy" onError={onError} src={src} />
        </Link>
    );
}

function sourceLabel(entry: DigestEntry): string {
    return entry.source.title || hostLabel(firstDefined(entry.source.siteUrl, entry.source.feedUrl));
}

function DigestItem({ date, group, onToggle, open }: GroupProps) {
    const entry = group.lead;
    const id = useId();
    const sourceTitle = sourceLabel(entry);
    const postHost = hostLabel(firstDefined(entry.url, entry.source.siteUrl, entry.source.feedUrl));
    const meta = [sourceTitle, postHost].filter(Boolean).join(' · ');
    const sourceHref = readerSourceHref(entry.source);
    return (
        <ListItem
            href={entryHref(entry.entrySlug, date)}
            icon={entry.source.faviconUrl}
            meta={sourceHref ? <Link className="rounded-sm hover:text-foreground" href={sourceHref}>{meta}</Link> : meta}
            note={group.more.length > 0 ? <MoreList className="text-lg" date={date} entries={group.more} id={id} open={open} /> : null}
            status={group.more.length > 0 ? <MoreToggle controls={id} group={group} onToggle={onToggle} open={open} /> : null}
            title={entry.title || 'Untitled'}
        />
    );
}

function MoreFromSource({ className, date, group, onToggle, open }: GroupProps & { className: string }) {
    const id = useId();
    return (
        <div>
            <div className="-ml-2">
                <MoreToggle controls={id} group={group} onToggle={onToggle} open={open} />
            </div>
            <MoreList className={className} date={date} entries={group.more} id={id} open={open} />
        </div>
    );
}

function MoreToggle({ controls, group, onToggle, open }: { controls: string; group: SourceGroup; onToggle: () => void; open: boolean }) {
    return (
        <button
            aria-controls={controls}
            aria-expanded={open}
            aria-label={`${open ? 'Hide' : 'Show'} ${group.more.length} more from ${sourceLabel(group.lead)}`}
            className="-my-1 inline-flex shrink-0 cursor-pointer items-center gap-1 rounded-full px-2 py-1 text-sm font-medium text-muted-foreground tabular-nums transition-[color,background-color,transform] duration-(--motion-duration-fast) hover:bg-foreground/5 hover:text-foreground active:scale-96 aria-expanded:bg-foreground/5 aria-expanded:text-foreground"
            onClick={onToggle}
            type="button"
        >
            +{group.more.length} more
            <ChevronDownIcon
                aria-hidden
                className={cn('size-3.5 shrink-0 transition-transform duration-(--motion-duration-ui) ease-(--ease-out) motion-reduce:transition-none', open && 'rotate-180')}
            />
        </button>
    );
}

// Closing runs faster than opening because the reader has already decided.
const MORE_TIMING = { open: 0.24, close: 0.16, ease: [0.16, 1, 0.3, 1] as const };

// The source already heads the group, so the folded posts show only their titles.
function MoreList({ className, date, entries, id, open }: { className: string; date: string; entries: DigestEntry[]; id: string; open: boolean }) {
    const reduced = useReducedMotion();
    return (
        <AnimatePresence initial={false}>
            {open ? (
                <motion.div
                    animate={{ height: 'auto', opacity: 1, transition: { duration: reduced ? 0 : MORE_TIMING.open, ease: MORE_TIMING.ease } }}
                    className="overflow-hidden"
                    exit={{ height: 0, opacity: 0, transition: { duration: reduced ? 0 : MORE_TIMING.close, ease: MORE_TIMING.ease } }}
                    id={id}
                    initial={{ height: 0, opacity: 0 }}
                >
                    <ol className="ml-[7px] flex flex-col gap-5 border-l border-border pt-5 pl-[17px]">
                        {entries.map((entry) => (
                            <li className="min-w-0" key={`${entry.id}-${entry.entrySlug}`}>
                                <Link className={cn('block min-w-0 truncate rounded-sm font-medium text-foreground hover:text-foreground/75', className)} href={entryHref(entry.entrySlug, date)} title={entry.title || 'Untitled'}>
                                    {entry.title || 'Untitled'}
                                </Link>
                            </li>
                        ))}
                    </ol>
                </motion.div>
            ) : null}
        </AnimatePresence>
    );
}

function hostLabel(value?: string | null): string {
    if (!value) return '';
    try {
        return new URL(value).hostname.replace(/^www\./, '');
    } catch {
        return value.replace(/^https?:\/\//, '').split('/')[0];
    }
}

function firstDefined(...values: Array<string | null | undefined>): string | undefined {
    return values.find(Boolean) || undefined;
}
