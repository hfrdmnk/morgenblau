import { Link as RouterLink } from 'wouter';
import { useSearch } from 'wouter/use-browser-location';

import { AccountMenu, AccountMenuSkeleton } from '@/components/account-menu';
import { ChevronDownIcon, DigestIcon, PlusIcon } from '@/components/icons';
import { Button } from '@/components/ui/button';
import { useAppProfile } from '@/hooks/use-app-profile';
import { useDigest, type DigestEntry } from '@/hooks/use-digest';
import {
    digestDateRange,
    parseDigestDate,
    toDateKey,
} from '@/lib/digest-dates';
import { digestHref, entryHref } from '@/lib/paths';
import { cn } from '@/lib/utils';

const dayName = new Intl.DateTimeFormat(undefined, { weekday: 'long' });
const dayNumber = new Intl.DateTimeFormat(undefined, { day: '2-digit' });
const monthName = new Intl.DateTimeFormat(undefined, { month: 'short' });
const fullDate = new Intl.DateTimeFormat(undefined, {
    weekday: 'long',
    day: 'numeric',
    month: 'long',
    year: 'numeric',
});

export function Digest() {
    const search = useSearch();
    const today = startOfToday();
    const requestedDate = parseDigestDate(new URLSearchParams(search).get('date'));
    const selectedDate =
        requestedDate && requestedDate <= today ? requestedDate : today;
    const todayKey = toDateKey(today);
    const selectedKey = toDateKey(selectedDate);
    const dates = digestDateRange(selectedDate, today);
    const emptyDateSlots = 7 - dates.length;
    const digest = useDigest(selectedKey);
    const profile = useAppProfile();

    return (
        <div className="min-h-dvh bg-background">
            <header className="grid grid-cols-[1fr_auto] items-start gap-y-6 px-5 pt-6 sm:px-8 sm:pt-8 md:grid-cols-3 md:items-center md:px-14 md:pt-10">
                <button
                    aria-haspopup="listbox"
                    className="inline-flex w-fit items-center gap-3 rounded-md text-muted-foreground"
                    type="button"
                >
                    <DigestIcon className="size-8 text-atmosphere-blue" />
                    <span className="text-sm">Digest</span>
                    <ChevronDownIcon className="size-4 text-subtle-foreground/50" />
                </button>

                <nav
                    aria-label="Digest date"
                    className="col-span-2 row-start-2 inline-grid grid-cols-7 items-start justify-self-center gap-2.5 sm:gap-3.5 md:col-span-1 md:col-start-2 md:row-start-1"
                >
                    {dates.map((date) => (
                        <DateOption
                            date={date}
                            key={toDateKey(date)}
                            selectedKey={selectedKey}
                            todayKey={todayKey}
                        />
                    ))}
                    {Array.from({ length: emptyDateSlots }, (_, index) => (
                        <span
                            aria-hidden="true"
                            className="invisible w-5 text-center text-sm"
                            key={index}
                        >
                            <span className="block">00</span>
                            <span className="block">Mon</span>
                        </span>
                    ))}
                </nav>

                <div className="col-start-2 row-start-1 flex items-center justify-end gap-4 md:col-start-3">
                    <Button
                        aria-label="Add a source"
                        className="text-subtle-foreground"
                        size="icon-sm"
                        type="button"
                        variant="ghost"
                    >
                        <PlusIcon className="size-5" />
                    </Button>
                    <ProfileAvatar profile={profile} />
                </div>
            </header>

            <main className="mx-auto w-full max-w-3xl px-5 pb-20 pt-20 sm:px-8 md:pt-24">
                <h1 className="mb-8 text-base font-medium text-muted-foreground">
                    {dayName.format(selectedDate)}
                </h1>
                <DigestContent date={selectedKey} state={digest} />
            </main>
        </div>
    );
}

function DateOption({
    date,
    selectedKey,
    todayKey,
}: {
    date: Date;
    selectedKey: string;
    todayKey: string;
}) {
    const key = toDateKey(date);
    const selected = key === selectedKey;
    return (
        <RouterLink
            aria-current={selected ? 'date' : undefined}
            aria-label={fullDate.format(date)}
            className={cn(
                'flex w-5 flex-col items-center rounded-sm text-center text-sm transition-colors duration-(--motion-duration-fast) hover:text-foreground',
                {
                    'text-primary': selected,
                    'text-subtle-foreground': !selected,
                },
            )}
            href={dateHref(key, todayKey)}
        >
            <span className="block">{dayNumber.format(date)}</span>
            <span className={cn('block text-sm', { invisible: !selected })}>
                {monthName.format(date)}
            </span>
        </RouterLink>
    );
}

function dateHref(date: string, today: string): string {
    return digestHref(date === today ? undefined : date);
}

function DigestContent({ date, state }: { date: string; state: ReturnType<typeof useDigest> }) {
    if (state.status === 'loading') return <DigestSkeleton />;
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
        <ol className="space-y-9">
            {digest.entries.map((entry) => (
                <DigestItem date={date} entry={entry} key={`${entry.id}-${entry.entrySlug}`} />
            ))}
        </ol>
    );
}

function DigestItem({ date, entry }: { date: string; entry: DigestEntry }) {
    const sourceTitle = entry.source.title || hostLabel(firstDefined(entry.source.siteUrl, entry.source.feedUrl));
    const postHost = hostLabel(firstDefined(entry.url, entry.source.siteUrl, entry.source.feedUrl));
    const sourceDetails = [sourceTitle, postHost].filter(Boolean).join(' · ');

    return (
        <li>
            <RouterLink
                className="inline-block rounded-sm text-xl font-medium text-foreground hover:text-foreground/75"
                href={entryHref(entry.entrySlug, date)}
            >
                {entry.title || 'Untitled'}
            </RouterLink>
            <div className="mt-2.5 flex items-center gap-2.5 text-sm text-subtle-foreground">
                {entry.source.faviconUrl ? (
                    <img
                        alt=""
                        className="size-4 shrink-0 rounded-sm object-cover"
                        loading="lazy"
                        onError={(event) => {
                            event.currentTarget.hidden = true;
                        }}
                        referrerPolicy="no-referrer"
                        src={entry.source.faviconUrl}
                    />
                ) : null}
                <span className="truncate">{sourceDetails}</span>
            </div>
        </li>
    );
}

function ProfileAvatar({ profile }: { profile: ReturnType<typeof useAppProfile> }) {
    if (profile.kind === 'loading') {
        return <AccountMenuSkeleton />;
    }
    if (profile.kind === 'error') {
        return <span aria-label="Profile unavailable" className="size-8 rounded-full bg-muted" />;
    }
    return <AccountMenu profile={profile.profile} />;
}

function DigestSkeleton() {
    return (
        <div aria-label="Loading digest" className="space-y-10" role="status">
            {[72, 88, 64].map((width) => (
                <div className="space-y-3" key={width}>
                    <div
                        className="h-7 animate-pulse rounded bg-muted"
                        style={{ width: `${width}%` }}
                    />
                    <div className="h-5 w-2/5 animate-pulse rounded bg-muted" />
                </div>
            ))}
        </div>
    );
}

function startOfToday(): Date {
    const now = new Date();
    return new Date(now.getFullYear(), now.getMonth(), now.getDate());
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
