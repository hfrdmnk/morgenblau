import { ArrowTrendingIcon } from '@proicons/react';
import { motion } from 'motion/react';
import type { Transition } from 'motion/react';
import { useState, type ReactNode } from 'react';
import { Link } from 'wouter';

import { CutPop } from '@/components/discover/cut-pop';
import { Avatar, AvatarFallback, AvatarImage } from '@/components/ui/avatar';
import { Skeleton } from '@/components/ui/skeleton';
import { discoverFollowerLabel } from '@/lib/discover';
import type { ContentState } from '@/lib/discover-cut';
import { initialsFromHandle } from '@/lib/handle';
import { CUT_SNAP, split, splitOpen } from '@/lib/motion-transitions';
import { personHref } from '@/lib/paths';
import type { Profile } from '@/lib/profile';
import { safeHref } from '@/lib/utils';

// min-h-6 reserves the avatar row's height on every variant so a reveal never changes row height.
export function ReasonRow({ children }: { children: ReactNode }) {
    return <div className="flex min-h-6 min-w-0 items-center">{children}</div>;
}

// Width and trailing gap tween together so at 0 the text sits exactly at its collapsed position.
function leadInTransition(contentState: ContentState): Transition {
    if (contentState === 'hold') return CUT_SNAP;
    if (contentState === 'closing') return split();
    return splitOpen();
}

// Cut-driven, never mount-driven: re-parented rows remount mid-cut and must not replay an entrance.
// The plan's row mounts hidden on the committed cut frame ('hold') and reveals when 'open' lands.
export function ReasonLeadIn({
    contentState,
    children,
}: {
    contentState: ContentState;
    children: ReactNode;
}) {
    const open = contentState === 'open';
    return (
        <motion.div
            initial={false}
            animate={
                open
                    ? { width: 'auto', marginRight: 8 }
                    : { width: 0, marginRight: 0 }
            }
            transition={leadInTransition(contentState)}
            className="flex shrink-0 items-center gap-2"
        >
            {children}
        </motion.div>
    );
}

export function ReasonText({ text }: { text: string }) {
    return (
        <ReasonRow>
            <span className="truncate text-sm font-light text-muted-foreground">
                {text}
            </span>
        </ReasonRow>
    );
}

export function ReasonTrending({
    expanded,
    contentState,
}: {
    expanded: boolean;
    contentState: ContentState;
}) {
    return (
        <ReasonRow>
            {expanded ? (
                <ReasonLeadIn contentState={contentState}>
                    <CutPop contentState={contentState} order={0}>
                        <span className="flex size-6 shrink-0 items-center justify-center rounded-full bg-overlay-2">
                            <ArrowTrendingIcon className="size-3.5 text-muted-foreground" />
                        </span>
                    </CutPop>
                </ReasonLeadIn>
            ) : null}
            <span className="truncate text-sm font-light text-muted-foreground">
                Trending in the reader network
            </span>
        </ReasonRow>
    );
}

// CutPop renders children verbatim (no clone/ref), so wrapping the avatar in a Link doesn't
// touch the pop/skeleton reveal; blockification inside CutPop's flex box keeps sizing intact.
export function ReasonAvatarLink({
    did,
    profile,
    className,
}: {
    did: string;
    profile: Profile | undefined;
    className?: string;
}) {
    return (
        <Link
            href={personHref(did, profile?.handle)}
            aria-label={discoverFollowerLabel(did, profile)}
            className="rounded-full outline-none focus-visible:outline-solid focus-visible:outline-1 focus-visible:outline-offset-2 focus-visible:outline-ring"
        >
            <ReasonAvatar did={did} profile={profile} className={className} />
        </Link>
    );
}

// The stack overlaps avatars, so the fallback layers its tint over bg-card to stay opaque; a
// skeleton holds the slot while the profile or its image loads, initials only when neither exists.
export function ReasonAvatar({
    did,
    profile,
    className,
}: {
    did: string;
    profile: Profile | undefined;
    className?: string;
}) {
    const avatarSrc = safeProfileAvatar(profile);
    const [failed, setFailed] = useState(false);
    return (
        <Avatar size="sm" className={className}>
            {avatarSrc && (
                <AvatarImage
                    src={avatarSrc}
                    alt=""
                    onLoadingStatusChange={(status) => setFailed(status === 'error')}
                />
            )}
            <AvatarFallback className="bg-card">
                <ReasonAvatarFallback did={did} profile={profile} failed={failed} />
            </AvatarFallback>
        </Avatar>
    );
}

function ReasonAvatarFallback({
    did,
    profile,
    failed,
}: {
    did: string;
    profile: Profile | undefined;
    failed: boolean;
}) {
    const showInitials = profile !== undefined && (!safeProfileAvatar(profile) || failed);
    if (!showInitials) return <Skeleton className="size-full rounded-full" />;
    return (
        <span className="flex size-full items-center justify-center rounded-full bg-muted">
            {initialsFromHandle(profileHandle(profile), did)}
        </span>
    );
}

function safeProfileAvatar(profile: Profile | undefined): string | undefined {
    return safeHref(profile?.avatar);
}

function profileHandle(profile: Profile | undefined): string | null {
    return profile?.handle ?? null;
}
