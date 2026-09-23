import {
    HourglassIcon,
    MailIcon,
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
import { useAuthedMe } from '@/hooks/use-authed-me';
import { useListNavKeyboard } from '@/hooks/use-list-nav-keyboard';
import {
    useListNavigation,
    type ListNavigation,
} from '@/hooks/use-list-navigation';
import {
    subscribeSubscriptionAdded,
    type AddedSubscription,
} from '@/lib/subscription-events';
import { mergeTagSuggestions } from '@/lib/tags';
import { useJobsPoll } from '@/hooks/use-jobs-poll';
import {
    emitNewsletterMutation,
    enableNewsletter,
    fetchNewsletters,
    patchNewsletter,
    stopNewsletter,
    subscribeNewsletterMutation,
    type NewsletterPatch,
    type NewsletterSource,
    type NewsletterSources,
} from '@/lib/newsletters';
import { newsletterSourceHref } from '@/lib/paths';
import { NewsletterSourceActions } from '@/components/newsletters/source-actions';

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
    | {
          kind: 'ok';
          records: Source[];
          newsletters: NewsletterSources;
          feedsError: boolean;
          newslettersError: boolean;
      }
    | { kind: 'error' };

type SourceNavItem =
    | { kind: 'feed'; source: Source }
    | { kind: 'newsletter'; source: NewsletterSource };

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
const EMPTY_NEWSLETTERS: NewsletterSources = { active: [], stopped: [] };
type SyncStatus = 'pending' | 'running' | 'done' | 'failed';

async function latestSyncStatus(): Promise<SyncStatus | null> {
    const job = await api<{ status: SyncStatus } | null>('/api/jobs/latest');
    return job?.status ?? null;
}

function syncIsRunning(status: SyncStatus | null): boolean {
    return status === 'pending' || status === 'running';
}

export function Sources() {
    useDocumentTitle('Sources');
    const [, navigate] = useLocation();
    const { state, setState, setHasPendingJobs, syncFailed } = useSourcesData();
    const view = useMemo(() => sourceView(state), [state]);
    const navItems = useMemo(
        () => sourceNavItems(view.records, view.newsletters),
        [view],
    );
    const onOpen = useCallback(
        (item: SourceNavItem) => {
            navigate(sourceNavHref(item));
        },
        [navigate],
    );
    const nav = useListNavigation(navItems, onOpen);
    useListNavKeyboard(nav);
    const feedMutations = useFeedMutations(setState, setHasPendingJobs);
    const onNewsletterPatch = useNewsletterPatch(setState);

    return (
        <SourcesPage
            state={state}
            view={view}
            nav={nav}
            onPatch={feedMutations.onPatch}
            onDelete={feedMutations.onDelete}
            onNewsletterPatch={onNewsletterPatch}
            syncFailed={syncFailed}
        />
    );
}

function useSourcesData() {
    const did = useAuthedMe()?.did;
    const [state, setState] = useState<State>({ kind: 'loading' });
    const [reloadTick, setReloadTick] = useState(0);
    const [hasPendingJobs, setHasPendingJobs] = useState(false);
    const [syncFailed, setSyncFailed] = useState(false);

    useEffect(() => {
        let cancelled = false;
        let timer: ReturnType<typeof setTimeout> | null = null;
        const check = async () => {
            const status = await latestSyncStatus().catch(() => null);
            if (cancelled) return;
            setSyncFailed(status === 'failed');
            if (syncIsRunning(status)) timer = setTimeout(check, 1500);
        };
        check();
        return () => {
            cancelled = true;
            if (timer) clearTimeout(timer);
        };
    }, [did]);

    useEffect(() => {
        let cancelled = false;
        Promise.allSettled([
            api<Source[]>('/api/subscriptions'),
            fetchNewsletters(),
        ]).then(([feedsResult, newslettersResult]) => {
            if (cancelled) return;
            setState((current) =>
                mergeSourceResults(current, feedsResult, newslettersResult),
            );
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
                return {
                    ...cur,
                    records: Array.from(byRkey.values()),
                };
            });
            if (event.jobIds.length > 0) {
                setHasPendingJobs(true);
            }
        });
    }, []);

    useEffect(
        () =>
            subscribeNewsletterMutation(() =>
                setReloadTick((tick) => tick + 1),
            ),
        [],
    );

    const onJobsQuiet = useCallback(() => {
        setHasPendingJobs(false);
        setReloadTick((tick) => tick + 1);
    }, []);
    useJobsPoll(hasPendingJobs, onJobsQuiet);

    return { state, setState, setHasPendingJobs, syncFailed };
}

