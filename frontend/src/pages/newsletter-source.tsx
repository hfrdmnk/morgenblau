import {
    ArrowLeftIcon,
    HourglassIcon,
    MailIcon,
    PulseIcon,
} from '@proicons/react';
import { useCallback, useEffect, useMemo, useState } from 'react';
import { useParams } from 'wouter';

import { Newspaper, type Entry } from '@/components/digest-rows';
import { NewsletterSourceActions } from '@/components/newsletters/source-actions';
import {
    EditSourceDialog,
    type SourcePatch,
} from '@/components/sources/edit-dialog';
import { Skeleton } from '@/components/ui/skeleton';
import { useDocumentTitle } from '@/hooks/use-document-title';
import { useEntryNavigation } from '@/hooks/use-entry-navigation';
import { useGoBackOr } from '@/hooks/use-go-back-or';
import { api } from '@/lib/api';
import { shortTimeAgo } from '@/lib/date';
import { toastMutationError } from '@/lib/mutation-toast';
import {
    emitNewsletterMutation,
    enableNewsletter,
    patchNewsletter,
    stopNewsletter,
    type NewsletterSource,
} from '@/lib/newsletters';
import { PATHS } from '@/lib/paths';
import { isPlainLeftClick } from '@/lib/utils';

type State =
    | { kind: 'loading' }
    | { kind: 'ok'; detail: NewsletterSource; entries: Entry[] }
    | { kind: 'error' };

const FREQUENCY_LABEL = {
    new: 'New',
    daily: 'Daily',
    weekly: 'Weekly',
    biweekly: 'Biweekly',
    monthly: 'Monthly',
    irregular: 'Irregular',
    noPosts: 'No issues',
} as const;

export function NewsletterSourcePage() {
    const { id } = useParams<{ id: string }>();
    const { state, onDetailChange, reload } = useNewsletterSource(id);

    useDocumentTitle(state.kind === 'ok' ? state.detail.title : 'Newsletter');

    if (state.kind === 'loading') return <NewsletterSourceSkeleton />;
    if (state.kind === 'error') return <NewsletterSourceError />;

    return (
        <NewsletterSourceView
            detail={state.detail}
            entries={state.entries}
            onDetailChange={onDetailChange}
            onReload={reload}
        />
    );
}

function useNewsletterSource(id: string | undefined) {
    const [state, setState] = useState<State>(
        id ? { kind: 'loading' } : { kind: 'error' },
    );
    const [reloadTick, setReloadTick] = useState(0);

    useEffect(() => {
        if (!id) return;
        const abort = new AbortController();
        Promise.all([
            api<NewsletterSource>(`/api/newsletters/${encodeURIComponent(id)}`, {
                signal: abort.signal,
            }),
            api<Entry[]>(
                `/api/newsletters/${encodeURIComponent(id)}/entries`,
                { signal: abort.signal },
            ),
        ])
            .then(([detail, entries]) =>
                setState({ kind: 'ok', detail, entries }),
            )
            .catch((error) => {
                if ((error as Error).name !== 'AbortError') {
                    setState({ kind: 'error' });
                }
            });
        return () => abort.abort();
    }, [id, reloadTick]);

    const onDetailChange = useCallback((detail: NewsletterSource) => {
        setState((current) => replaceNewsletterDetail(current, detail));
    }, []);
    const reload = useCallback(() => {
        setReloadTick((tick) => tick + 1);
    }, []);

    return { state, onDetailChange, reload };
}

function replaceNewsletterDetail(
    state: State,
    detail: NewsletterSource,
): State {
    if (state.kind !== 'ok') return state;
    return { ...state, detail };
}

function NewsletterSourceError() {
    return (
        <main className="mx-auto w-full max-w-2xl px-4 pt-16 pb-12 sm:px-6">
            <p className="text-label text-muted-foreground">
                Couldn't load this newsletter.
            </p>
        </main>
    );
}

function NewsletterSourceView({
    detail,
    entries,
    onDetailChange,
    onReload,
}: {
    detail: NewsletterSource;
    entries: Entry[];
    onDetailChange: (detail: NewsletterSource) => void;
    onReload: () => void;
}) {
    const [editing, setEditing] = useState(false);
    const entryFrom = useMemo(
        () => ({ newsletterSourceId: detail.id }),
        [detail.id],
    );
    const nav = useEntryNavigation(entries, entryFrom, { r: onReload });
    const actions = useNewsletterSourceActions(
        detail,
        onDetailChange,
        onReload,
    );

    return (
        <main className="mx-auto w-full max-w-2xl px-4 pt-16 pb-12 sm:px-6">
            <header className="mb-10 flex flex-col gap-4">
                <div className="relative flex items-center gap-3 font-sans">
                    <BackButton />
                    <span className="grid size-10 shrink-0 place-items-center rounded-lg bg-muted text-muted-foreground">
                        <MailIcon className="size-5" />
                    </span>
                    <div className="min-w-0 flex-1">
                        <p className="truncate text-caption text-muted-foreground">
                            {detail.senderName || detail.senderAddress}
                        </p>
                        <h1 className="truncate text-display">{detail.title}</h1>
                    </div>
                    <div className="flex shrink-0 items-center gap-1">
                        <NewsletterSourceActions
                            status={detail.status}
                            onEdit={() => setEditing(true)}
                            onStop={actions.onStop}
                            onEnable={actions.onEnable}
                        />
                    </div>
                </div>
                <Stats source={detail} />
                <StoppedNotice status={detail.status} />
            </header>

            <NewsletterIssues
                entries={entries}
                source={detail}
                entryFrom={entryFrom}
                nav={nav}
            />

            <EditSourceDialog
                open={editing}
                onOpenChange={setEditing}
                initialTitle={detail.title}
                initialPrimary={detail.primary}
                initialTags={detail.tags}
                tagSuggestions={detail.tags}
                onSave={actions.onPatch}
            />
        </main>
    );
}

