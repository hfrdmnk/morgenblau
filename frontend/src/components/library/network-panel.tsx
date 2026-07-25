import { PersonMultipleIcon, SpinnerIcon } from '@proicons/react';
import { Fragment, useEffect, useState } from 'react';
import { Link } from 'wouter';

import { ListPanelShell, SectionState } from '@/components/library/library-panel-shell';
import {
    RowDivider,
    RowOverlayLink,
    ROW_CLASS,
    ShareComment,
} from '@/components/library/share-row';
import { Avatar, AvatarFallback, AvatarImage } from '@/components/ui/avatar';
import { formatDate } from '@/lib/date';
import { initialsFromHandle, truncateDid } from '@/lib/handle';
import {
    fetchNetworkShares,
    type NetworkShareWithProfile,
} from '@/lib/library';
import { personHref } from '@/lib/paths';
import { shareTargetPresentation } from '@/lib/share-target';
import { safeHref } from '@/lib/utils';

type NetworkPerson = NetworkShareWithProfile;

type State =
    | { kind: 'loading' }
    | { kind: 'ok'; shares: NetworkPerson[] }
    | { kind: 'error' };

// Stable empty list so list navigation doesn't reset every render while loading.
const EMPTY_SHARES: NetworkPerson[] = [];

// NetworkPanel: the Library "Network" tab — shares from people the reader follows. SPEC <social-layer> Follow Contract.
export function NetworkPanel() {
    const [state, setState] = useState<State>({ kind: 'loading' });

    useEffect(() => {
        let cancelled = false;
        const load = async () => {
            try {
                const shares = await fetchNetworkShares();
                if (cancelled) return;
                setState({ kind: 'ok', shares });
            } catch {
                if (!cancelled) setState({ kind: 'error' });
            }
        };
        load();
        return () => {
            cancelled = true;
        };
    }, []);

    const items = state.kind === 'ok' ? state.shares : EMPTY_SHARES;

    return (
        <ListPanelShell eyebrow="Library" heading="From your network" items={items}>
            {(nav) => (
                <NetworkShares state={state} onActivate={nav.setActive} />
            )}
        </ListPanelShell>
    );
}

function NetworkShares({
    state,
    onActivate,
}: {
    state: State;
    onActivate: (index: number) => void;
}) {
    if (state.kind === 'loading') {
        return <SectionState icon={SpinnerIcon} spin lead="Loading…" />;
    }
    if (state.kind === 'error') {
        return (
            <SectionState
                lead="Couldn't load shares from your network."
                detail="Try again in a moment."
            />
        );
    }
    if (state.shares.length === 0) {
        return (
            <SectionState
                icon={PersonMultipleIcon}
                lead="Nothing here yet."
                detail="Follow someone to see what they share."
            />
        );
    }

    return (
        <ul className="flex flex-col">
            {state.shares.map((share, index) => (
                <Fragment
                    key={`${share.sharerDid}:${share.document ?? share.itemUrl ?? share.createdAt}`}
                >
                    {index > 0 ? <RowDivider /> : null}
                    <NetworkShareRow
                        share={share}
                        index={index}
                        onActivate={onActivate}
                    />
                </Fragment>
            ))}
        </ul>
    );
}

function NetworkShareRow({
    share,
    index,
    onActivate,
}: {
    share: NetworkPerson;
    index: number;
    onActivate: (index: number) => void;
}) {
    const target = shareTargetPresentation(share);
    const handle = share.profile?.handle;
    const sharerLabel = networkSharerLabel(share.sharerDid, handle);
    const sharerHref = personHref(share.sharerDid, handle);

    return (
        <li
            data-nav-row=""
            onMouseEnter={() => onActivate(index)}
            className={ROW_CLASS}
        >
            <RowOverlayLink target={target} />
            <Link
                href={sharerHref}
                aria-label={sharerLabel}
                className="relative z-10 mt-0.5 shrink-0 rounded-full outline-none focus-visible:outline-solid focus-visible:outline-1 focus-visible:outline-offset-2 focus-visible:outline-ring"
            >
                <NetworkShareAvatar share={share} handle={handle} />
            </Link>
            <div className="min-w-0 flex-1">
                <Link
                    href={sharerHref}
                    className="relative z-10 block w-full truncate rounded-sm text-xs font-light text-muted-foreground outline-none hover:underline focus-visible:outline-solid focus-visible:outline-1 focus-visible:outline-offset-2 focus-visible:outline-ring"
                >
                    {sharerLabel}
                </Link>
                <div className="pointer-events-none">
                    <h3 className="line-clamp-1 text-heading text-foreground">
                        {target.label}
                    </h3>
                    <ShareComment comment={share.comment} />
                    <p className="mt-1 text-caption text-muted-foreground">
                        {formatDate(share.createdAt)}
                    </p>
                </div>
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
