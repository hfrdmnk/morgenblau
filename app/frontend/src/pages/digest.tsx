import { Link, useLocation } from 'wouter';
import { useSearch } from 'wouter/use-browser-location';

import { AddSourceDialog } from '@/components/add-source-dialog';
import { ItemList, ListError, ListItem, ListSkeleton } from '@/components/item-list';
import { EmptyState } from '@/components/status-state';
import { Button } from '@/components/ui/button';
import { useDigest, type DigestEntry } from '@/hooks/use-digest';
import { useDocumentTitle } from '@/hooks/use-document-title';
import { useShortcuts } from '@/hooks/use-shortcuts';
import { useSources, type FeedSource } from '@/hooks/use-sources';
import { addCalendarDays, selectedDigestDate, startOfToday, toDateKey } from '@/lib/digest-dates';
import { digestHref, entryHref, readerSourceHref } from '@/lib/paths';
import { showShortcutNotice } from '@/lib/shortcuts';

const dayName = new Intl.DateTimeFormat(undefined, { weekday: 'long' });
export function Digest() {
    const search = useSearch();
    const today = startOfToday();
    const selectedDate = selectedDigestDate(search, today);
    const selectedKey = toDateKey(selectedDate);
    const digest = useDigest(selectedKey);
    useDayShortcuts(selectedDate, today);
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

function useDayShortcuts(selected: Date, today: Date) {
    const [, navigate] = useLocation();
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
}

function DigestContent({ date, state }: { date: string; state: ReturnType<typeof useDigest> }) {
    if (state.status === 'loading') return <ListSkeleton label="Loading digest" />;
    if (state.status === 'error') return <ListError label="this digest" retry={state.retry} />;
    return <LoadedDigest date={date} digest={state.data} />;
}

function LoadedDigest({
    date,
    digest,
}: {
    date: string;
    digest: Extract<ReturnType<typeof useDigest>, { status: 'loaded' }>['data'];
}) {
    if (digest.entries.length === 0) {
        if (digest.hasActiveJob) return <EmptyDay>New entries are still being collected.</EmptyDay>;
        return <EmptyDigest />;
    }
    return (
        <ItemList ordered>
            {digest.entries.map((entry) => (
                <DigestItem date={date} entry={entry} key={`${entry.id}-${entry.entrySlug}`} />
            ))}
        </ItemList>
    );
}

function EmptyDay({ children }: { children: string }) {
    return <p className="py-8 text-sm text-muted-foreground">{children}</p>;
}

// An empty day reads differently for a reader who has nothing to follow yet, so it checks before choosing the copy.
function EmptyDigest() {
    const feeds = useSources<FeedSource[]>('/api/subscriptions');
    const newsletters = useSources<{ active: unknown[]; stopped: unknown[] }>('/api/newsletters');
    if (feeds.status === 'loading' || newsletters.status === 'loading') return <ListSkeleton label="Loading digest" />;
    if (feeds.status === 'error') return <ListError label="your sources" retry={feeds.retry} />;
    // Servers without newsletter delivery answer 404 here, which means the reader has no newsletters.
    const noSources = feeds.data.length === 0
        && (newsletters.status === 'error' || newsletters.data.active.length === 0);
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

function DigestItem({ date, entry }: { date: string; entry: DigestEntry }) {
    const sourceTitle = entry.source.title || hostLabel(firstDefined(entry.source.siteUrl, entry.source.feedUrl));
    const postHost = hostLabel(firstDefined(entry.url, entry.source.siteUrl, entry.source.feedUrl));
    const meta = [sourceTitle, postHost].filter(Boolean).join(' · ');
    const sourceHref = readerSourceHref(entry.source);
    return (
        <ListItem
            href={entryHref(entry.entrySlug, date)}
            icon={entry.source.faviconUrl}
            meta={sourceHref ? <Link className="rounded-sm hover:text-foreground" href={sourceHref}>{meta}</Link> : meta}
            title={entry.title || 'Untitled'}
        />
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