function mergeSourceResults(
    current: State,
    feeds: PromiseSettledResult<Source[]>,
    newsletters: PromiseSettledResult<NewsletterSources>,
): State {
    if (feeds.status === 'rejected' && newsletters.status === 'rejected') {
        return { kind: 'error' };
    }
    const previous = current.kind === 'ok' ? current : emptySourceView();
    return {
        kind: 'ok',
        records: settledValue(feeds, previous.records),
        newsletters: settledValue(newsletters, previous.newsletters),
        feedsError: feeds.status === 'rejected',
        newslettersError: newsletters.status === 'rejected',
    };
}

function settledValue<T>(result: PromiseSettledResult<T>, fallback: T): T {
    if (result.status === 'fulfilled') return result.value;
    return fallback;
}

function emptySourceView() {
    return {
        records: EMPTY_SOURCES,
        newsletters: EMPTY_NEWSLETTERS,
        feedsError: false,
        newslettersError: false,
    };
}

function sourceView(state: State) {
    if (state.kind === 'ok') {
        return {
            ...state,
            records: state.records.toSorted((a, b) =>
                displayLabel(a).localeCompare(displayLabel(b)),
            ),
            tagSuggestions: mergeTagSuggestions(
                state.records.flatMap((record) => record.tags ?? []),
                state.newsletters.active.flatMap((source) => source.tags),
                state.newsletters.stopped.flatMap((source) => source.tags),
            ),
        };
    }
    return { ...emptySourceView(), tagSuggestions: [] };
}

function sourceNavItems(
    records: Source[],
    newsletters: NewsletterSources,
): SourceNavItem[] {
    return [
        ...records.map((source) => ({ kind: 'feed' as const, source })),
        ...newsletters.active.map((source) => ({
            kind: 'newsletter' as const,
            source,
        })),
        ...newsletters.stopped.map((source) => ({
            kind: 'newsletter' as const,
            source,
        })),
    ];
}

function sourceNavHref(item: SourceNavItem): string {
    if (item.kind === 'feed') return sourceHref(item.source.rkey);
    return newsletterSourceHref(item.source.id);
}

function useFeedMutations(
    setState: React.Dispatch<React.SetStateAction<State>>,
    setHasPendingJobs: React.Dispatch<React.SetStateAction<boolean>>,
) {
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
        setState((current) => patchFeedState(current, rkey, patch));
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
        setState((current) => removeFeed(current, rkey));
        return true;
    };

    return { onPatch, onDelete };
}

function patchFeedState(state: State, rkey: string, patch: SourcePatch): State {
    if (state.kind !== 'ok') return state;
    return {
        ...state,
        records: state.records.map((source) => patchFeed(source, rkey, patch)),
    };
}

function patchFeed(source: Source, rkey: string, patch: SourcePatch): Source {
    if (source.rkey !== rkey) return source;
    const feedPatch = patch.feedUrl ? { feedUrl: patch.feedUrl } : {};
    return {
        ...source,
        title: patch.title,
        primary: patch.primary,
        tags: patch.tags,
        ...feedPatch,
        value: {
            ...source.value,
            title: patch.title,
            primary: patch.primary,
            tags: patch.tags,
            ...feedPatch,
        },
    };
}

function removeFeed(state: State, rkey: string): State {
    if (state.kind !== 'ok') return state;
    return {
        ...state,
        records: state.records.filter((source) => source.rkey !== rkey),
    };
}

function useNewsletterPatch(
    setState: React.Dispatch<React.SetStateAction<State>>,
) {
    return async (
        id: string,
        patch: SourcePatch,
    ): Promise<boolean> => {
        const newsletterPatch: NewsletterPatch = {
            title: patch.title,
            primary: patch.primary,
            tags: patch.tags,
        };
        try {
            const updated = await patchNewsletter(id, newsletterPatch);
            setState((current) => replaceNewsletter(current, updated));
            emitNewsletterMutation();
            return true;
        } catch (error) {
            toastMutationError(error, "Couldn't save your changes. Try again.");
            return false;
        }
    };
}

