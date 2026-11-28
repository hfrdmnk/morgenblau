import { AnimatePresence, useReducedMotion } from 'motion/react';
import { useEffect, useId, useMemo, useState } from 'react';
import { useLocation } from 'wouter';
import { useSearch } from 'wouter/use-browser-location';

import { AddSourceDialog } from '@/components/add-source-dialog';
import { ChevronDownIcon } from '@/components/icons';
import { ItemList, ListError, ListItem } from '@/components/item-list';
import { Skeleton } from '@/components/ui/skeleton';
import { EmptyState } from '@/components/status-state';
import { Button } from '@/components/ui/button';
import { Link } from '@/components/ui/link';
import { FeaturePost, FoldedPosts, PostList, PostRow } from '@/components/ui/post-list';
import { SourceIcon } from '@/components/ui/source-icon';
import { Frame } from '@/design/frame';
import { MotionFrame } from '@/design/motion';
import { Text } from '@/design/text';
import { useCommands } from '@/hooks/use-commands';
import { useDigest, type DigestEntry } from '@/hooks/use-digest';
import { useDocumentTitle } from '@/hooks/use-document-title';
import { useShortcuts } from '@/hooks/use-shortcuts';
import { useSources, type FeedSource } from '@/hooks/use-sources';
import { subscriptionChanges } from '@/lib/add-source';
import { ACTIONS } from '@/lib/commands';
import { addCalendarDays, selectedDigestDate, startOfToday, toDateKey } from '@/lib/digest-dates';
import { digestHref, entryHref, readerSourceHref } from '@/lib/paths';
import { entryExcerpt } from '@/lib/reader';
import { showShortcutNotice } from '@/lib/shortcuts';

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
        <Frame as="main" gap="3xl" id="main" maxWidth="3xl" paddingX={{ base: 'xl', sm: '3xl' }} width="full">
            <Text as="h1" size="base" tone="muted" weight="book">
                {dayName.format(selectedDate)}
            </Text>
            <DigestContent date={selectedKey} state={digest} />
        </Frame>
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
    const later = () =>
        isToday ? showShortcutNotice('Today is the latest digest') : show(addCalendarDays(selected, 1));
    useShortcuts({
        ArrowLeft: earlier,
        h: earlier,
        ArrowRight: later,
        l: later,
        'Shift+T': () => (isToday ? showShortcutNotice('Already showing today') : show(today)),
    });
    useCommands([
        { id: 'day:previous', group: ACTIONS, label: 'Previous day', keywords: ['earlier', 'back'], run: earlier },
        ...(isToday
            ? []
            : [
                  { id: 'day:next', group: ACTIONS, label: 'Next day', keywords: ['later', 'forward'], run: later },
                  { id: 'day:today', group: ACTIONS, label: 'Today', run: () => show(today) },
              ]),
        ...entries.map((entry) => ({
            id: `headline:${entry.entrySlug}`,
            group,
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
        'A sample headline that runs about as long as most do',
        'A shorter sample headline',
        'Another sample headline of a typical length',
        'A sample headline',
    ].map((title, index) => ({ id: index + 1, entrySlug: `${index}`, title, body: null, source: SAMPLE_SOURCE })),
};

function DigestContent({ date, state }: { date: string; state: ReturnType<typeof useDigest> }) {
    const collecting = state.status === 'loaded' && state.data.hasActiveJob && state.data.entries.length === 0;
    return (
        <Skeleton
            data={state.status === 'loaded' && !collecting ? state.data : null}
            error={state.status === 'error' ? <ListError label="this digest" retry={state.retry} /> : undefined}
            label="Loading digest"
            sample={SAMPLE_DIGEST}
        >
            {(digest) => <LoadedDigest date={date} digest={digest} key={date} />}
        </Skeleton>
    );
}

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
        <PostList>
            <Frame gap="7xl">
                {primary.length > 0 ? (
                    <Frame aria-label="Primary sources" as="ol" gap="5xl">
                        {primary.map((group) => (
                            <PrimaryItem
                                date={date}
                                group={group}
                                key={group.key}
                                onToggle={() => toggle(group.key)}
                                open={open.has(group.key)}
                            />
                        ))}
                    </Frame>
                ) : null}
                {rest.length > 0 ? (
                    <ItemList ordered>
                        {rest.map((group) => (
                            <DigestItem
                                date={date}
                                group={group}
                                key={group.key}
                                onToggle={() => toggle(group.key)}
                                open={open.has(group.key)}
                            />
                        ))}
                    </ItemList>
                ) : null}
            </Frame>
        </PostList>
    );
}

type SourceGroup = { key: string; lead: DigestEntry; more: DigestEntry[] };

function groupBySource(entries: DigestEntry[]): SourceGroup[] {
    const groups = new Map<string, SourceGroup>();
    for (const entry of entries) {
        const { source } = entry;
        const key =
            source.kind === 'newsletter'
                ? `newsletter:${source.id}`
                : source.feedUrl
                  ? `feed:${source.feedUrl}`
                  : `entry:${entry.id}`;
        const group = groups.get(key);
        if (group) group.more.push(entry);
        else groups.set(key, { key, lead: entry, more: [] });
    }
    return [...groups.values()];
}

function EmptyDigest({ digest }: { digest: DigestData }) {
    const feeds = useSources<FeedSource[]>('/api/subscriptions');
    const newsletters = useSources<{ active: unknown[]; stopped: unknown[] }>('/api/newsletters');
    const noSources =
        feeds.status === 'loaded' && newsletters.status !== 'loading'
            ? feeds.data.length === 0 && (newsletters.status === 'error' || newsletters.data.active.length === 0)
            : null;
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
        return (
            <EmptyState
                symbol="calm"
                title="A quiet day."
                description="None of your sources published anything on this day."
            />
        );
    }
    return (
        <EmptyState
            symbol="calm"
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

type GroupProps = { date: string; group: SourceGroup; onToggle: () => void; open: boolean };

function PrimaryItem({ date, group, onToggle, open }: GroupProps) {
    const entry = group.lead;
    const excerpt = useMemo(() => entryExcerpt(entry.body), [entry.body]);
    const [failedImage, setFailedImage] = useState<string | null>(null);
    const image = entry.image && entry.image !== failedImage ? entry.image : null;
    const href = entryHref(entry.entrySlug, date);
    return (
        <Frame as="li" gap="m">
            <FeaturePost
                excerpt={
                    excerpt ? (
                        <Text lineClamp={2} size="base" tone="muted" wrap="pretty">
                            {excerpt}
                        </Text>
                    ) : null
                }
                headline={
                    <Text
                        as="h2"
                        break="words"
                        size={{ base: '2xl', sm: '3xl' }}
                        tone={entry.title ? undefined : 'muted'}
                        weight="medium"
                        wrap="balance"
                    >
                        <Link data-post-link href={href} variant={entry.title ? 'title' : 'quiet'}>
                            {entry.title || 'Untitled'}
                        </Link>
                    </Text>
                }
                image={image ? { href, onError: () => setFailedImage(image), src: image } : null}
                source={<PrimarySource entry={entry} />}
            />
            {group.more.length > 0 ? (
                <MoreFromSource date={date} group={group} onToggle={onToggle} open={open} size="xl" />
            ) : null}
        </Frame>
    );
}

function PrimarySource({ entry }: { entry: DigestEntry }) {
    const sourceHref = readerSourceHref(entry.source);
    const source = (
        <>
            <SourceIcon src={entry.source.faviconUrl} />
            <Text as="span" size="sm" truncate weight="medium">
                {sourceLabel(entry)}
            </Text>
        </>
    );
    if (!sourceHref)
        return (
            <Frame align="center" direction="row" gap="s" minWidth={0}>
                {source}
            </Frame>
        );
    return (
        <Link href={sourceHref} variant="title-row">
            {source}
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
            meta={
                sourceHref ? (
                    <Link href={sourceHref} variant="quiet">
                        {meta}
                    </Link>
                ) : (
                    meta
                )
            }
            note={
                group.more.length > 0 ? (
                    <MoreList date={date} entries={group.more} id={id} open={open} size="lg" />
                ) : null
            }
            status={
                group.more.length > 0 ? (
                    <MoreToggle controls={id} group={group} onToggle={onToggle} open={open} size="pill" />
                ) : null
            }
            title={entry.title || 'Untitled'}
        />
    );
}

type TitleSize = 'lg' | 'xl';

function MoreFromSource({ date, group, onToggle, open, size }: GroupProps & { size: TitleSize }) {
    const id = useId();
    return (
        <Frame>
            <Frame display="block">
                <MoreToggle controls={id} group={group} onToggle={onToggle} open={open} size="pill-start" />
            </Frame>
            <MoreList date={date} entries={group.more} id={id} open={open} size={size} />
        </Frame>
    );
}

function MoreToggle({
    controls,
    group,
    onToggle,
    open,
    size,
}: {
    controls: string;
    group: SourceGroup;
    onToggle: () => void;
    open: boolean;
    size: 'pill' | 'pill-start';
}) {
    return (
        <Button
            aria-controls={controls}
            aria-expanded={open}
            aria-label={`${open ? 'Hide' : 'Show'} ${group.more.length} more from ${sourceLabel(group.lead)}`}
            onClick={onToggle}
            size={size}
            variant="disclosure"
        >
            <Text as="span" numeric="tabular">
                +{group.more.length} more
            </Text>
            <ChevronDownIcon aria-hidden rotate={open ? 180 : undefined} size="sm" />
        </Button>
    );
}

const MORE_TIMING = { open: 0.24, close: 0.16, ease: [0.16, 1, 0.3, 1] as const };

function MoreList({
    date,
    entries,
    id,
    open,
    size,
}: {
    date: string;
    entries: DigestEntry[];
    id: string;
    open: boolean;
    size: TitleSize;
}) {
    const reduced = useReducedMotion();
    return (
        <AnimatePresence initial={false}>
            {open ? (
                <MotionFrame
                    animate={{
                        height: 'auto',
                        opacity: 1,
                        transition: { duration: reduced ? 0 : MORE_TIMING.open, ease: MORE_TIMING.ease },
                    }}
                    exit={{
                        height: 0,
                        opacity: 0,
                        transition: { duration: reduced ? 0 : MORE_TIMING.close, ease: MORE_TIMING.ease },
                    }}
                    id={id}
                    initial={{ height: 0, opacity: 0 }}
                    overflow="hidden"
                >
                    <FoldedPosts>
                        {entries.map((entry) => (
                            <PostRow as="li" key={`${entry.id}-${entry.entrySlug}`} size="compact">
                                <Text as="span" display="block" size={size} truncate weight="medium">
                                    <Link
                                        data-post-link
                                        href={entryHref(entry.entrySlug, date)}
                                        title={entry.title || 'Untitled'}
                                        variant="title"
                                    >
                                        {entry.title || 'Untitled'}
                                    </Link>
                                </Text>
                            </PostRow>
                        ))}
                    </FoldedPosts>
                </MotionFrame>
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
