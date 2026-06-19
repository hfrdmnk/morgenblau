import {
    Delete02Icon,
    Edit02Icon,
    HelpCircleIcon,
    HourglassIcon,
    Pulse01Icon,
} from '@hugeicons/core-free-icons';
import { HugeiconsIcon } from '@hugeicons/react';
import {
    Fragment,
    useCallback,
    useEffect,
    useMemo,
    useRef,
    useState,
} from 'react';

import { Favicon } from '@/components/favicon';
import {
    EditSourceDialog,
    type SourcePatch,
} from '@/components/sources/edit-dialog';
import { sourceHref } from '@/lib/paths';
import { Button } from '@/components/ui/button';
import { Separator } from '@/components/ui/separator';
import { shortTimeAgo } from '@/lib/date';
import { useDocumentTitle } from '@/hooks/use-document-title';
import {
    subscribeSubscriptionAdded,
    type AddedSubscription,
} from '@/lib/subscription-events';
import { useJobsPoll } from '@/hooks/use-jobs-poll';
import { cn } from '@/lib/utils';

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
    feedUrl: string;
    title?: string;
    siteUrl?: string;
    faviconUrl?: string;
    frequency?: Frequency;
    lastPublishedAt?: string;
    primary?: boolean;
    tags?: string[];
    value: {
        title?: string;
        feedUrl?: string;
        siteUrl?: string;
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

export function Sources() {
    useDocumentTitle('Sources');
    const [state, setState] = useState<State>({ kind: 'loading' });
    const [reloadTick, setReloadTick] = useState(0);
    const [hasPendingJobs, setHasPendingJobs] = useState(false);

    useEffect(() => {
        let cancelled = false;
        fetch('/api/subscriptions', { credentials: 'same-origin' })
            .then((r) => {
                if (!r.ok) throw new Error(String(r.status));
                return r.json() as Promise<Source[]>;
            })
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

    // Tag suggestions for the edit dialog: the distinct set already in use
    // across the user's sources, deduped case-insensitively and sorted.
    const tagSuggestions = useMemo(() => {
        if (state.kind !== 'ok') return [];
        const byLower = new Map<string, string>();
        for (const record of state.records) {
            for (const tag of record.tags ?? []) {
                const key = tag.toLowerCase();
                if (!byLower.has(key)) byLower.set(key, tag);
            }
        }
        return [...byLower.values()].sort((a, b) => a.localeCompare(b));
    }, [state]);

    const onPatch = async (rkey: string, patch: SourcePatch) => {
        const resp = await fetch(`/api/subscriptions/${rkey}`, {
            method: 'PATCH',
            headers: { 'content-type': 'application/json' },
            credentials: 'same-origin',
            body: JSON.stringify(patch),
        });
        if (!resp.ok) return false;
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
                            value: {
                                ...r.value,
                                title: patch.title,
                                primary: patch.primary,
                                tags: patch.tags,
                            },
                        }
                        : r,
                ),
            };
        });
        return true;
    };

    const onDelete = async (rkey: string) => {
        const resp = await fetch(`/api/subscriptions/${rkey}`, {
            method: 'DELETE',
            credentials: 'same-origin',
        });
        if (!resp.ok) return false;
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

    const sortedRecords = [...state.records].sort((a, b) =>
        displayLabel(a).localeCompare(displayLabel(b)),
    );

    return (
        <main className="mx-auto max-w-2xl px-6 py-8">
            <div className="overflow-hidden rounded-xl bg-card">
                <SourcesMasthead count={sortedRecords.length} />
                <div
                    aria-hidden
                    className="mx-6 border-t border-gray-100 dark:border-gray-700"
                />
                <ul className="flex flex-col">
                    {sortedRecords.map((r, i) => (
                        <Fragment key={r.rkey}>
                            {i > 0 ? (
                                <li
                                    aria-hidden
                                    className="mx-6 border-t border-gray-100 dark:border-gray-700"
                                />
                            ) : null}
                            <SourceRow
                                source={r}
                                onPatch={onPatch}
                                onDelete={onDelete}
                                tagSuggestions={tagSuggestions}
                            />
                        </Fragment>
                    ))}
                </ul>
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
        feedUrl: added.feedUrl,
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
    const candidate =
        s.siteUrl ||
        (typeof s.value.siteUrl === 'string' ? s.value.siteUrl : '') ||
        s.feedUrl;
    if (!candidate) return '';
    try {
        return new URL(candidate).hostname.replace(/^www\./, '');
    } catch {
        return candidate;
    }
}

type RowProps = {
    source: Source;
    onPatch: (rkey: string, patch: SourcePatch) => Promise<boolean>;
    onDelete: (rkey: string) => Promise<boolean>;
    tagSuggestions: string[];
};

function SourceRow({ source, onPatch, onDelete, tagSuggestions }: RowProps) {
    const [editing, setEditing] = useState(false);
    const title = displayLabel(source);
    const domain = siteDomain(source);
    const frequency = source.frequency ?? 'noPosts';

    return (
        <>
            <li className="relative flex items-start justify-between gap-3 px-5 py-4 transition-colors duration-200 ease-out hover:bg-gray-50 has-[a:focus-visible]:outline-1 has-[a:focus-visible]:-outline-offset-2 has-[a:focus-visible]:outline-solid has-[a:focus-visible]:outline-ring dark:hover:bg-gray-900">
                <a
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
                            <HugeiconsIcon
                                icon={Pulse01Icon}
                                className="size-3.5"
                            />
                            {FREQUENCY_LABEL[frequency]}
                        </span>
                        {frequency !== 'noPosts' && source.lastPublishedAt ? (
                            <span className="inline-flex items-center gap-1">
                                <HugeiconsIcon
                                    icon={HourglassIcon}
                                    className="size-3.5"
                                />
                                {shortTimeAgo(source.lastPublishedAt)}
                            </span>
                        ) : null}
                    </div>
                </div>
                <div className="relative z-10 flex shrink-0 items-center gap-1">
                    <Button
                        variant="ghost"
                        size="icon-sm"
                        aria-label="Edit source"
                        onClick={() => setEditing(true)}
                    >
                        <HugeiconsIcon
                            icon={Edit02Icon}
                            className="size-3.5"
                        />
                    </Button>
                    <Separator orientation="vertical" className="h-5" />
                    <RowDeleteButton
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
                tagSuggestions={tagSuggestions}
                onSave={(patch) => onPatch(source.rkey, patch)}
            />
        </>
    );
}

// Two-stage icon-morph: first click arms (Delete → red HelpCircle, scale-down
// + blur switch), second click within the window confirms. Auto-cancels after
// 3 s; click-outside or Escape also cancel.
function RowDeleteButton({ onConfirm }: { onConfirm: () => Promise<boolean> }) {
    const [armed, setArmed] = useState(false);
    const buttonRef = useRef<HTMLButtonElement | null>(null);
    const timerRef = useRef<number | null>(null);

    const cancel = useCallback(() => {
        setArmed(false);
        if (timerRef.current !== null) {
            window.clearTimeout(timerRef.current);
            timerRef.current = null;
        }
    }, []);

    useEffect(() => {
        if (!armed) return;
        const onDocClick = (e: MouseEvent) => {
            if (
                buttonRef.current &&
                !buttonRef.current.contains(e.target as Node)
            ) {
                cancel();
            }
        };
        const onKey = (e: KeyboardEvent) => {
            if (e.key === 'Escape') cancel();
        };
        timerRef.current = window.setTimeout(cancel, 3000);
        document.addEventListener('mousedown', onDocClick);
        document.addEventListener('keydown', onKey);
        return () => {
            document.removeEventListener('mousedown', onDocClick);
            document.removeEventListener('keydown', onKey);
            if (timerRef.current !== null) {
                window.clearTimeout(timerRef.current);
                timerRef.current = null;
            }
        };
    }, [armed, cancel]);

    const onClick = () => {
        if (armed) {
            cancel();
            onConfirm();
        } else {
            setArmed(true);
        }
    };

    return (
        <Button
            ref={buttonRef}
            variant="ghost"
            size="icon-sm"
            aria-label={armed ? 'Confirm remove' : 'Remove source'}
            aria-pressed={armed}
            onClick={onClick}
            className={armed ? 'text-destructive hover:text-destructive' : ''}
        >
            <span className="relative grid size-3.5 place-items-center">
                <HugeiconsIcon
                    icon={Delete02Icon}
                    className={cn(
                        'absolute size-3.5 transition-all duration-200 ease-in-out',
                        armed
                            ? 'scale-50 opacity-0 blur-[3px]'
                            : 'scale-100 opacity-100 blur-0',
                    )}
                />
                <HugeiconsIcon
                    icon={HelpCircleIcon}
                    className={cn(
                        'absolute size-3.5 transition-all duration-200 ease-in-out',
                        armed
                            ? 'scale-100 opacity-100 blur-0'
                            : 'scale-50 opacity-0 blur-[3px]',
                    )}
                />
            </span>
        </Button>
    );
}
