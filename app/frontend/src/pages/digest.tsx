import { Link, useLocation } from 'wouter';
import { useSearch } from 'wouter/use-browser-location';

import { ItemList, ListItem, ListSkeleton } from '@/components/item-list';
import { useDigest, type DigestEntry } from '@/hooks/use-digest';
import { useShortcuts } from '@/hooks/use-shortcuts';
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

    return (
        <main className="mx-auto w-full max-w-3xl px-5 pb-20 pt-20 sm:px-8 md:pt-24">
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
    if (state.status === 'error') {
        return (
            <p className="py-8 text-sm text-muted-foreground" role="alert">
                The digest could not be loaded.
            </p>
        );
    }
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
        return (
            <p className="py-8 text-sm text-muted-foreground">
                {digest.hasActiveJob
                    ? 'New entries are still being collected.'
                    : 'Nothing was published here today.'}
            </p>
        );
    }
    return (
        <ItemList ordered>
            {digest.entries.map((entry) => (
                <DigestItem date={date} entry={entry} key={`${entry.id}-${entry.entrySlug}`} />
            ))}
        </ItemList>
    );
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
