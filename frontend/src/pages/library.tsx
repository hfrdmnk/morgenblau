import { useEffect, useState } from 'react';
import { Link } from 'wouter';

import { InputError } from '@/components/input-error';
import { Avatar, AvatarFallback, AvatarImage } from '@/components/ui/avatar';
import { Button } from '@/components/ui/button';
import { useDocumentTitle } from '@/hooks/use-document-title';
import { classifyShareError, type ShareError } from '@/hooks/use-share-toggle';
import { api } from '@/lib/api';
import { initialsFromHandle, truncateDid } from '@/lib/handle';
import { uniqueSharerDIDs, type NetworkShare } from '@/lib/library';
import { PATHS, personHref } from '@/lib/paths';
import { fetchProfile, type Profile } from '@/lib/profile';
import {
    shareTargetPresentation,
    type ShareTargetPresentation,
} from '@/lib/share-target';
import { safeHref } from '@/lib/utils';

type Share = {
    rkey: string;
    kind: 'rss' | 'standardfeed';
    itemUrl?: string;
    document?: string;
    comment?: string;
    createdAt: string;
    title?: string;
    targetUrl?: string;
    entrySlug?: string;
};

type State =
    | { kind: 'loading' }
    | { kind: 'ok'; shares: Share[] }
    | { kind: 'error' };

type NetworkPerson = NetworkShare & { profile?: Profile };

type NetworkState =
    | { kind: 'loading' }
    | { kind: 'ok'; shares: NetworkPerson[] }
    | { kind: 'error' };

