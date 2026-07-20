import {
    HourglassIcon,
    MoonIcon,
    PencilIcon,
    PulseIcon,
} from '@proicons/react';
import {
    Fragment,
    useCallback,
    useEffect,
    useMemo,
    useRef,
    useState,
} from 'react';
import { Link, useLocation } from 'wouter';

import { Favicon } from '@/components/favicon';
import { ListHighlight } from '@/components/list-highlight';
import { DeleteSourceButton } from '@/components/sources/delete-button';
import {
    EditSourceDialog,
    type SourcePatch,
} from '@/components/sources/edit-dialog';
import { api } from '@/lib/api';
import { toastMutationError } from '@/lib/mutation-toast';
import { sourceHref } from '@/lib/paths';
import { Button } from '@/components/ui/button';
import { Separator } from '@/components/ui/separator';
import { shortTimeAgo } from '@/lib/date';
import { useDocumentTitle } from '@/hooks/use-document-title';
import { useListNavKeyboard } from '@/hooks/use-list-nav-keyboard';
import { useListNavigation } from '@/hooks/use-list-navigation';
import {
    subscribeSubscriptionAdded,
    type AddedSubscription,
} from '@/lib/subscription-events';
import { mergeTagSuggestions } from '@/lib/tags';
import { useJobsPoll } from '@/hooks/use-jobs-poll';

type Frequency =
    | 'new'
    | 'daily'
    | 'weekly'
    | 'biweekly'
    | 'monthly'
    | 'irregular'
    | 'noPosts';

type Source = {
    uri: string;
    rkey: string;
    kind?: 'rss' | 'standardfeed';
    feedUrl: string;
    publication?: string;
    title?: string;
    siteUrl?: string;
    faviconUrl?: string;
    frequency?: Frequency;
    lastPublishedAt?: string;
    lastFetchedAt?: string;
    muted?: boolean;
    primary?: boolean;
    tags?: string[];
    value: {
        title?: string;
        feedUrl?: string;
        [k: string]: unknown;
    };
};

type State =
    | { kind: 'loading' }
    | { kind: 'ok'; records: Source[] }
    | { kind: 'error' };

const FREQUENCY_LABEL: Record<Frequency, string> = {
    new: 'New',
    daily: 'Daily',
    weekly: 'Weekly',
    biweekly: 'Biweekly',
    monthly: 'Monthly',
    irregular: 'Irregular',
    noPosts: 'No post',
};

// Stable empty list so list navigation doesn't reset every render while loading.
const EMPTY_SOURCES: Source[] = [];

