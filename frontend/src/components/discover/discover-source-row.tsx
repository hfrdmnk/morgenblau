import { EyeOffIcon } from '@proicons/react';
import { motion, useReducedMotion } from 'motion/react';
import { useId, type ReactNode } from 'react';

import { CutPop } from '@/components/discover/cut-pop';
import {
    DiscoverRowFooter,
    DiscoverRowShell,
} from '@/components/discover/discover-row-shell';
import { SubscribeAction } from '@/components/discover/subscribe-action';
import { Favicon } from '@/components/favicon';
import { Button } from '@/components/ui/button';
import { Skeleton } from '@/components/ui/skeleton';
import { shortTimeAgo } from '@/lib/date';
import {
    discoverSourceFaviconUrl,
    discoverSourceLinkHref,
    discoverSourceTitle,
    type DiscoverReasonDisplay,
    type DiscoverSource,
    type DiscoverSourceCard,
    type DiscoverSourcePost,
    type PostsState,
} from '@/lib/discover';
import type { ContentState, DividerState } from '@/lib/discover-cut';
import { CUT_SNAP, splitFade, splitOpenFade } from '@/lib/motion-transitions';
import type { Profile } from '@/lib/profile';
import { cn, safeHref } from '@/lib/utils';

import { DiscoverReasonLine } from './discover-reason-line';

// SPEC <discovery> Cards, rendered as rows of the unified discover card.
export function DiscoverSourceRow({
    source,
    postsState,
    expanded,
    intentExpanded,
    contentState,
    onToggle,
    showDivider,
    dividerState,
    reasonDisplay,
    profiles,
    subscribed,
    onSubscribe,
    onHide,
}: {
    source: DiscoverSource;
    postsState: PostsState | undefined;
    expanded: boolean;
    intentExpanded: boolean;
    contentState: ContentState;
    onToggle: () => void;
    showDivider: boolean;
    dividerState: DividerState;
    reasonDisplay: DiscoverReasonDisplay;
    profiles: Record<string, Profile | undefined>;
    subscribed: boolean;
    onSubscribe: () => void;
    onHide: () => void;
}) {
    return (
        <SourceCardRow
            source={source}
            postsState={postsState}
            expanded={expanded}
            intentExpanded={intentExpanded}
            contentState={contentState}
            onToggle={onToggle}
            showDivider={showDivider}
            dividerState={dividerState}
            footerContext={
                <DiscoverReasonLine
                    display={reasonDisplay}
                    reason={source.reason}
                    profiles={profiles}
                    expanded={expanded}
                    contentState={contentState}
                />
            }
            subscribed={subscribed}
            onSubscribe={onSubscribe}
            onHide={onHide}
        />
    );
}

export function SourceCardRow({
    source,
    postsState,
    expanded,
    intentExpanded,
    contentState,
    onToggle,
    showDivider,
    dividerState,
    footerContext,
    subscribed,
    onSubscribe,
    onHide,
}: {
    source: DiscoverSourceCard;
    postsState: PostsState | undefined;
    expanded: boolean;
    intentExpanded: boolean;
    contentState: ContentState;
    onToggle: () => void;
    showDivider: boolean;
    dividerState: DividerState;
    footerContext?: ReactNode;
    subscribed: boolean;
    onSubscribe: () => void;
    onHide?: () => void;
}) {
    const postsId = useId();
    const subscribeAction = (
        <SubscribeAction subscribed={subscribed} onSubscribe={onSubscribe} />
    );
    const hasFooter = footerContext !== undefined;
    return (
        <DiscoverRowShell
            showDivider={showDivider}
            dividerState={dividerState}
            clickable
            onToggle={onToggle}
        >
            <CardHead
                source={source}
                expanded={expanded}
                intentExpanded={intentExpanded}
                contentState={contentState}
                postsId={postsId}
                onToggle={onToggle}
                onHide={onHide}
                inlineAction={hasFooter ? undefined : subscribeAction}
            />
            {expanded ? (
                <PostsDisclosure
                    postsId={postsId}
                    contentState={contentState}
                    postsState={postsState}
                />
            ) : null}
            {hasFooter ? (
                <DiscoverRowFooter
                    expanded={expanded}
                    contentState={contentState}
                    reason={footerContext}
                    action={subscribeAction}
                />
            ) : null}
        </DiscoverRowShell>
    );
}

// Direction picks the curve: opening releases (ease-out), closing settles with the merge.
function postsTransition(contentState: ContentState) {
    if (contentState === 'hold') return CUT_SNAP;
    if (contentState === 'closing') return splitFade();
    return splitOpenFade();
}

function PostsDisclosure({
    postsId,
    contentState,
    postsState,
}: {
    postsId: string;
    contentState: ContentState;
    postsState: PostsState | undefined;
}) {
    const reducedMotion = useReducedMotion();
    return (
        <motion.div
            initial={reducedMotion ? false : { height: 0, opacity: 0 }}
            animate={
                contentState === 'open'
                    ? { height: 'auto', opacity: 1 }
                    : { height: 0, opacity: 0 }
            }
            transition={postsTransition(contentState)}
            className="overflow-hidden"
            id={postsId}
        >
            <div className="pt-4">
                <PostsDisclosureBody postsState={postsState} />
            </div>
        </motion.div>
    );
}