function replaceNewsletter(state: State, updated: NewsletterSource): State {
    if (state.kind !== 'ok') return state;
    return {
        ...state,
        newsletters: {
            active: state.newsletters.active.map((source) =>
                updatedNewsletter(source, updated),
            ),
            stopped: state.newsletters.stopped.map((source) =>
                updatedNewsletter(source, updated),
            ),
        },
    };
}

function updatedNewsletter(
    source: NewsletterSource,
    updated: NewsletterSource,
): NewsletterSource {
    if (source.id === updated.id) return updated;
    return source;
}

type SourceView = ReturnType<typeof sourceView>;

function SourcesPage({
    state,
    view,
    nav,
    onPatch,
    onDelete,
    onNewsletterPatch,
    syncFailed,
}: {
    state: State;
    view: SourceView;
    nav: ListNavigation;
    onPatch: (rkey: string, patch: SourcePatch) => Promise<boolean>;
    onDelete: (rkey: string) => Promise<boolean>;
    onNewsletterPatch: (id: string, patch: SourcePatch) => Promise<boolean>;
    syncFailed: boolean;
}) {
    if (state.kind === 'loading') return <SourcesMessage>Loading…</SourcesMessage>;
    if (state.kind === 'error') {
        return <SourcesMessage>Couldn’t load your sources.</SourcesMessage>;
    }

    return (
        <LoadedSources
            view={view}
            nav={nav}
            onPatch={onPatch}
            onDelete={onDelete}
            onNewsletterPatch={onNewsletterPatch}
            syncFailed={syncFailed}
        />
    );
}

function LoadedSources({
    view,
    nav,
    onPatch,
    onDelete,
    onNewsletterPatch,
    syncFailed,
}: {
    view: SourceView;
    nav: ListNavigation;
    onPatch: (rkey: string, patch: SourcePatch) => Promise<boolean>;
    onDelete: (rkey: string) => Promise<boolean>;
    onNewsletterPatch: (id: string, patch: SourcePatch) => Promise<boolean>;
    syncFailed: boolean;
}) {
    const newsletterCount =
        view.newsletters.active.length + view.newsletters.stopped.length;
    if (hasNoSources(view, newsletterCount) && !syncFailed) {
        return <SourcesMessage>No sources yet — paste a URL to add one.</SourcesMessage>;
    }

    return (
        <main className="mx-auto max-w-2xl px-6 py-8">
            <SourcesMasthead count={view.records.length + newsletterCount} />
            <LoadWarning failed={syncFailed}>
                Some sources or saves may be out of date. Refresh the Digest to retry.
            </LoadWarning>
            <LoadWarning failed={view.feedsError}>Couldn’t load feeds.</LoadWarning>
            <LoadWarning failed={view.newslettersError}>
                Couldn’t load newsletters.
            </LoadWarning>
            <div className="mt-6 flex flex-col gap-6">
                <FeedSection
                    sources={view.records}
                    nav={nav}
                    onPatch={onPatch}
                    onDelete={onDelete}
                    tagSuggestions={view.tagSuggestions}
                />
                <OptionalNewsletterSection
                    title="Newsletters"
                    sources={view.newsletters.active}
                    navOffset={view.records.length}
                    nav={nav}
                    onPatch={onNewsletterPatch}
                    tagSuggestions={view.tagSuggestions}
                />
                <OptionalNewsletterSection
                    title="Stopped newsletters"
                    sources={view.newsletters.stopped}
                    navOffset={
                        view.records.length + view.newsletters.active.length
                    }
                    nav={nav}
                    onPatch={onNewsletterPatch}
                    tagSuggestions={view.tagSuggestions}
                />
            </div>
        </main>
    );
}

function hasNoSources(view: SourceView, newsletterCount: number): boolean {
    return (
        view.records.length === 0 &&
        newsletterCount === 0 &&
        !view.feedsError &&
        !view.newslettersError
    );
}

function SourcesMessage({ children }: { children: React.ReactNode }) {
    return (
        <main className="mx-auto max-w-2xl px-6 py-8">
            <p className="text-sm font-light text-muted-foreground">{children}</p>
        </main>
    );
}

