import { useEffect, useState } from 'react';

import { Button } from '@/components/ui/button';
import { useDocumentTitle } from '@/hooks/use-document-title';
import { entryHref } from '@/lib/paths';
import { safeHref } from '@/lib/utils';

type Share = {
    rkey: string;
    kind: 'rss' | 'standardfeed';
    itemUrl?: string;
    document?: string;
    comment?: string;
    createdAt: string;
    title?: string;
    entrySlug?: string;
};

type State =
    | { kind: 'loading' }
    | { kind: 'ok'; shares: Share[] }
    | { kind: 'error' };

// Library — for now, the user's own shares. Saves + network-shared land later.
export function Library() {
    useDocumentTitle('Library');
    const [state, setState] = useState<State>({ kind: 'loading' });

    useEffect(() => {
        let cancelled = false;
        const load = async () => {
            try {
                const r = await fetch('/api/shares', {
                    credentials: 'same-origin',
                });
                if (!r.ok) throw new Error(String(r.status));
                const shares = ((await r.json()) as Share[] | null) ?? [];
                if (!cancelled) setState({ kind: 'ok', shares });
            } catch {
                if (!cancelled) setState({ kind: 'error' });
            }
        };
        load();
        return () => {
            cancelled = true;
        };
    }, []);

    const unshare = (rkey: string) => {
        setState((prev) => {
            if (prev.kind !== 'ok') return prev;
            return {
                kind: 'ok',
                shares: prev.shares.filter((s) => s.rkey !== rkey),
            };
        });
        fetch(`/api/shares/${encodeURIComponent(rkey)}`, {
            method: 'DELETE',
            credentials: 'same-origin',
        }).catch(() => {
            // Best-effort optimistic removal; a later sync reconciles on failure.
        });
    };

    return (
        <main className="mx-auto w-full max-w-2xl px-4 py-10 sm:px-6">
            <header className="mb-6">
                <h1>My shares</h1>
                <p className="mt-1 text-sm font-light text-muted-foreground">
                    Everything you've shared to your network and the Atmosphere.
                </p>
            </header>

            {state.kind === 'loading' ? (
                <p className="text-sm font-light text-muted-foreground">
                    Loading…
                </p>
            ) : state.kind === 'error' ? (
                <p className="text-sm font-light text-muted-foreground">
                    Couldn't load your shares.
                </p>
            ) : state.shares.length === 0 ? (
                <p className="text-sm font-light text-muted-foreground">
                    Nothing shared yet. Share an article from the reader to see
                    it here.
                </p>
            ) : (
                <ul className="divide-y divide-border overflow-hidden rounded-xl border border-border bg-card">
                    {state.shares.map((s) => (
                        <ShareRow
                            key={s.rkey}
                            share={s}
                            onUnshare={() => unshare(s.rkey)}
                        />
                    ))}
                </ul>
            )}
        </main>
    );
}

function ShareRow({
    share,
    onUnshare,
}: {
    share: Share;
    onUnshare: () => void;
}) {
    const label = share.title ?? share.itemUrl ?? share.document ?? 'Untitled';

    return (
        <li className="flex items-start gap-3 px-4 py-3">
            <div className="min-w-0 flex-1">
                <ShareTitle share={share} label={label} />
                {share.comment ? (
                    <p className="mt-1 line-clamp-2 text-sm font-light text-muted-foreground">
                        {share.comment}
                    </p>
                ) : null}
                <p className="mt-1 text-xs font-light text-muted-foreground">
                    {formatDate(share.createdAt)}
                </p>
            </div>
            <Button
                variant="ghost"
                size="sm"
                onClick={onUnshare}
                aria-label="Unshare"
            >
                Unshare
            </Button>
        </li>
    );
}

function ShareTitle({ share, label }: { share: Share; label: string }) {
    const className =
        'line-clamp-1 text-sm text-foreground transition-colors duration-200 ease-out outline-none hover:text-primary focus-visible:outline-1 focus-visible:outline-offset-2 focus-visible:outline-ring focus-visible:outline-solid';

    // Alive entry → the in-app reader; otherwise the original link if we have one.
    if (share.entrySlug) {
        return (
            <a href={entryHref(share.entrySlug)} className={className}>
                {label}
            </a>
        );
    }
    const external = safeHref(share.itemUrl ?? null);
    if (external) {
        return (
            <a
                href={external}
                target="_blank"
                rel="noopener noreferrer"
                className={className}
            >
                {label}
            </a>
        );
    }
    return <p className="line-clamp-1 text-sm text-foreground">{label}</p>;
}

function formatDate(iso: string): string {
    try {
        return new Date(iso).toLocaleDateString(undefined, {
            year: 'numeric',
            month: 'short',
            day: 'numeric',
        });
    } catch {
        return iso;
    }
}