function CardHead({
    source,
    expanded,
    intentExpanded,
    contentState,
    postsId,
    onToggle,
    onHide,
    inlineAction,
}: {
    source: DiscoverSourceCard;
    expanded: boolean;
    intentExpanded: boolean;
    contentState: ContentState;
    postsId: string;
    onToggle: () => void;
    onHide?: () => void;
    inlineAction?: ReactNode;
}) {
    const title = discoverSourceTitle(source);
    const href = discoverSourceLinkHref(source);
    return (
        <div className="flex items-start gap-3">
            <button
                type="button"
                aria-label={`${intentExpanded ? 'Collapse' : 'Expand'} ${title}`}
                aria-expanded={intentExpanded}
                aria-controls={intentExpanded ? postsId : undefined}
                onClick={onToggle}
                className="shrink-0 cursor-pointer rounded-lg outline-none focus-visible:outline-solid focus-visible:outline-1 focus-visible:outline-offset-2 focus-visible:outline-ring"
            >
                <Favicon
                    src={discoverSourceFaviconUrl(source)}
                    className="size-[34px] rounded-lg"
                />
            </button>
            <div className="flex min-w-0 flex-1 items-baseline gap-2">
                <div className="min-w-0 flex-1">
                    <SourceTitle title={title} href={href} />
                </div>
                <InlineHeadAction action={inlineAction} />
                <ExpandedHeadActions
                    expanded={expanded}
                    contentState={contentState}
                    onHide={onHide}
                />
            </div>
        </div>
    );
}

function InlineHeadAction({ action }: { action?: ReactNode }) {
    if (!action) return null;
    return <div className="shrink-0">{action}</div>;
}

function ExpandedHeadActions({
    expanded,
    contentState,
    onHide,
}: {
    expanded: boolean;
    contentState: ContentState;
    onHide?: () => void;
}) {
    if (!expanded || !onHide) return null;
    return (
        <div
            className={cn(
                'flex shrink-0 items-center gap-0.5',
                contentState !== 'open' && 'pointer-events-none',
            )}
        >
            <HideSourceAction
                onHide={onHide}
                contentState={contentState}
            />
        </div>
    );
}

function HideSourceAction({
    onHide,
    contentState,
}: {
    onHide: () => void;
    contentState: ContentState;
}) {
    return (
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
    );
}

function SourceTitle({
    title,
    href,
}: {
    title: string;
    href: string | undefined;
}) {
    const className =
        'inline-block max-w-full truncate rounded-sm text-lg font-medium tracking-tight text-foreground outline-none hover:underline focus-visible:outline-solid focus-visible:outline-1 focus-visible:outline-offset-2 focus-visible:outline-ring';
    if (!href) return <span className={className}>{title}</span>;
    return (
        <a
            href={href}
            target="_blank"
            rel="noopener noreferrer"
            className={className}
        >
            {title}
        </a>
    );
}

function PostsDisclosureBody({ postsState }: { postsState: PostsState | undefined }) {
    if (!postsState || postsState.status === 'loading') return <PostsPreviewSkeleton />;
    if (postsState.posts.length === 0) {
        return <p className="text-sm font-light text-muted-foreground">No recent posts.</p>;
    }
    return <PostsList posts={postsState.posts} />;
}

function PostsList({ posts }: { posts: DiscoverSourcePost[] }) {
    return (
        <>
            <p className="mb-1 text-sm font-light text-muted-foreground">
                Last {posts.length} post{posts.length === 1 ? '' : 's'}
            </p>
            {posts.map((post) => (
                <PostRow key={post.key} post={post} />
            ))}
        </>
    );
}

function PostsPreviewSkeleton() {
    return (
        <div aria-busy aria-label="Loading recent posts">
            <Skeleton className="mb-1 h-3 w-24" />
            {Array.from({ length: 3 }).map((_, i) => (
                <div key={i} className="flex items-baseline gap-4 py-0.5">
                    <Skeleton className="h-4 flex-1" />
                    <Skeleton className="h-3 w-10 shrink-0" />
                </div>
            ))}
        </div>
    );
}

function PostRow({ post }: { post: DiscoverSourcePost }) {
    const href = safeHref(post.url);
    return (
        <div className="flex items-baseline gap-4 py-0.5">
            <PostTitle post={post} href={href} />
            <span className="shrink-0 text-xs font-light text-muted-foreground tabular-nums">
                {shortTimeAgo(post.publishedAt)}
            </span>
        </div>
    );
}

function PostTitle({
    post,
    href,
}: {
    post: DiscoverSourcePost;
    href: string | undefined;
}) {
    const className = 'flex-1 min-w-0 truncate text-sm text-foreground';
    if (href) {
        return (
            <a href={href} target="_blank" rel="noopener noreferrer" className={cn(className, 'hover:underline')}>
                {post.title}
            </a>
        );
    }
    return <span className={className}>{post.title}</span>;
}
