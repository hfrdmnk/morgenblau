import {
    EyeOffIcon,
    PersonAddIcon,
    PersonIcon,
    SubtractIcon,
} from '@proicons/react';
import { motion, useReducedMotion } from 'motion/react';
import type { Transition } from 'motion/react';
import { useId, type ReactNode } from 'react';
import { Link } from 'wouter';

import { CutPop } from '@/components/discover/cut-pop';
import {
    DiscoverRowFooter,
    DiscoverRowShell,
} from '@/components/discover/discover-row-shell';
import {
    ReasonAvatar,
    ReasonLeadIn,
    ReasonRow,
    ReasonText,
    ReasonTrending,
} from '@/components/discover/reason-primitives';
import { InertBadge, SubscribeAction } from '@/components/discover/subscribe-action';
import { Favicon } from '@/components/favicon';
import { Avatar, AvatarFallback, AvatarImage } from '@/components/ui/avatar';
import { Button } from '@/components/ui/button';
import { Skeleton } from '@/components/ui/skeleton';
import { usePopOnRise } from '@/hooks/use-pop-on-rise';
import { shortTimeAgo } from '@/lib/date';
import { discoverSourceFaviconUrl } from '@/lib/discover';
import type { ContentState, DividerState } from '@/lib/discover-cut';
import {
    personPreviewHiddenCount,
    personPreviewEmpty,
    type PersonPreview,
    type PersonPreviewShare,
    type PersonPreviewSource,
    type PersonReasonDisplay,
    type PreviewState,
} from '@/lib/discover-people';
import { initialsFromHandle, personRowLines } from '@/lib/handle';
import { CUT_SNAP, splitFade, splitOpenFade } from '@/lib/motion-transitions';
import { personHref } from '@/lib/paths';
import type { Profile } from '@/lib/profile';
import {
    shareTargetPresentation,
    type ShareTargetPresentation,
} from '@/lib/share-target';
import { cn, safeHref } from '@/lib/utils';

type CommonRowProps = {
    did: string;
    profile: Profile | undefined;
    previewState: PreviewState | undefined;
    expanded: boolean;
    intentExpanded: boolean;
    contentState: ContentState;
    onToggle: () => void;
    showDivider: boolean;
    dividerState: DividerState;
    isSubscribed: (source: PersonPreviewSource) => boolean;
    onSubscribeSource: (source: PersonPreviewSource) => void;
};

type SuggestionRowProps = CommonRowProps & {
    variant: 'suggestion';
    reasonDisplay: PersonReasonDisplay;
    profiles: Record<string, Profile | undefined>;
    following: boolean;
    followPending: boolean;
    canFollow: boolean;
    onFollow: () => void;
    onHide: () => void;
};

type FollowRowProps = CommonRowProps & {
    variant: 'follow';
    onUnfollow: () => void;
};

// SPEC <discovery> line 558: a materialized search result — follow action, no hide, always
// expanded, no disclosure chevron since there's nothing to collapse it back into.
type SearchedRowProps = CommonRowProps & {
    variant: 'searched';
    isSelf: boolean;
    following: boolean;
    followPending: boolean;
    canFollow: boolean;
    onFollow: () => void;
    presenceless: boolean;
};

type RowProps = SuggestionRowProps | FollowRowProps | SearchedRowProps;

// SPEC <discovery> Cards: person cards mirror source cards — head, a writes/reads/shares
// expansion, one reason line, and a follow (or unfollow) action, rendered as rows of a cut card.
export function DiscoverPersonRow(props: RowProps) {
    const previewId = useId();
    return (
        <DiscoverRowShell
            showDivider={props.showDivider}
            dividerState={props.dividerState}
            clickable={props.variant !== 'searched'}
            onToggle={props.onToggle}
        >
            <PersonHead
                did={props.did}
                profile={props.profile}
                expanded={props.expanded}
                intentExpanded={props.intentExpanded}
                contentState={props.contentState}
                previewId={previewId}
                onToggle={props.onToggle}
                onHide={suggestionHideAction(props)}
                showToggle={props.variant !== 'searched'}
                inlineAction={<InlinePersonRowAction row={props} />}
            />
            <ExpandedPersonRowPreview row={props} previewId={previewId} />
            <PersonRowFooter row={props} />
        </DiscoverRowShell>
    );
}

function suggestionHideAction(row: RowProps): (() => void) | undefined {
    return row.variant === 'suggestion' ? row.onHide : undefined;
}

function InlinePersonRowAction({ row }: { row: RowProps }) {
    if (row.variant === 'suggestion') return null;
    return <PersonRowAction row={row} />;
}

