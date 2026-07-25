import { SendIcon, SpinnerIcon } from '@proicons/react';
import { Fragment, useEffect, useState } from 'react';

import { InputError } from '@/components/input-error';
import {
    ListPanelShell,
    SectionState,
} from '@/components/library/library-panel-shell';
import {
    RowDivider,
    RowOverlayLink,
    ROW_CLASS,
    ShareComment,
} from '@/components/library/share-row';
import { Button } from '@/components/ui/button';
import {
    classifyShareError,
    type ShareError,
} from '@/hooks/use-share-toggle';
import { api } from '@/lib/api';
import { formatDate } from '@/lib/date';
import { fetchShares, type Share } from '@/lib/library';
import { PATHS } from '@/lib/paths';
import { shareTargetPresentation } from '@/lib/share-target';
import { cn } from '@/lib/utils';

type State =
    | { kind: 'loading' }
    | { kind: 'ok'; shares: Share[] }
    | { kind: 'error' };

// Stable empty list so list navigation doesn't reset every render while loading.
const EMPTY_SHARES: Share[] = [];

// SharedPanel: the Library "Shared" tab — everything this user has sent to their network and the Atmosphere.
export function SharedPanel() {
    const [state, setState] = useState<State>({ kind: 'loading' });
    const [error, setError] = useState<ShareError | null>(null);

    useEffect(() => {
        let cancelled = false;
        const load = async () => {
            try {
                const shares = await fetchShares();
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

    const items = state.kind === 'ok' ? state.shares : EMPTY_SHARES;

    return (
        <ListPanelShell eyebrow="Library" heading="Shared by you" items={items}>
            {(nav) => (
                <>
                    <LibraryMutationError error={error} />
                    <OwnShares
                        state={state}
                        onUnshare={unshare}
                        onActivate={nav.setActive}
                    />
                </>
            )}
        </ListPanelShell>
    );
}

function LibraryMutationError({ error }: { error: ShareError | null }) {
    if (error === 'reauth') {
        return (
            <p
                role="status"
                className="px-6 py-4 text-sm font-light text-muted-foreground"
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
                className="px-6 py-4"
                message="Couldn't unshare just now. Try again."
            />
        );
    }
    return null;
}

function OwnShares({
    state,
    onUnshare,
    onActivate,
}: {
    state: State;
    onUnshare: (rkey: string) => void;
    onActivate: (index: number) => void;
}) {
    if (state.kind === 'loading') {
        return <SectionState icon={SpinnerIcon} spin lead="Loading…" />;
    }
    if (state.kind === 'error') {
        return (
            <SectionState
                lead="Couldn't load your shares."
                detail="Try again in a moment."
            />
        );
    }
    if (state.shares.length === 0) {
        return (
            <SectionState
                icon={SendIcon}
                lead="Nothing shared yet."
                detail="Share an article from the reader to see it here."
            />
        );
    }

    return (
        <ul className="flex flex-col">
            {state.shares.map((share, index) => (
                <Fragment key={share.rkey}>
                    {index > 0 ? <RowDivider /> : null}
                    <ShareRow
                        share={share}
                        index={index}
                        onActivate={onActivate}
                        onUnshare={() => onUnshare(share.rkey)}
                    />
                </Fragment>
            ))}
        </ul>
    );
}

function ShareRow({
    share,
    index,
    onActivate,
    onUnshare,
}: {
    share: Share;
    index: number;
    onActivate: (index: number) => void;
    onUnshare: () => void;
}) {
    const target = shareTargetPresentation(share);

    return (
        <li
            data-nav-row=""
            onMouseEnter={() => onActivate(index)}
            className={cn(ROW_CLASS, 'justify-between')}
        >
            <RowOverlayLink target={target} />
            <div className="pointer-events-none min-w-0 flex-1">
                <h3 className="line-clamp-1 text-heading text-foreground">
                    {target.label}
                </h3>
                <ShareComment comment={share.comment} />
                <p className="mt-1 text-caption text-muted-foreground">
                    {formatDate(share.createdAt)}
                </p>
            </div>
            <div className="relative z-10 shrink-0">
                <Button
                    variant="ghost"
                    size="sm"
                    onClick={onUnshare}
                    aria-label="Unshare"
                >
                    Unshare
                </Button>
            </div>
        </li>
    );
}