function useNewsletterSourceActions(
    detail: NewsletterSource,
    onDetailChange: (detail: NewsletterSource) => void,
    onReload: () => void,
) {
    const onPatch = async (patch: SourcePatch) => {
        try {
            const next = await patchNewsletter(detail.id, {
                title: patch.title,
                primary: patch.primary,
                tags: patch.tags,
            });
            onDetailChange(next);
            emitNewsletterMutation();
            return true;
        } catch (error) {
            toastMutationError(error, "Couldn't save your changes. Try again.");
            return false;
        }
    };

    const onStop = async () => {
        try {
            const next = await stopNewsletter(detail.id);
            onDetailChange(next);
            emitNewsletterMutation();
            onReload();
            return true;
        } catch (error) {
            toastMutationError(error, "Couldn't stop this newsletter. Try again.");
            return false;
        }
    };

    const onEnable = async () => {
        try {
            const next = await enableNewsletter(detail.id);
            onDetailChange(next);
            emitNewsletterMutation();
        } catch (error) {
            toastMutationError(error, "Couldn't re-enable this newsletter. Try again.");
        }
    };

    return { onPatch, onStop, onEnable };
}

function StoppedNotice({ status }: { status: NewsletterSource['status'] }) {
    if (status !== 'stopped') return null;
    return (
        <p className="text-label text-muted-foreground">
            Stopped. Future deliveries are discarded until you re-enable this
            newsletter.
        </p>
    );
}

function NewsletterIssues({
    entries,
    source,
    entryFrom,
    nav,
}: {
    entries: Entry[];
    source: NewsletterSource;
    entryFrom: { newsletterSourceId: string };
    nav: ReturnType<typeof useEntryNavigation>;
}) {
    if (entries.length > 0) {
        return <Newspaper entries={entries} entryFrom={entryFrom} nav={nav} />;
    }
    return (
        <p className="text-label text-muted-foreground">
            {source.status === 'stopped'
                ? 'No saved issues remain.'
                : 'No issues yet.'}
        </p>
    );
}

function Stats({ source }: { source: NewsletterSource }) {
    return (
        <dl className="grid grid-cols-4 gap-3 font-sans text-sm">
            <Stat label="Frequency">
                <span className="inline-flex items-center gap-1.5">
                    <PulseIcon className="size-3.5 text-muted-foreground" />
                    {FREQUENCY_LABEL[source.frequency]}
                </span>
            </Stat>
            <Stat label="Last issue">
                {source.lastReceivedAt ? (
                    <span className="inline-flex items-center gap-1.5">
                        <HourglassIcon className="size-3.5 text-muted-foreground" />
                        {shortTimeAgo(source.lastReceivedAt)}
                    </span>
                ) : (
                    <span className="text-muted-foreground">—</span>
                )}
            </Stat>
            <Stat label="Issues">{source.issueCount}</Stat>
            <Stat label="Saved by you">{source.savedByYou}</Stat>
        </dl>
    );
}

function Stat({ label, children }: { label: string; children: React.ReactNode }) {
    return (
        <div className="flex flex-col gap-0.5">
            <dt className="text-overline text-muted-foreground">{label}</dt>
            <dd className="truncate font-medium tracking-tight">{children}</dd>
        </div>
    );
}

function BackButton() {
    const goBackOr = useGoBackOr();
    const onClick = (event: React.MouseEvent<HTMLAnchorElement>) => {
        if (!isPlainLeftClick(event)) return;
        event.preventDefault();
        goBackOr(PATHS.sources);
    };

    return (
        <a
            href={PATHS.sources}
            aria-label="Back to sources"
            onClick={onClick}
            className="absolute top-1/2 right-full mr-2 inline-flex size-9 -translate-y-1/2 items-center justify-center rounded-xl text-muted-foreground outline-none transition-colors duration-200 ease-out hover:text-foreground focus-visible:outline-1 focus-visible:outline-offset-2 focus-visible:outline-ring focus-visible:outline-solid"
        >
            <ArrowLeftIcon className="size-5" />
        </a>
    );
}

function NewsletterSourceSkeleton() {
    return (
        <main
            aria-busy
            aria-label="Loading newsletter"
            className="mx-auto w-full max-w-2xl px-4 pt-16 pb-12 sm:px-6"
        >
            <div className="mb-10 flex items-center gap-3">
                <Skeleton className="size-10 rounded-lg" />
                <div className="flex flex-1 flex-col gap-2">
                    <Skeleton className="h-3 w-36" />
                    <Skeleton className="h-6 w-2/3" />
                </div>
            </div>
            <Skeleton className="h-64 w-full rounded-xl" />
        </main>
    );
}