function PersonRowFooter({ row }: { row: RowProps }) {
    if (row.variant !== 'suggestion') return null;
    return (
        <DiscoverRowFooter
            expanded={row.expanded}
            contentState={row.contentState}
            reason={<PersonRowReason row={row} />}
            action={<PersonRowAction row={row} />}
        />
    );
}

function ExpandedPersonRowPreview({
    row,
    previewId,
}: {
    row: RowProps;
    previewId: string;
}) {
    if (!row.expanded) return null;
    return <PersonRowPreview row={row} previewId={previewId} />;
}

function PersonRowPreview({
    row,
    previewId,
}: {
    row: RowProps;
    previewId: string;
}) {
    return (
        <PreviewDisclosure
            previewId={previewId}
            contentState={row.contentState}
            previewState={row.previewState}
            isSubscribed={row.isSubscribed}
            onSubscribeSource={row.onSubscribeSource}
            presenceless={row.variant === 'searched' ? row.presenceless : undefined}
            revealOnMount={row.variant !== 'searched'}
        />
    );
}

function PersonRowReason({ row }: { row: RowProps }) {
    if (row.variant !== 'suggestion') return null;
    return (
        <PersonReasonLine
            display={row.reasonDisplay}
            profiles={row.profiles}
            expanded={row.expanded}
            contentState={row.contentState}
        />
    );
}

function PersonRowAction({ row }: { row: RowProps }) {
    if (row.variant === 'follow') {
        return (
            <Button
                variant="ghost"
                size="sm"
                iconTint="error"
                onClick={row.onUnfollow}
            >
                <PersonMinusIcon />
                Unfollow
            </Button>
        );
    }
    if (row.variant === 'searched' && row.isSelf) return null;
    return (
        <FollowAction
            following={row.following}
            pending={row.followPending}
            canFollow={row.canFollow}
            onFollow={row.onFollow}
        />
    );
}

function PersonHead({
    did,
    profile,
    expanded,
    intentExpanded,
    contentState,
    previewId,
    onToggle,
    onHide,
    showToggle,
    inlineAction,
}: {
    did: string;
    profile: Profile | undefined;
    expanded: boolean;
    intentExpanded: boolean;
    contentState: ContentState;
    previewId: string;
    onToggle: () => void;
    onHide: (() => void) | undefined;
    showToggle: boolean;
    inlineAction?: ReactNode;
}) {
    return (
        <div className="flex items-start gap-3">
            <PersonAvatar
                profile={profile}
                did={did}
                collapsible={showToggle}
                expanded={intentExpanded}
                previewId={previewId}
                onToggle={onToggle}
            />
            <div className="flex min-w-0 flex-1 items-baseline gap-2">
                <PersonHeadLines did={did} profile={profile} />
                <InlinePersonHeadAction action={inlineAction} />
                <ExpandedPersonHeadActions
                    expanded={expanded}
                    contentState={contentState}
                    onHide={onHide}
                />
            </div>
        </div>
    );
}

function InlinePersonHeadAction({ action }: { action?: ReactNode }) {
    if (!action) return null;
    return <div className="shrink-0">{action}</div>;
}

function ExpandedPersonHeadActions({
    expanded,
    contentState,
    onHide,
}: {
    expanded: boolean;
    contentState: ContentState;
    onHide: (() => void) | undefined;
}) {
    if (!expanded || !onHide) return null;
    return (
        <PersonHeadActions
            contentState={contentState}
            onHide={onHide}
        />
    );
}

function PersonHeadLines({
    did,
    profile,
}: {
    did: string;
    profile: Profile | undefined;
}) {
    const lines = personRowLines({
        handle: profile?.handle,
        displayName: profile?.displayName,
        did,
        handleAsSecondary: true,
    });
    return (
        <div className="min-w-0 flex-1">
            <PersonName
                name={lines.primary}
                href={personHref(did, profile?.handle)}
            />
            <PersonSecondaryLine text={lines.secondary} />
        </div>
    );
}

function PersonSecondaryLine({ text }: { text: string | undefined }) {
    if (!text) return null;
    return (
        <p className="truncate text-sm font-light text-muted-foreground">
            {text}
        </p>
    );
}

function PersonAvatar({
    profile,
    did,
    collapsible,
    expanded,
    previewId,
    onToggle,
}: {
    profile: Profile | undefined;
    did: string;
    collapsible: boolean;
    expanded: boolean;
    previewId: string;
    onToggle: () => void;
}) {
    if (!collapsible) {
        return <PersonAvatarImage profile={profile} did={did} />;
    }
    return (
        <PersonDisclosureAvatar
            profile={profile}
            did={did}
            name={personPrimaryName(profile, did)}
            expanded={expanded}
            previewId={previewId}
            onToggle={onToggle}
        />
    );
}