// Library: user's own shares plus a network section for people they follow. SPEC <social-layer> Follow Contract.
export function Library() {
    useDocumentTitle('Library');
    const [state, setState] = useState<State>({ kind: 'loading' });
    const [error, setError] = useState<ShareError | null>(null);
    const [networkState, setNetworkState] = useState<NetworkState>({
        kind: 'loading',
    });

    useEffect(() => {
        let cancelled = false;
        const load = async () => {
            try {
                const shares = (await api<Share[] | null>('/api/shares')) ?? [];
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

    useEffect(() => {
        let cancelled = false;
        const load = async () => {
            try {
                const shares =
                    (await api<NetworkShare[] | null>(
                        '/api/library/network-shares',
                    )) ?? [];
                const dids = uniqueSharerDIDs(shares);
                const profiles = await Promise.all(dids.map(fetchProfile));
                if (cancelled) return;
                const profileByDID = new Map(
                    dids.map((did, i) => [did, profiles[i]]),
                );
                setNetworkState({
                    kind: 'ok',
                    shares: shares.map((s) => ({
                        ...s,
                        profile: profileByDID.get(s.sharerDid),
                    })),
                });
            } catch {
                if (!cancelled) setNetworkState({ kind: 'error' });
            }
        };
        load();
        return () => {
            cancelled = true;
        };
    }, []);

    const unshare = async (rkey: string) => {
        if (state.kind !== 'ok') return;
        const index = state.shares.findIndex((s) => s.rkey === rkey);
        if (index === -1) return;
        const removed = state.shares[index];

        setError(null);
        setState((prev) =>
            prev.kind === 'ok'
                ? {
                      kind: 'ok',
                      shares: prev.shares.filter((s) => s.rkey !== rkey),
                  }
                : prev,
        );

        const reinsert = () =>
            setState((prev) => {
                if (prev.kind !== 'ok') return prev;
                const shares = prev.shares.slice();
                shares.splice(index, 0, removed);
                return { kind: 'ok', shares };
            });

        try {
            await api(`/api/shares/${encodeURIComponent(rkey)}`, {
                method: 'DELETE',
            });
        } catch (err) {
            reinsert();
            setError(classifyShareError(err));
        }
    };

    return (
        <main className="mx-auto w-full max-w-2xl px-4 py-10 sm:px-6">
            <header className="mb-6">
                <h1>My shares</h1>
                <p className="mt-1 text-sm font-light text-muted-foreground">
                    Everything you've shared to your network and the Atmosphere.
                </p>
            </header>

            <LibraryMutationError error={error} />
            <OwnShares state={state} onUnshare={unshare} />
            <NetworkShares state={networkState} />
        </main>
    );
}

function LibraryMutationError({ error }: { error: ShareError | null }) {
    if (error === 'reauth') {
        return (
            <p
                role="status"
                className="mb-4 text-sm font-light text-muted-foreground"
            >
                Your session is out of date.{' '}
                {/* Native anchor: reauth exits the authed shell, which app.tsx assumes is a full server round trip. */}
                <a
                    href={PATHS.login}
                    className="text-primary underline underline-offset-4"
                >
                    Sign in again
                </a>{' '}
                to manage your shares.
            </p>
        );
    }
    if (error === 'failed') {
        return (
            <InputError
                className="mb-4"
                message="Couldn't unshare just now. Try again."
            />
        );
    }
    return null;
}

function OwnShares({
    state,
    onUnshare,
}: {
    state: State;
    onUnshare: (rkey: string) => void;
}) {
    if (state.kind === 'loading') {
        return (
            <p className="text-sm font-light text-muted-foreground">Loading…</p>
        );
    }
    if (state.kind === 'error') {
        return (
            <p className="text-sm font-light text-muted-foreground">
                Couldn't load your shares.
            </p>
        );
    }
    if (state.shares.length === 0) {
        return (
            <p className="text-sm font-light text-muted-foreground">
                Nothing shared yet. Share an article from the reader to see it
                here.
            </p>
        );
    }

    return (
        <ul className="divide-y divide-border overflow-hidden rounded-xl bg-card shadow-card">
            {state.shares.map((share) => (
                <ShareRow
                    key={share.rkey}
                    share={share}
                    onUnshare={() => onUnshare(share.rkey)}
                />
            ))}
        </ul>
    );
}

function NetworkShares({ state }: { state: NetworkState }) {
    let content;
    if (state.kind === 'loading') {
        content = (
            <p className="mt-4 text-sm font-light text-muted-foreground">
                Loading…
            </p>
        );
    } else if (state.kind === 'error') {
        content = (
            <p className="mt-4 text-sm font-light text-muted-foreground">
                Couldn't load shares from people you follow.
            </p>
        );
    } else if (state.shares.length === 0) {
        content = (
            <p className="mt-4 text-sm font-light text-muted-foreground">
                Nothing here yet. Follow someone to see their shares.
            </p>
        );
    } else {
        content = (
            <ul className="mt-4 divide-y divide-border overflow-hidden rounded-xl bg-card shadow-card">
                {state.shares.map((share) => (
                    <NetworkShareRow
                        key={`${share.sharerDid}:${share.document ?? share.itemUrl ?? share.createdAt}`}
                        share={share}
                    />
                ))}
            </ul>
        );
    }

    return (
        <section className="mt-10">
            <h2 className="text-xl font-medium">From people you follow</h2>
            {content}
        </section>
    );
}

function ShareRow({
    share,
    onUnshare,
}: {
    share: Share;
    onUnshare: () => void;
}) {
    const target = shareTargetPresentation(share);

    return (
        <li className="flex items-start gap-3 px-4 py-3">
            <div className="min-w-0 flex-1">
                <ShareTitle target={target} />
                <ShareComment comment={share.comment} />
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

function ShareTitle({ target }: { target: ShareTargetPresentation }) {
    const className =
        'line-clamp-1 text-sm text-foreground transition-colors duration-200 ease-out outline-none hover:text-primary focus-visible:outline-1 focus-visible:outline-offset-2 focus-visible:outline-ring focus-visible:outline-solid';

    if (target.href && !target.external) {
        return (
            <Link href={target.href} className={className}>
                {target.label}
            </Link>
        );
    }
    if (target.href) {
        return (
            <a
                href={target.href}
                target="_blank"
                rel="noopener noreferrer"
                className={className}
            >
                {target.label}
            </a>
        );
    }
    return (
        <p className="line-clamp-1 text-sm text-foreground">{target.label}</p>
    );
}

function NetworkShareRow({ share }: { share: NetworkPerson }) {
    const target = shareTargetPresentation(share);
    const handle = share.profile?.handle;
    const sharerLabel = networkSharerLabel(share.sharerDid, handle);
    const sharerHref = personHref(share.sharerDid, handle);

    return (
        <li className="flex items-start gap-3 px-4 py-3">
            <Link
                href={sharerHref}
                aria-label={sharerLabel}
                className="mt-0.5 shrink-0 rounded-full outline-none focus-visible:outline-solid focus-visible:outline-1 focus-visible:outline-offset-2 focus-visible:outline-ring"
            >
                <NetworkShareAvatar share={share} handle={handle} />
            </Link>
            <div className="min-w-0 flex-1">
                <Link
                    href={sharerHref}
                    className="block w-full truncate rounded-sm text-xs font-light text-muted-foreground outline-none hover:underline focus-visible:outline-solid focus-visible:outline-1 focus-visible:outline-offset-2 focus-visible:outline-ring"
                >
                    {sharerLabel}
                </Link>
                <ShareTitle target={target} />
                <ShareComment comment={share.comment} />
                <p className="mt-1 text-xs font-light text-muted-foreground">
                    {formatDate(share.createdAt)}
                </p>
            </div>
        </li>
    );
}

function networkSharerLabel(did: string, handle: string | undefined): string {
    return handle ? `@${handle}` : truncateDid(did);
}

function NetworkShareAvatar({
    share,
    handle,
}: {
    share: NetworkPerson;
    handle: string | undefined;
}) {
    const avatar = share.profile?.avatar;
    return (
        <Avatar>
            {avatar ? <AvatarImage src={safeHref(avatar)} alt="" /> : null}
            <AvatarFallback>
                {initialsFromHandle(handle ?? null, share.sharerDid)}
            </AvatarFallback>
        </Avatar>
    );
}

function ShareComment({ comment }: { comment: string | undefined }) {
    if (!comment) return null;
    return (
        <p className="mt-1 line-clamp-2 text-sm font-light text-muted-foreground">
            {comment}
        </p>
    );
}

function formatDate(iso: string): string {
    const d = new Date(iso);
    return Number.isNaN(d.getTime())
        ? iso
        : d.toLocaleDateString(undefined, {
              year: 'numeric',
              month: 'short',
              day: 'numeric',
          });
}
