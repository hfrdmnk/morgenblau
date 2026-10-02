import { Link as RouterLink } from 'wouter';
import { useSearch } from 'wouter/use-browser-location';

import { useDigest, type DigestEntry } from '@/hooks/use-digest';
import { parseDigestDate, toDateKey } from '@/lib/digest-dates';
import { entryHref } from '@/lib/paths';

const dayName = new Intl.DateTimeFormat(undefined, { weekday: 'long' });
export function Digest() {
    const search = useSearch();
    const today = startOfToday();
    const requestedDate = parseDigestDate(new URLSearchParams(search).get('date'));
    const selectedDate =
        requestedDate && requestedDate <= today ? requestedDate : today;
    const selectedKey = toDateKey(selectedDate);
    const digest = useDigest(selectedKey);

    return (
        <main className="mx-auto w-full max-w-3xl px-5 pb-20 pt-20 sm:px-8 md:pt-24">
            <h1 className="mb-8 text-base font-medium text-muted-foreground">
                {dayName.format(selectedDate)}
            </h1>
            <DigestContent date={selectedKey} state={digest} />
        </main>
    );
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