function personPrimaryName(profile: Profile | undefined, did: string) {
    return personRowLines({
        handle: profile?.handle,
        displayName: profile?.displayName,
        did,
    }).primary;
}

function PersonAvatarImage({
    profile,
    did,
}: {
    profile: Profile | undefined;
    did: string;
}) {
    const avatarSrc = safeHref(profile?.avatar);
    return (
        <Avatar className="size-9 shrink-0">
            {avatarSrc ? <AvatarImage src={avatarSrc} alt="" /> : null}
            <AvatarFallback>{initialsFromHandle(profile?.handle, did)}</AvatarFallback>
        </Avatar>
    );
}

function PersonDisclosureAvatar({
    profile,
    did,
    name,
    expanded,
    previewId,
    onToggle,
}: {
    profile: Profile | undefined;
    did: string;
    name: string;
    expanded: boolean;
    previewId: string;
    onToggle: () => void;
}) {
    return (
        <button
            type="button"
            aria-label={`${expanded ? 'Collapse' : 'Expand'} ${name}`}
            aria-expanded={expanded}
            aria-controls={expanded ? previewId : undefined}
            onClick={onToggle}
            className="shrink-0 cursor-pointer rounded-full outline-none focus-visible:outline-solid focus-visible:outline-1 focus-visible:outline-offset-2 focus-visible:outline-ring"
        >
            <PersonAvatarImage profile={profile} did={did} />
        </button>
    );
}

function PersonName({
    name,
    href,
}: {
    name: string;
    href: string;
}) {
    const className =
        'inline-block max-w-full truncate rounded-sm text-lg font-medium tracking-tight text-foreground outline-none hover:underline focus-visible:outline-solid focus-visible:outline-1 focus-visible:outline-offset-2 focus-visible:outline-ring';
    return (
        <Link href={href} className={className}>
            {name}
        </Link>
    );
}

function PersonHeadActions({
    contentState,
    onHide,
}: {
    contentState: ContentState;
    onHide: () => void;
}) {
    return (
        <div
            className={cn(
                'flex shrink-0 items-center gap-0.5',
                contentState !== 'open' && 'pointer-events-none',
            )}
        >
            <CutPop contentState={contentState} order={0} lastOrder={0}>
                <Button
                    variant="ghost"
                    size="icon-sm"
                    className="text-muted-foreground"
                    aria-label="Not interested"
                    onClick={onHide}
                >
                    <EyeOffIcon className="size-[1.125rem]" />
                </Button>
            </CutPop>
        </div>
    );
}

// Direction picks the curve: opening releases (ease-out), closing settles with the merge.
function previewTransition(contentState: ContentState): Transition {
    if (contentState === 'hold') return CUT_SNAP;
    if (contentState === 'closing') return splitFade();
    return splitOpenFade();
}

function PreviewDisclosure({
    previewId,
    contentState,
    previewState,
    isSubscribed,
    onSubscribeSource,
    presenceless,
    revealOnMount = true,
}: {
    previewId: string;
    contentState: ContentState;
    previewState: PreviewState | undefined;
    isSubscribed: (source: PersonPreviewSource) => boolean;
    onSubscribeSource: (source: PersonPreviewSource) => void;
    presenceless?: boolean;
    // False for a row born already-expanded (the search slot card): its own height:0→auto tween
    // would race the card's identical entrance tween on the same 'auto' measurement.
    revealOnMount?: boolean;
}) {
    const reducedMotion = useReducedMotion();
    return (
        <motion.div
            initial={reducedMotion || !revealOnMount ? false : { height: 0, opacity: 0 }}
            animate={
                contentState === 'open'
                    ? { height: 'auto', opacity: 1 }
                    : { height: 0, opacity: 0 }
            }
            transition={previewTransition(contentState)}
            className="overflow-hidden"
            id={previewId}
        >
            <div className="pt-3">
                <PreviewBody
                    previewState={previewState}
                    isSubscribed={isSubscribed}
                    onSubscribeSource={onSubscribeSource}
                    presenceless={presenceless}
                />
            </div>
        </motion.div>
    );
}