export function Sources() {
    useDocumentTitle('Sources');
    const [, navigate] = useLocation();
    const [state, setState] = useState<State>({ kind: 'loading' });
    const [reloadTick, setReloadTick] = useState(0);
    const [hasPendingJobs, setHasPendingJobs] = useState(false);

    useEffect(() => {
        let cancelled = false;
        api<Source[]>('/api/subscriptions')
            .then((records) => {
                if (!cancelled) setState({ kind: 'ok', records });
            })
            .catch(() => {
                if (!cancelled) setState({ kind: 'error' });
            });
        return () => {
            cancelled = true;
        };
    }, [reloadTick]);

    useEffect(() => {
        return subscribeSubscriptionAdded((event) => {
            setState((cur) => {
                if (cur.kind !== 'ok') return cur;
                const byRkey = new Map(cur.records.map((r) => [r.rkey, r]));
                for (const added of event.records) {
                    byRkey.set(added.rkey, addedToSource(added));
                }
                return { kind: 'ok', records: Array.from(byRkey.values()) };
            });
            if (event.jobIds.length > 0) {
                setHasPendingJobs(true);
            }
        });
    }, []);

    const onJobsQuiet = useCallback(() => {
        setHasPendingJobs(false);
        setReloadTick((tick) => tick + 1);
    }, []);
    useJobsPoll(hasPendingJobs, onJobsQuiet);

    // Tag suggestions for the edit dialog: distinct tags already in use, deduped case-insensitively.
    const tagSuggestions = useMemo(
        () =>
            state.kind === 'ok'
                ? mergeTagSuggestions(
                      state.records.flatMap((record) => record.tags ?? []),
                  )
                : [],
        [state],
    );

    const sortedRecords = useMemo(
        () =>
            state.kind === 'ok'
                ? state.records.toSorted((a, b) =>
                      displayLabel(a).localeCompare(displayLabel(b)),
                  )
                : EMPTY_SOURCES,
        [state],
    );

    const listRef = useRef<HTMLDivElement>(null);
    const onOpen = useCallback(
        (s: Source) => {
            navigate(sourceHref(s.rkey));
        },
        [navigate],
    );
    const nav = useListNavigation(sortedRecords, onOpen);
    useListNavKeyboard(nav);

    const onPatch = async (rkey: string, patch: SourcePatch) => {
        try {
            await api(`/api/subscriptions/${rkey}`, {
                method: 'PATCH',
                body: patch,
            });
        } catch (err) {
            toastMutationError(err, "Couldn't save your changes. Try again.");
            return false;
        }
        const feedPatch = patch.feedUrl ? { feedUrl: patch.feedUrl } : {};
        setState((cur) => {
            if (cur.kind !== 'ok') return cur;
            return {
                ...cur,
                records: cur.records.map((r) =>
                    r.rkey === rkey
                        ? {
                            ...r,
                            title: patch.title,
                            primary: patch.primary,
                            tags: patch.tags,
                            ...feedPatch,
                            value: {
                                ...r.value,
                                title: patch.title,
                                primary: patch.primary,
                                tags: patch.tags,
                                ...feedPatch,
                            },
                        }
                        : r,
                ),
            };
        });
        // Re-pointing the feed dispatched a fetch; poll until it lands so the row picks up new entries and cadence.
        if (patch.feedUrl) setHasPendingJobs(true);
        return true;
    };

    const onDelete = async (rkey: string) => {
        try {
            await api(`/api/subscriptions/${rkey}`, { method: 'DELETE' });
        } catch (err) {
            toastMutationError(err, "Couldn't remove this source. Try again.");
            return false;
        }
        setState((cur) =>
            cur.kind === 'ok'
                ? { ...cur, records: cur.records.filter((r) => r.rkey !== rkey) }
                : cur,
        );
        return true;
    };

    if (state.kind === 'loading') {
        return (
            <main className="mx-auto max-w-2xl px-6 py-8">
                <p className="text-sm font-light text-muted-foreground">
                    Loading…
                </p>
            </main>
        );
    }
    if (state.kind === 'error') {
        return (
            <main className="mx-auto max-w-2xl px-6 py-8">
                <p className="text-sm font-light text-muted-foreground">
                    Couldn’t load your sources.
                </p>
            </main>
        );
    }
    if (state.records.length === 0) {
        return (
            <main className="mx-auto max-w-2xl px-6 py-8">
                <p className="text-sm font-light text-muted-foreground">
                    No sources yet — paste a URL to add one.
                </p>
            </main>
        );
    }

    return (
        <main className="mx-auto max-w-2xl px-6 py-8">
            <div className="overflow-hidden rounded-xl bg-card shadow-card">
                <SourcesMasthead count={sortedRecords.length} />
                <div
                    aria-hidden
                    className="mx-6 border-t border-border"
                />
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
                        {sortedRecords.map((r, i) => (
                            <Fragment key={r.rkey}>
                                {i > 0 ? (
                                    <li
                                        aria-hidden
                                        className="mx-6 border-t border-border"
                                    />
                                ) : null}
                                <SourceRow
                                    source={r}
                                    index={i}
                                    onActivate={nav.setActive}
                                    onPatch={onPatch}
                                    onDelete={onDelete}
                                    tagSuggestions={tagSuggestions}
                                />
                            </Fragment>
                        ))}
                    </ul>
                </div>
            </div>
        </main>
    );
}

function SourcesMasthead({ count }: { count: number }) {
    const noun = count === 1 ? 'source' : 'sources';

    return (
        <div className="flex flex-col gap-1 px-6 pt-6 pb-5">
            <p className="text-sm font-light text-muted-foreground">
                Your publication
            </p>
            <div className="flex items-baseline justify-between gap-4">
                <h2 className="text-xl font-medium">Sources</h2>
                <p className="shrink-0 text-sm text-muted-foreground">
                    {count} {noun}
                </p>
            </div>
        </div>
    );
}

function addedToSource(added: AddedSubscription): Source {
    const primary =
        typeof added.value.primary === 'boolean'
            ? added.value.primary
            : undefined;
    const tags = Array.isArray(added.value.tags)
        ? (added.value.tags as string[])
        : undefined;
    return {
        uri: added.uri,
        rkey: added.rkey,
        kind: added.kind,
        feedUrl: added.feedUrl,
        publication: added.publication,
        title: added.title,
        siteUrl: added.siteUrl,
        primary,
        tags,
        value: {
            ...added.value,
            feedUrl: added.feedUrl,
            title: added.title,
            siteUrl: added.siteUrl,
        },
    };
}