function LoadWarning({
    failed,
    children,
}: {
    failed: boolean;
    children: React.ReactNode;
}) {
    if (!failed) return null;
    return (
        <p className="mt-4 text-label text-muted-foreground" role="status">
            {children}
        </p>
    );
}

function FeedSection({
    sources,
    nav,
    onPatch,
    onDelete,
    tagSuggestions,
}: {
    sources: Source[];
    nav: ListNavigation;
    onPatch: (rkey: string, patch: SourcePatch) => Promise<boolean>;
    onDelete: (rkey: string) => Promise<boolean>;
    tagSuggestions: string[];
}) {
    if (sources.length === 0) return null;
    return (
        <SourceSection title="Feeds" count={sources.length}>
            <FeedList
                sources={sources}
                navOffset={0}
                active={nav.active}
                scrollKey={nav.scrollKey}
                onActivate={nav.setActive}
                onMouseLeave={nav.clearPointer}
                onPatch={onPatch}
                onDelete={onDelete}
                tagSuggestions={tagSuggestions}
            />
        </SourceSection>
    );
}

function OptionalNewsletterSection({
    title,
    sources,
    navOffset,
    nav,
    onPatch,
    tagSuggestions,
}: {
    title: string;
    sources: NewsletterSource[];
    navOffset: number;
    nav: ListNavigation;
    onPatch: (id: string, patch: SourcePatch) => Promise<boolean>;
    tagSuggestions: string[];
}) {
    if (sources.length === 0) return null;
    return (
        <NewsletterSection
            title={title}
            sources={sources}
            navOffset={navOffset}
            active={nav.active}
            scrollKey={nav.scrollKey}
            onActivate={nav.setActive}
            onMouseLeave={nav.clearPointer}
            onPatch={onPatch}
            tagSuggestions={tagSuggestions}
        />
    );
}

function SourcesMasthead({ count }: { count: number }) {
    const noun = count === 1 ? 'source' : 'sources';

    return (
        <div className="flex flex-col gap-1">
            <p className="text-sm font-light text-muted-foreground">
                Your publication
            </p>
            <div className="flex items-baseline justify-between gap-4">
                <h1 className="text-title">Sources</h1>
                <p className="shrink-0 text-sm text-muted-foreground">
                    {count} {noun}
                </p>
            </div>
        </div>
    );
}

function SourceSection({
    title,
    count,
    children,
}: {
    title: string;
    count: number;
    children: React.ReactNode;
}) {
    return (
        <section className="overflow-hidden rounded-xl bg-card shadow-card">
            <div className="flex items-baseline justify-between gap-4 px-6 pt-6 pb-5">
                <h2 className="text-title">{title}</h2>
                <p className="text-label text-muted-foreground">{count}</p>
            </div>
            <div aria-hidden className="mx-6 border-t border-border" />
            {children}
        </section>
    );
}

type ListSectionProps = {
    navOffset: number;
    active: number | null;
    scrollKey: number;
    onActivate: (index: number) => void;
    onMouseLeave: () => void;
};

function FeedList({
    sources,
    navOffset,
    active,
    scrollKey,
    onActivate,
    onMouseLeave,
    onPatch,
    onDelete,
    tagSuggestions,
}: ListSectionProps & {
    sources: Source[];
    onPatch: (rkey: string, patch: SourcePatch) => Promise<boolean>;
    onDelete: (rkey: string) => Promise<boolean>;
    tagSuggestions: string[];
}) {
    const listRef = useRef<HTMLDivElement>(null);
    const localActive =
        active !== null && active >= navOffset && active < navOffset + sources.length
            ? active - navOffset
            : null;

    return (
        <div ref={listRef} className="relative" onMouseLeave={onMouseLeave}>
            <ListHighlight
                containerRef={listRef}
                active={localActive}
                scrollKey={scrollKey}
            />
            <ul className="relative z-10 flex flex-col">
                {sources.map((source, index) => (
                    <Fragment key={source.rkey}>
                        {index > 0 ? (
                            <li
                                aria-hidden
                                className="mx-6 border-t border-border"
                            />
                        ) : null}
                        <SourceRow
                            source={source}
                            index={navOffset + index}
                            onActivate={onActivate}
                            onPatch={onPatch}
                            onDelete={onDelete}
                            tagSuggestions={tagSuggestions}
                        />
                    </Fragment>
                ))}
            </ul>
        </div>
    );
}