function PreviewBody({
    previewState,
    isSubscribed,
    onSubscribeSource,
    presenceless,
}: {
    previewState: PreviewState | undefined;
    isSubscribed: (source: PersonPreviewSource) => boolean;
    onSubscribeSource: (source: PersonPreviewSource) => void;
    presenceless?: boolean;
}) {
    // A presence-less search result has nothing to fetch: SPEC <discovery> line 558's honest
    // emptiness, shown immediately rather than behind a loading skeleton.
    if (presenceless) {
        return <PreviewNote text="Not in the reader network yet." />;
    }
    if (!previewState || previewState.status === 'loading') {
        return <PreviewSkeleton />;
    }
    return (
        <PreviewLoaded
            preview={previewState.preview}
            isSubscribed={isSubscribed}
            onSubscribeSource={onSubscribeSource}
        />
    );
}

function PreviewNote({ text }: { text: string }) {
    return <p className="text-sm font-light text-muted-foreground">{text}</p>;
}

function PreviewLoaded({
    preview,
    isSubscribed,
    onSubscribeSource,
}: {
    preview: PersonPreview;
    isSubscribed: (source: PersonPreviewSource) => boolean;
    onSubscribeSource: (source: PersonPreviewSource) => void;
}) {
    if (personPreviewEmpty(preview)) {
        return <PreviewNote text="Nothing to preview yet." />;
    }
    return (
        <div className="flex flex-col gap-3">
            <PreviewSourcesSection
                label="Writes"
                sources={preview.writes}
                total={preview.writesTotal}
                isSubscribed={isSubscribed}
                onSubscribeSource={onSubscribeSource}
            />
            <PreviewSourcesSection
                label="Reads"
                sources={preview.reads}
                total={preview.readsTotal}
                isSubscribed={isSubscribed}
                onSubscribeSource={onSubscribeSource}
            />
            {preview.latestShare ? (
                <PreviewSection label="Latest share">
                    <ShareRow share={preview.latestShare} />
                </PreviewSection>
            ) : null}
        </div>
    );
}

function PreviewSourcesSection({
    label,
    sources,
    total,
    isSubscribed,
    onSubscribeSource,
}: {
    label: string;
    sources: PersonPreviewSource[];
    total: number | undefined;
    isSubscribed: (source: PersonPreviewSource) => boolean;
    onSubscribeSource: (source: PersonPreviewSource) => void;
}) {
    if (sources.length === 0) return null;
    const hiddenCount = personPreviewHiddenCount(total, sources.length);
    return (
        <PreviewSection label={label}>
            {sources.map((source) => (
                <PreviewSourceRow
                    key={source.key}
                    source={source}
                    subscribed={isSubscribed(source)}
                    onSubscribe={() => onSubscribeSource(source)}
                />
            ))}
            {hiddenCount > 0 ? <PreviewMoreRow count={hiddenCount} /> : null}
        </PreviewSection>
    );
}

function PreviewSection({
    label,
    children,
}: {
    label: string;
    children: ReactNode;
}) {
    return (
        <div>
            <p className="mb-1 text-sm font-light text-muted-foreground">{label}</p>
            {children}
        </div>
    );
}

function PreviewSourceRow({
    source,
    subscribed,
    onSubscribe,
}: {
    source: PersonPreviewSource;
    subscribed: boolean;
    onSubscribe: () => void;
}) {
    const href = safeHref(source.siteUrl);
    return (
        <div className="flex items-center gap-3 py-0.5">
            <Favicon
                src={discoverSourceFaviconUrl(source)}
                className="size-[18px] shrink-0 rounded-sm"
            />
            <PreviewSourceTitle title={source.title} href={href} />
            <SubscribeAction subscribed={subscribed} onSubscribe={onSubscribe} size="xs" />
        </div>
    );
}

function PreviewMoreRow({ count }: { count: number }) {
    return (
        <div className="flex items-center gap-3 py-0.5">
            <span aria-hidden className="size-4.5 shrink-0" />
            <p className="text-sm font-light text-muted-foreground">+{count} more</p>
        </div>
    );
}

function PreviewSourceTitle({
    title,
    href,
}: {
    title: string;
    href: string | undefined;
}) {
    const className =
        'inline-block max-w-full truncate rounded-sm text-sm text-foreground outline-none hover:underline focus-visible:outline-solid focus-visible:outline-1 focus-visible:outline-offset-2 focus-visible:outline-ring';
    if (href) {
        return (
            <div className="min-w-0 flex-1">
                <a
                    href={href}
                    target="_blank"
                    rel="noopener noreferrer"
                    className={className}
                >
                    {title}
                </a>
            </div>
        );
    }
    return (
        <span className="min-w-0 flex-1 truncate text-sm text-foreground">
            {title}
        </span>
    );
}

