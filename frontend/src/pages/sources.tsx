import { Delete02Icon, Edit02Icon } from '@hugeicons/core-free-icons';
import { HugeiconsIcon } from '@hugeicons/react';
import { useCallback, useEffect, useState } from 'react';

import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { useDocumentTitle } from '@/hooks/use-document-title';
import {
    subscribeSubscriptionAdded,
    type AddedSubscription,
} from '@/lib/subscription-events';
import { useJobsPoll } from '@/hooks/use-jobs-poll';

type Subscription = {
    uri: string;
    cid?: string;
    rkey: string;
    feedUrl: string;
    title?: string;
    value: {
        title?: string;
        customTitle?: string;
        feedUrl?: string;
        siteUrl?: string;
        [k: string]: unknown;
    };
};

type State =
    | { kind: 'loading' }
    | { kind: 'ok'; records: Subscription[] }
    | { kind: 'error' };

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
                return r.json() as Promise<Subscription[]>;
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
                    byRkey.set(added.rkey, addedToSubscription(added));
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

    if (state.kind === 'loading') {
        return (
            <main className="p-8">
                <p className="text-muted-foreground">Loading…</p>
            </main>
        );
    }
    if (state.kind === 'error') {
        return (
            <main className="p-8">
                <p className="text-muted-foreground">
                    Could not load your sources.
                </p>
            </main>
        );
    }
    if (state.records.length === 0) {
        return (
            <main className="p-8">
                <p className="text-muted-foreground">
                    No sources yet — paste a URL to add one.
                </p>
            </main>
        );
    }

    const onPatch = async (rkey: string, title: string) => {
        const resp = await fetch(`/api/subscriptions/${rkey}`, {
            method: 'PATCH',
            headers: { 'content-type': 'application/json' },
            credentials: 'same-origin',
            body: JSON.stringify({ customTitle: title }),
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
                              value: { ...r.value, customTitle: title },
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

    const sortedRecords = [...state.records].sort((a, b) =>
        displayLabel(a).localeCompare(displayLabel(b)),
    );

    return (
        <main className="mx-auto max-w-2xl px-6 py-8">
            <ul className="flex flex-col gap-2">
                {sortedRecords.map((r) => (
                    <SourceRow
                        key={r.rkey}
                        sub={r}
                        onPatch={onPatch}
                        onDelete={onDelete}
                    />
                ))}
            </ul>
        </main>
    );
}

function addedToSubscription(added: AddedSubscription): Subscription {
    return {
        uri: added.uri,
        cid: added.cid,
        rkey: added.rkey,
        feedUrl: added.feedUrl,
        title: added.title,
        value: {
            ...added.value,
            feedUrl: added.feedUrl,
            title: added.title,
            siteUrl: added.siteUrl,
        },
    };
}

function displayLabel(s: Subscription): string {
    const custom =
        typeof s.value.customTitle === 'string' ? s.value.customTitle : '';
    return (
        custom ||
        (typeof s.value.title === 'string' ? s.value.title : '') ||
        s.feedUrl ||
        s.uri
    );
}

type RowProps = {
    sub: Subscription;
    onPatch: (rkey: string, title: string) => Promise<boolean>;
    onDelete: (rkey: string) => Promise<boolean>;
};

function SourceRow({ sub, onPatch, onDelete }: RowProps) {
    const [editing, setEditing] = useState(false);
    const [confirming, setConfirming] = useState(false);
    const initial = displayLabel(sub);
    const [draft, setDraft] = useState(initial);
    const [saving, setSaving] = useState(false);

    const onSave = async () => {
        if (saving) return;
        setSaving(true);
        const ok = await onPatch(sub.rkey, draft.trim());
        setSaving(false);
        if (ok) setEditing(false);
    };

    return (
        <li className="rounded-xl border border-border bg-card px-4 py-3">
            {editing ? (
                <div className="flex items-center gap-2">
                    <Input
                        autoFocus
                        value={draft}
                        onChange={(e) => setDraft(e.target.value)}
                    />
                    <Button
                        type="button"
                        variant="secondary"
                        onClick={() => {
                            setEditing(false);
                            setDraft(initial);
                        }}
                    >
                        Cancel
                    </Button>
                    <Button type="button" onClick={onSave} disabled={saving}>
                        Save
                    </Button>
                </div>
            ) : confirming ? (
                <div className="flex items-center justify-between gap-3">
                    <p className="text-sm text-muted-foreground">
                        Remove “{initial}”?
                    </p>
                    <div className="flex items-center gap-2">
                        <Button
                            type="button"
                            variant="secondary"
                            onClick={() => setConfirming(false)}
                        >
                            Keep
                        </Button>
                        <Button
                            type="button"
                            onClick={() => onDelete(sub.rkey)}
                        >
                            Remove
                        </Button>
                    </div>
                </div>
            ) : (
                <div className="flex items-center justify-between gap-3">
                    <div className="min-w-0 flex-1">
                        <p className="truncate text-sm font-medium">
                            {initial}
                        </p>
                        <p className="truncate text-xs font-light text-muted-foreground">
                            {sub.feedUrl}
                        </p>
                    </div>
                    <div className="flex items-center gap-1 text-muted-foreground">
                        <Button
                            variant="ghost"
                            size="icon-sm"
                            aria-label="Rename"
                            onClick={() => setEditing(true)}
                        >
                            <HugeiconsIcon icon={Edit02Icon} className="size-4" />
                        </Button>
                        <Button
                            variant="ghost"
                            size="icon-sm"
                            aria-label="Remove"
                            onClick={() => setConfirming(true)}
                        >
                            <HugeiconsIcon
                                icon={Delete02Icon}
                                className="size-4"
                            />
                        </Button>
                    </div>
                </div>
            )}
        </li>
    );
}