function NewsletterSection({
    title,
    sources,
    navOffset,
    active,
    scrollKey,
    onActivate,
    onMouseLeave,
    onPatch,
    tagSuggestions,
}: ListSectionProps & {
    title: string;
    sources: NewsletterSource[];
    onPatch: (id: string, patch: SourcePatch) => Promise<boolean>;
    tagSuggestions: string[];
}) {
    const listRef = useRef<HTMLDivElement>(null);
    const localActive =
        active !== null && active >= navOffset && active < navOffset + sources.length
            ? active - navOffset
            : null;

    return (
        <SourceSection title={title} count={sources.length}>
            <div
                ref={listRef}
                className="relative"
                onMouseLeave={onMouseLeave}
            >
                <ListHighlight
                    containerRef={listRef}
                    active={localActive}
                    scrollKey={scrollKey}
                />
                <ul className="relative z-10 flex flex-col">
                    {sources.map((source, index) => (
                        <Fragment key={source.id}>
                            {index > 0 ? (
                                <li
                                    aria-hidden
                                    className="mx-6 border-t border-border"
                                />
                            ) : null}
                            <NewsletterRow
                                source={source}
                                index={navOffset + index}
                                onActivate={onActivate}
                                onPatch={onPatch}
                                tagSuggestions={tagSuggestions}
                            />
                        </Fragment>
                    ))}
                </ul>
            </div>
        </SourceSection>
    );
}

function NewsletterRow({
    source,
    index,
    onActivate,
    onPatch,
    tagSuggestions,
}: {
    source: NewsletterSource;
    index: number;
    onActivate: (index: number) => void;
    onPatch: (id: string, patch: SourcePatch) => Promise<boolean>;
    tagSuggestions: string[];
}) {
    const [editing, setEditing] = useState(false);

    const onStop = async () => {
        try {
            await stopNewsletter(source.id);
            emitNewsletterMutation();
            return true;
        } catch (error) {
            toastMutationError(error, "Couldn't stop this newsletter. Try again.");
            return false;
        }
    };

    const onEnable = async () => {
        try {
            await enableNewsletter(source.id);
            emitNewsletterMutation();
        } catch (error) {
            toastMutationError(error, "Couldn't re-enable this newsletter. Try again.");
        }
    };

    return (
        <>
            <li
                data-nav-row=""
                onMouseEnter={() => onActivate(index)}
                className="relative flex items-start justify-between gap-3 px-5 py-4 transition-colors duration-200 ease-out has-[a:focus-visible]:outline-1 has-[a:focus-visible]:-outline-offset-2 has-[a:focus-visible]:outline-solid has-[a:focus-visible]:outline-ring"
            >
                <Link
                    href={newsletterSourceHref(source.id)}
                    aria-label={source.title}
                    className="absolute inset-0 outline-none"
                />
                <div className="pointer-events-none min-w-0 flex-1">
                    <div className="flex items-center gap-1.5 text-caption text-muted-foreground">
                        <MailIcon className="size-3.5 shrink-0" />
                        <span className="truncate">
                            {source.senderName || source.senderAddress}
                        </span>
                    </div>
                    <h3 className="mt-0.5 truncate text-heading">
                        {source.title}
                    </h3>
                    <div className="mt-2 flex items-center gap-3 text-caption text-muted-foreground">
                        <span className="inline-flex items-center gap-1">
                            <PulseIcon className="size-3.5" />
                            {FREQUENCY_LABEL[source.frequency]}
                        </span>
                        {source.lastReceivedAt ? (
                            <span className="inline-flex items-center gap-1">
                                <HourglassIcon className="size-3.5" />
                                {shortTimeAgo(source.lastReceivedAt)}
                            </span>
                        ) : null}
                    </div>
                </div>
                <div className="relative z-10 flex shrink-0 items-center gap-1">
                    <NewsletterSourceActions
                        status={source.status}
                        onEdit={() => setEditing(true)}
                        onStop={onStop}
                        onEnable={onEnable}
                    />
                </div>
            </li>
            <EditSourceDialog
                open={editing}
                onOpenChange={setEditing}
                initialTitle={source.title}
                initialPrimary={source.primary}
                initialTags={source.tags}
                tagSuggestions={tagSuggestions}
                onSave={(patch) => onPatch(source.id, patch)}
            />
        </>
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