function PersonMinusIcon() {
    return (
        <span aria-hidden className="relative size-3.5 shrink-0">
            <PersonIcon className="absolute inset-0 size-3.5" />
            <SubtractIcon className="absolute -right-0.5 bottom-0 size-2.5" />
        </span>
    );
}

function ShareRow({ share }: { share: PersonPreviewShare }) {
    const target = shareTargetPresentation(share);
    return (
        <div className="py-0.5">
            <div className="flex items-baseline gap-4">
                <ShareLabel target={target} />
                <span className="shrink-0 text-xs font-light text-muted-foreground tabular-nums">
                    {shortTimeAgo(share.createdAt)}
                </span>
            </div>
            {share.comment ? (
                <p className="mt-0.5 line-clamp-2 text-sm font-light text-muted-foreground">
                    {share.comment}
                </p>
            ) : null}
        </div>
    );
}

function ShareLabel({ target }: { target: ShareTargetPresentation }) {
    const className =
        'min-w-0 flex-1 truncate rounded-sm text-sm text-foreground outline-none hover:underline focus-visible:outline-solid focus-visible:outline-1 focus-visible:outline-offset-2 focus-visible:outline-ring';
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
    return <span className={className}>{target.label}</span>;
}

function PreviewSkeleton() {
    return (
        <div aria-busy aria-label="Loading preview" className="flex flex-col gap-3">
            {Array.from({ length: 2 }).map((_, section) => (
                <div key={section}>
                    <Skeleton className="mb-1 h-3 w-16" />
                    {Array.from({ length: 2 }).map((_, i) => (
                        <div key={i} className="flex items-center gap-3 py-0.5">
                            <Skeleton className="size-[18px] shrink-0 rounded-sm" />
                            <Skeleton className="h-4 flex-1" />
                            <Skeleton className="h-6 w-16 shrink-0 rounded-lg" />
                        </div>
                    ))}
                </div>
            ))}
        </div>
    );
}

function PersonReasonLine({
    display,
    profiles,
    expanded,
    contentState,
}: {
    display: PersonReasonDisplay;
    profiles: Record<string, Profile | undefined>;
    expanded: boolean;
    contentState: ContentState;
}) {
    if (display.kind === 'text') {
        return <ReasonText text={display.text} />;
    }
    if (display.kind === 'followedBy') {
        return (
            <ReasonFollowedBy
                did={display.did}
                profile={profiles[display.did]}
                expanded={expanded}
                contentState={contentState}
            />
        );
    }
    return <ReasonTrending expanded={expanded} contentState={contentState} />;
}

function ReasonFollowedBy({
    did,
    profile,
    expanded,
    contentState,
}: {
    did: string;
    profile: Profile | undefined;
    expanded: boolean;
    contentState: ContentState;
}) {
    const handle = profile?.handle;
    return (
        <ReasonRow>
            {expanded ? (
                <ReasonLeadIn contentState={contentState}>
                    <CutPop contentState={contentState} order={0}>
                        <ReasonAvatar did={did} profile={profile} />
                    </CutPop>
                </ReasonLeadIn>
            ) : null}
            <span className="truncate text-sm font-light text-muted-foreground">
                followed by{' '}
                {handle ? (
                    <Link
                        href={personHref(did, handle)}
                        className="rounded-sm text-foreground outline-none hover:underline focus-visible:outline-solid focus-visible:outline-1 focus-visible:outline-offset-2 focus-visible:outline-ring"
                    >
                        @{handle}
                    </Link>
                ) : (
                    'someone you follow'
                )}
            </span>
        </ReasonRow>
    );
}

// SPEC <discovery>: following flips the card to an inert state in place, like a subscribed source.
function FollowAction({
    following,
    pending,
    canFollow,
    onFollow,
}: {
    following: boolean;
    pending: boolean;
    canFollow: boolean;
    onFollow: () => void;
}) {
    const { pop, endPop } = usePopOnRise(following);
    if (following) {
        return <InertBadge label="Following" pop={pop} onPopEnd={endPop} />;
    }
    return <FollowButton pending={pending} canFollow={canFollow} onFollow={onFollow} />;
}

function FollowButton({
    pending,
    canFollow,
    onFollow,
}: {
    pending: boolean;
    canFollow: boolean;
    onFollow: () => void;
}) {
    return (
        <Button
            variant="ghost"
            size="sm"
            iconTint="primary"
            disabled={pending || !canFollow}
            onClick={onFollow}
        >
            <PersonAddIcon />
            {pending ? 'Following…' : 'Follow'}
        </Button>
    );
}
