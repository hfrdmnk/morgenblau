import { type ReactNode } from 'react';
import { Link } from 'wouter';

import { CutPop } from '@/components/discover/cut-pop';
import {
    ReasonAvatarLink,
    ReasonLeadIn,
    ReasonRow,
    ReasonText,
    ReasonTrending,
} from '@/components/discover/reason-primitives';
import {
    discoverFollowerLabel,
    formatDiscoverReason,
    type DiscoverReason,
    type DiscoverReasonDisplay,
} from '@/lib/discover';
import type { ContentState } from '@/lib/discover-cut';
import { personHref } from '@/lib/paths';
import type { Profile } from '@/lib/profile';
import { cn } from '@/lib/utils';

type Profiles = Record<string, Profile | undefined>;

// SPEC <discovery> Cards: renders whichever reason shape resolveDiscoverReasonDisplay picked.
export function DiscoverReasonLine({
    display,
    reason,
    profiles,
    expanded,
    contentState,
}: {
    display: DiscoverReasonDisplay;
    reason: DiscoverReason;
    profiles: Profiles;
    expanded: boolean;
    contentState: ContentState;
}): ReactNode {
    if (display.kind === 'text') {
        return <ReasonText text={display.text} />;
    }
    if (display.kind === 'author') {
        return (
            <ReasonAuthor
                did={display.did}
                profiles={profiles}
                expanded={expanded}
                contentState={contentState}
            />
        );
    }
    if (display.kind === 'people') {
        return (
            <ReasonPeople
                followerDids={display.followerDids}
                total={display.total}
                reason={reason}
                profiles={profiles}
                expanded={expanded}
                contentState={contentState}
            />
        );
    }
    return <ReasonTrending expanded={expanded} contentState={contentState} />;
}

function ReasonAuthor({
    did,
    profiles,
    expanded,
    contentState,
}: {
    did: string;
    profiles: Profiles;
    expanded: boolean;
    contentState: ContentState;
}) {
    const profile = profiles[did];
    return (
        <ReasonRow>
            {expanded ? (
                <ReasonLeadIn contentState={contentState}>
                    <CutPop contentState={contentState} order={0}>
                        <ReasonAvatarLink did={did} profile={profile} />
                    </CutPop>
                </ReasonLeadIn>
            ) : null}
            <span className="truncate text-sm font-light text-muted-foreground">
                Written by{' '}
                <Link
                    href={personHref(did, profile?.handle)}
                    className="rounded-sm outline-none hover:underline focus-visible:outline-solid focus-visible:outline-1 focus-visible:outline-offset-2 focus-visible:outline-ring"
                >
                    {discoverFollowerLabel(did, profile)}
                </Link>
            </span>
        </ReasonRow>
    );
}

function soleFollowerLabel(
    total: number,
    reason: DiscoverReason,
    profiles: Profiles,
): string | undefined {
    if (total !== 1 || !reason.topFollowerDid) return undefined;
    return discoverFollowerLabel(reason.topFollowerDid, profiles[reason.topFollowerDid]);
}

function ReasonPeople({
    followerDids,
    total,
    reason,
    profiles,
    expanded,
    contentState,
}: {
    followerDids: string[];
    total: number;
    reason: DiscoverReason;
    profiles: Profiles;
    expanded: boolean;
    contentState: ContentState;
}) {
    const extra = total - followerDids.length;
    const lastOrder = extra > 0 ? followerDids.length : followerDids.length - 1;
    return (
        <ReasonRow>
            {expanded ? (
                <ReasonLeadIn contentState={contentState}>
                    <FollowerAvatarStack
                        dids={followerDids}
                        profiles={profiles}
                        contentState={contentState}
                        lastOrder={lastOrder}
                    />
                    {extra > 0 && (
                        <CutPop
                            contentState={contentState}
                            order={followerDids.length}
                            lastOrder={lastOrder}
                        >
                            <span className="shrink-0 text-sm font-light text-muted-foreground">
                                +{extra}
                            </span>
                        </CutPop>
                    )}
                </ReasonLeadIn>
            ) : null}
            <span className="truncate text-sm font-light text-muted-foreground">
                {formatDiscoverReason(reason, soleFollowerLabel(total, reason, profiles))}
            </span>
        </ReasonRow>
    );
}

function FollowerAvatarStack({
    dids,
    profiles,
    contentState,
    lastOrder,
}: {
    dids: string[];
    profiles: Profiles;
    contentState: ContentState;
    lastOrder: number;
}) {
    return (
        <div className="flex shrink-0 items-center">
            {dids.map((did, i) => (
                <CutPop
                    key={did}
                    contentState={contentState}
                    order={i}
                    lastOrder={lastOrder}
                    className={cn(i > 0 && '-ml-2')}
                >
                    <ReasonAvatarLink
                        did={did}
                        profile={profiles[did]}
                        className="ring-2 ring-card"
                    />
                </CutPop>
            ))}
        </div>
    );
}