function displayLabel(s: Source): string {
    return (
        s.title ||
        (typeof s.value.title === 'string' ? s.value.title : '') ||
        s.feedUrl ||
        s.uri
    );
}

function siteDomain(s: Source): string {
    const candidate = s.siteUrl || s.feedUrl;
    // A standardfeed key is an at-uri; never show it as a "domain" (site URL fills in after first fetch).
    if (!candidate || candidate.startsWith('at://')) return '';
    try {
        return new URL(candidate).hostname.replace(/^www\./, '');
    } catch {
        return candidate;
    }
}

type RowProps = {
    source: Source;
    index: number;
    onActivate: (index: number) => void;
    onPatch: (rkey: string, patch: SourcePatch) => Promise<boolean>;
    onDelete: (rkey: string) => Promise<boolean>;
    tagSuggestions: string[];
};

function SourceRow({
    source,
    index,
    onActivate,
    onPatch,
    onDelete,
    tagSuggestions,
}: RowProps) {
    const [editing, setEditing] = useState(false);
    const title = displayLabel(source);
    const domain = siteDomain(source);
    const frequency = source.frequency ?? 'noPosts';

    return (
        <>
            <li
                data-nav-row=""
                onMouseEnter={() => onActivate(index)}
                className="relative flex items-start justify-between gap-3 px-5 py-4 transition-colors duration-200 ease-out has-[a:focus-visible]:outline-1 has-[a:focus-visible]:-outline-offset-2 has-[a:focus-visible]:outline-solid has-[a:focus-visible]:outline-ring"
            >
                <Link
                    href={sourceHref(source.rkey)}
                    aria-label={title}
                    className="absolute inset-0 outline-none"
                />
                <div className="pointer-events-none min-w-0 flex-1">
                    <div className="flex items-center gap-1.5 text-xs font-light text-muted-foreground">
                        <Favicon src={source.faviconUrl} className="size-3.5 shrink-0" />
                        <span className="truncate">{domain}</span>
                    </div>
                    <h3 className="mt-0.5 truncate text-base font-medium tracking-tight">
                        {title}
                    </h3>
                    <div className="mt-2 flex items-center gap-3 text-xs font-light text-muted-foreground">
                        <span className="inline-flex items-center gap-1">
                            <PulseIcon className="size-3.5" />
                            {FREQUENCY_LABEL[frequency]}
                        </span>
                        {frequency !== 'noPosts' && source.lastPublishedAt ? (
                            <span className="inline-flex items-center gap-1">
                                <HourglassIcon className="size-3.5" />
                                {shortTimeAgo(source.lastPublishedAt)}
                            </span>
                        ) : null}
                        {source.muted ? (
                            <span className="inline-flex items-center gap-1">
                                <MoonIcon className="size-3.5" />
                                {source.lastFetchedAt
                                    ? `Quiet · last update ${shortTimeAgo(source.lastFetchedAt)}`
                                    : 'Quiet'}
                            </span>
                        ) : null}
                        {source.kind === 'standardfeed' ? (
                            <span className="inline-flex items-center gap-1">
                                <span aria-hidden className="text-sm leading-none font-medium">
                                    @
                                </span>
                                ATProto
                            </span>
                        ) : null}
                    </div>
                </div>
                <div className="relative z-10 flex shrink-0 items-center gap-1">
                    <Button
                        variant="ghost"
                        size="icon-sm"
                        aria-label="Edit source"
                        className="text-muted-foreground"
                        onClick={() => setEditing(true)}
                    >
                        <PencilIcon className="size-3.5" />
                    </Button>
                    <Separator orientation="vertical" className="h-5" />
                    <DeleteSourceButton
                        onConfirm={() => onDelete(source.rkey)}
                    />
                </div>
            </li>
            <EditSourceDialog
                open={editing}
                onOpenChange={setEditing}
                initialTitle={title}
                initialPrimary={source.primary ?? false}
                initialTags={source.tags ?? []}
                initialFeedUrl={source.feedUrl}
                tagSuggestions={tagSuggestions}
                onSave={(patch) => onPatch(source.rkey, patch)}
            />
        </>
    );
}

