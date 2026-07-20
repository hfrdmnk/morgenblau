import { ArrowLeftIcon } from '@proicons/react';
import {
    useEffect,
    useReducer,
    useRef,
    useState,
    type ReactNode,
} from 'react';
import { Link, useParams } from 'wouter';

import { CutSegmentStack } from '@/components/discover/cut-segment-stack';
import { DiscoverLoadMoreFooter } from '@/components/discover/discover-load-more-footer';
import { SourceCardRow } from '@/components/discover/discover-source-row';
import { SubscribeDialog } from '@/components/discover/subscribe-dialog';
import { Subnav } from '@/components/subnav';
import { Avatar, AvatarFallback, AvatarImage } from '@/components/ui/avatar';
import { Button } from '@/components/ui/button';
import { Skeleton } from '@/components/ui/skeleton';
import { useDiscoverCut } from '@/hooks/use-discover-cut';
import { useDocumentTitle } from '@/hooks/use-document-title';
import { useGoBackOr } from '@/hooks/use-go-back-or';
import { useSubscribeTarget } from '@/hooks/use-subscribe-target';
import { api } from '@/lib/api';
import { shortTimeAgo } from '@/lib/date';
import {
    type DiscoverSourceCard,
    type DiscoverSourcePost,
    type PostsState,
} from '@/lib/discover';
import type { PersonPreviewSource } from '@/lib/discover-people';
import { partitionSourceSegments } from '@/lib/discover-segments';
import { requestFollow, requestUnfollow } from '@/lib/follow';
import { initialsFromHandle } from '@/lib/handle';
import { toastMutationError } from '@/lib/mutation-toast';
import { PATHS } from '@/lib/paths';
import {
    PENDING_FOLLOW,
    SEGMENT_LABEL,
    appendAction,
    canUnfollow,
    defaultSegment,
    initialListsState,
    isShareItem,
    loadedAction,
    metaLine,
    profileDisplayName,
    profileItemKey,
    profileListsReducer,
    profileTitle,
    visibleSegments,
    type ProfileCounts,
    type ProfileListItem,
    type ProfileShareItem,
    type Segment,
    type SegmentState,
} from '@/lib/profile-page';
import {
    shareTargetPresentation,
    type ShareTargetPresentation,
} from '@/lib/share-target';
import { isPlainLeftClick, safeHref } from '@/lib/utils';

type ProfileHeader = {
    did: string;
    handle: string;
    displayName?: string | null;
    avatar?: string | null;
    description?: string | null;
    isSelf: boolean;
    followRkey: string | null;
    counts: ProfileCounts;
};

type State =
    | { kind: 'loading' }
    | { kind: 'ok'; handleOrDid: string; profile: ProfileHeader }
    | { kind: 'error' };

const EMPTY_SEGMENT_COPY: Record<Segment, string> = {
    writes: 'Hasn’t published anything yet.',
    reads: 'Hasn’t subscribed to any sources yet.',
    shares: 'Hasn’t shared anything yet.',
};

export function Profile() {
    const { handleOrDid } = useParams<{ handleOrDid: string }>();
    const [state, setState] = useState<State>(
        handleOrDid ? { kind: 'loading' } : { kind: 'error' },
    );

    useDocumentTitle(profileTitle(state.kind === 'ok' ? state.profile : undefined));

    useEffect(() => {
        if (!handleOrDid) return;
        let cancelled = false;
        api<ProfileHeader>(`/api/profile/${encodeURIComponent(handleOrDid)}`)
            .then((profile) => {
                if (!cancelled) setState({ kind: 'ok', handleOrDid, profile });
            })
            .catch(() => {
                if (!cancelled) setState({ kind: 'error' });
            });
        return () => {
            cancelled = true;
        };
    }, [handleOrDid]);

    if (state.kind !== 'ok') {
        return <ProfileFallback kind={state.kind} />;
    }

    const onFollowChanged = (followRkey: string | null) => {
        setState((cur) =>
            cur.kind === 'ok'
                ? { ...cur, profile: { ...cur.profile, followRkey } }
                : cur,
        );
    };

    return (
        <ProfileView
            key={state.handleOrDid}
            handleOrDid={state.handleOrDid}
            profile={state.profile}
            onFollowChanged={onFollowChanged}
        />
    );
}

function ProfileFallback({ kind }: { kind: 'loading' | 'error' }) {
    if (kind === 'loading') return <ProfileSkeleton />;
    return (
        <main className="mx-auto w-full max-w-2xl px-4 pt-16 pb-12 sm:px-6">
            <p className="text-sm font-light text-muted-foreground">
                Couldn't load this profile.
            </p>
        </main>
    );
}

function fetchSegmentPage(
    handleOrDid: string,
    segment: Segment,
    cursor?: string,
    signal?: AbortSignal,
) {
    const base = `/api/profile/${encodeURIComponent(handleOrDid)}/${segment}`;
    const url = cursor ? `${base}?cursor=${encodeURIComponent(cursor)}` : base;
    return api<{ items: ProfileListItem[]; nextCursor?: string } | null>(url, {
        signal,
    });
}

function ProfileView({
    handleOrDid,
    profile,
    onFollowChanged,
}: {
    handleOrDid: string;
    profile: ProfileHeader;
    onFollowChanged: (followRkey: string | null) => void;
}) {
    const segments = visibleSegments(profile.counts);
    const [activeSegment, setActiveSegment] = useState<Segment>(() =>
        defaultSegment(profile.counts),
    );
    const [lists, dispatch] = useReducer(
        profileListsReducer,
        undefined,
        initialListsState,
    );
    const loadedSegmentsRef = useRef<Set<Segment>>(new Set());
    const loadMoreControllers = useRef<Map<Segment, AbortController>>(
        new Map(),
    );
    const subscribe = useSubscribeTarget<DiscoverSourceCard>();

    // A segment earns its fetch-once marker only after its response is accepted.
    useEffect(() => {
        if (loadedSegmentsRef.current.has(activeSegment)) return;
        const controller = new AbortController();
        fetchSegmentPage(
            handleOrDid,
            activeSegment,
            undefined,
            controller.signal,
        )
            .then((body) => {
                if (controller.signal.aborted) return;
                loadedSegmentsRef.current.add(activeSegment);
                dispatch(loadedAction(activeSegment, body));
            })
            .catch(() => {
                if (controller.signal.aborted) return;
                dispatch({ type: 'error', segment: activeSegment });
            });
        return () => {
            controller.abort();
        };
    }, [activeSegment, handleOrDid]);

    useEffect(() => {
        const controllers = loadMoreControllers.current;
        return () => {
            controllers.forEach((controller) => controller.abort());
            controllers.clear();
        };
    }, []);

    const onLoadMore = (segment: Segment) => {
        const current = lists[segment];
        if (
            !current.nextCursor ||
            current.status === 'loadingMore' ||
            loadMoreControllers.current.has(segment)
        ) {
            return;
        }
        const controller = new AbortController();
        loadMoreControllers.current.set(segment, controller);
        dispatch({ type: 'loadMore', segment });
        fetchSegmentPage(
            handleOrDid,
            segment,
            current.nextCursor,
            controller.signal,
        )
            .then((body) => {
                if (controller.signal.aborted) return;
                dispatch(appendAction(segment, body));
            })
            .catch((err) => {
                if (controller.signal.aborted) return;
                dispatch({ type: 'error', segment });
                toastMutationError(err, "Couldn't load more. Try again.");
            })
            .finally(() => {
                if (loadMoreControllers.current.get(segment) === controller) {
                    loadMoreControllers.current.delete(segment);
                }
            });
    };

    return (
        <>
            <main className="mx-auto w-full max-w-2xl px-4 pt-16 pb-12 sm:px-6">
                <ProfilePageHeader
                    profile={profile}
                    onFollowChanged={onFollowChanged}
                />

                <div className="mb-6">
                    <Subnav
                        items={segments.map((segment) => ({
                            id: segment,
                            label: SEGMENT_LABEL[segment],
                        }))}
                        activeId={activeSegment}
                        onSelect={(id) => setActiveSegment(id as Segment)}
                        ariaLabel="Profile sections"
                    />
                </div>

                <div className="flex flex-col gap-4">
                    <SegmentContent
                        key={activeSegment}
                        segment={activeSegment}
                        state={lists[activeSegment]}
                        emptyCopy={EMPTY_SEGMENT_COPY[activeSegment]}
                        subscribedKeys={subscribe.subscribedKeys}
                        onSubscribe={subscribe.onSubscribe}
                    />
                    <LoadMoreFooter
                        state={lists[activeSegment]}
                        onLoadMore={() => onLoadMore(activeSegment)}
                    />
                </div>
            </main>
            <SubscribeDialog
                source={subscribe.dialogSource}
                open={subscribe.dialogOpen}
                onOpenChange={subscribe.onDialogOpenChange}
                onSubscribed={subscribe.onSubscribed}
            />
        </>
    );
}

function ProfilePageHeader({
    profile,
    onFollowChanged,
}: {
    profile: ProfileHeader;
    onFollowChanged: (followRkey: string | null) => void;
}) {
    return (
        <header className="mb-10 flex flex-col gap-4">
            <div className="relative flex items-center gap-3 font-sans">
                <BackButton />
                <Avatar size="lg">
                    {profile.avatar ? (
                        <AvatarImage src={safeHref(profile.avatar)} alt="" />
                    ) : null}
                    <AvatarFallback>
                        {initialsFromHandle(profile.handle, profile.did)}
                    </AvatarFallback>
                </Avatar>
                <div className="min-w-0 flex-1">
                    <h1 className="truncate text-2xl font-medium tracking-tight text-balance text-foreground">
                        {profileDisplayName(profile)}
                    </h1>
                    <p className="truncate text-xs font-light text-muted-foreground">
                        @{profile.handle}
                    </p>
                </div>
                {!profile.isSelf ? (
                    <FollowButton
                        handle={profile.handle}
                        followRkey={profile.followRkey}
                        onChange={onFollowChanged}
                    />
                ) : null}
            </div>
            {profile.description ? (
                <p className="text-sm font-light text-muted-foreground">
                    {profile.description}
                </p>
            ) : null}
            <p className="text-sm text-muted-foreground">
                {metaLine(profile.counts)}
            </p>
        </header>
    );
}

function SegmentContent({
    segment,
    state,
    emptyCopy,
    subscribedKeys,
    onSubscribe,
}: {
    segment: Segment;
    state: SegmentState;
    emptyCopy: string;
    subscribedKeys: ReadonlySet<string>;
    onSubscribe: (source: DiscoverSourceCard) => void;
}) {
    const fallback = segmentFallback(state, emptyCopy);
    if (fallback) return <ProfileCard>{fallback}</ProfileCard>;
    return (
        <LoadedSegmentContent
            segment={segment}
            state={state}
            subscribedKeys={subscribedKeys}
            onSubscribe={onSubscribe}
        />
    );
}

function segmentFallback(
    state: SegmentState,
    emptyCopy: string,
): ReactNode | null {
    if (state.status === 'loading') {
        return <SegmentSkeleton />;
    }
    if (state.status === 'error') {
        return <SegmentNote text="Couldn't load this." />;
    }
    if (state.items.length === 0) {
        return <SegmentNote text={emptyCopy} />;
    }
    return null;
}

function LoadedSegmentContent({
    segment,
    state,
    subscribedKeys,
    onSubscribe,
}: {
    segment: Segment;
    state: SegmentState;
    subscribedKeys: ReadonlySet<string>;
    onSubscribe: (source: DiscoverSourceCard) => void;
}) {
    if (segment !== 'shares') {
        return (
            <ProfileSourceCards
                sources={sourceItems(state.items)}
                subscribedKeys={subscribedKeys}
                onSubscribe={onSubscribe}
            />
        );
    }
    return <ProfileShares shares={shareItems(state.items)} />;
}

function ProfileShares({ shares }: { shares: ProfileShareItem[] }) {
    return (
        <ProfileCard>
            <ul className="flex list-none flex-col">
                {shares.map((share, index) => (
                    <li key={profileItemKey(share)}>
                        {index > 0 ? (
                            <div
                                aria-hidden
                                className="mx-6 border-t border-border"
                            />
                        ) : null}
                        <ShareRow share={share} />
                    </li>
                ))}
            </ul>
        </ProfileCard>
    );
}

function ProfileCard({ children }: { children: ReactNode }) {
    return (
        <article className="overflow-hidden rounded-xl bg-card shadow-card">
            {children}
        </article>
    );
}

function SegmentNote({ text }: { text: string }) {
    return (
        <p className="px-6 py-8 text-center text-sm font-light text-muted-foreground">
            {text}
        </p>
    );
}

function isSourceItem(item: ProfileListItem): item is PersonPreviewSource {
    return !isShareItem(item);
}

function sourceItems(items: ProfileListItem[]): PersonPreviewSource[] {
    const sources: PersonPreviewSource[] = [];
    for (const item of items) {
        if (isSourceItem(item)) sources.push(item);
    }
    return sources;
}

function shareItems(items: ProfileListItem[]): ProfileShareItem[] {
    const shares: ProfileShareItem[] = [];
    for (const item of items) {
        if (isShareItem(item)) shares.push(item);
    }
    return shares;
}

function ProfileSourceCards({
    sources,
    subscribedKeys,
    onSubscribe,
}: {
    sources: PersonPreviewSource[];
    subscribedKeys: ReadonlySet<string>;
    onSubscribe: (source: DiscoverSourceCard) => void;
}) {
    const [expandedKeys, setExpandedKeys] = useState<ReadonlySet<string>>(
        () => new Set(),
    );
    const [posts, setPosts] = useState<
        Record<string, PostsState | undefined>
    >({});
    const postControllers = useRef<Map<string, AbortController>>(new Map());
    const cut = useDiscoverCut({ sources, expandedKeys, setExpandedKeys });
    const segments = partitionSourceSegments(sources, expandedKeys);

    useEffect(() => {
        const controllers = postControllers.current;
        return () => {
            controllers.forEach((controller) => controller.abort());
            controllers.clear();
        };
    }, []);

    const onToggle = (key: string) => {
        const opening = cut.toggle(key);
        if (!opening || posts[key] || postControllers.current.has(key)) return;

        const controller = new AbortController();
        postControllers.current.set(key, controller);
        setPosts((current) => ({
            ...current,
            [key]: { status: 'loading' },
        }));
        api<DiscoverSourcePost[] | null>(
            `/api/discover/sources/posts?key=${encodeURIComponent(key)}`,
            { signal: controller.signal },
        )
            .catch(() => null)
            .then((result) => {
                if (controller.signal.aborted) return;
                setPosts((current) => ({
                    ...current,
                    [key]: {
                        status: 'loaded',
                        posts: result ?? [],
                    },
                }));
            })
            .finally(() => {
                if (postControllers.current.get(key) === controller) {
                    postControllers.current.delete(key);
                }
            });
    };

    return (
        <CutSegmentStack
            cut={cut}
            segments={segments}
            renderRow={(source, row) => (
                <SourceCardRow
                    key={source.key}
                    source={source}
                    postsState={posts[source.key]}
                    expanded={expandedKeys.has(source.key)}
                    intentExpanded={row.intentExpanded}
                    contentState={row.contentState}
                    onToggle={() => onToggle(source.key)}
                    showDivider={row.showDivider}
                    dividerState={row.dividerState}
                    subscribed={
                        source.subscribed || subscribedKeys.has(source.key)
                    }
                    onSubscribe={() => onSubscribe(source)}
                />
            )}
        />
    );
}

function LoadMoreFooter({
    state,
    onLoadMore,
}: {
    state: SegmentState;
    onLoadMore: () => void;
}) {
    if (!state.nextCursor) return null;
    return (
        <DiscoverLoadMoreFooter
            loading={state.status === 'loadingMore'}
            onLoadMore={onLoadMore}
        />
    );
}

function FollowButton({
    handle,
    followRkey,
    onChange,
}: {
    handle: string;
    followRkey: string | null;
    onChange: (followRkey: string | null) => void;
}) {
    const [pending, setPending] = useState(false);

    // Both directions flip optimistically; PENDING_FOLLOW keeps the placeholder rkey un-deletable.
    const onFollow = async () => {
        if (pending || followRkey) return;
        setPending(true);
        onChange(PENDING_FOLLOW);
        onChange(await requestFollow(handle));
        setPending(false);
    };

    const onUnfollow = async () => {
        if (pending || !canUnfollow(followRkey)) return;
        setPending(true);
        onChange(null);
        if (!(await requestUnfollow(followRkey))) onChange(followRkey);
        setPending(false);
    };

    if (followRkey) {
        return (
            <Button
                variant="secondary"
                size="sm"
                onClick={onUnfollow}
                disabled={pending}
            >
                Unfollow
            </Button>
        );
    }
    return (
        <Button size="sm" onClick={onFollow} disabled={pending}>
            Follow
        </Button>
    );
}

function ShareRow({ share }: { share: ProfileShareItem }) {
    const target = shareTargetPresentation(share);

    return (
        <div className="px-6 py-4">
            <div className="flex items-baseline gap-4">
                <ProfileShareTitle target={target} />
                <span className="shrink-0 text-xs font-light text-muted-foreground tabular-nums">
                    {shortTimeAgo(share.createdAt)}
                </span>
            </div>
            {share.comment ? (
                <p className="mt-1 line-clamp-2 text-sm font-light text-muted-foreground">
                    {share.comment}
                </p>
            ) : null}
        </div>
    );
}

function ProfileShareTitle({
    target,
}: {
    target: ShareTargetPresentation;
}) {
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

function ProfileSkeleton() {
    return (
        <main
            aria-busy
            aria-label="Loading profile"
            className="mx-auto w-full max-w-2xl px-4 pt-16 pb-12 sm:px-6"
        >
            <header className="mb-10 flex flex-col gap-4">
                <div className="relative flex items-center gap-3">
                    <BackButton />
                    <Skeleton className="size-10 rounded-full" />
                    <div className="flex min-w-0 flex-1 flex-col gap-1.5">
                        <Skeleton className="h-6 w-1/2" />
                        <Skeleton className="h-3 w-32" />
                    </div>
                </div>
                <Skeleton className="h-3 w-40" />
            </header>
            <div className="mb-6 flex items-center justify-center gap-5">
                <Skeleton className="h-4 w-14" />
                <Skeleton className="h-4 w-14" />
            </div>
            <article className="overflow-hidden rounded-xl bg-card shadow-card">
                <SegmentSkeleton />
            </article>
        </main>
    );
}

function SegmentSkeleton() {
    return (
        <ul aria-busy aria-label="Loading" className="flex list-none flex-col">
            {Array.from({ length: 3 }).map((_, index) => (
                <li key={index}>
                    {index > 0 ? (
                        <div
                            aria-hidden
                            className="mx-6 border-t border-border"
                        />
                    ) : null}
                    <div className="flex items-center gap-3 px-6 py-4">
                        <Skeleton className="size-8 rounded-lg" />
                        <div className="flex min-w-0 flex-1 flex-col gap-1.5">
                            <Skeleton className="h-4 w-2/3" />
                            <Skeleton className="h-3 w-1/3" />
                        </div>
                    </div>
                </li>
            ))}
        </ul>
    );
}

function BackButton() {
    const goBackOr = useGoBackOr();
    return (
        <a
            href={PATHS.discover}
            aria-label="Back"
            onClick={(e) => {
                if (!isPlainLeftClick(e)) return;
                e.preventDefault();
                goBackOr(PATHS.discover);
            }}
            className="absolute top-1/2 right-full mr-2 inline-flex size-9 -translate-y-1/2 items-center justify-center rounded-xl text-muted-foreground transition-colors duration-200 ease-out outline-none hover:text-foreground focus-visible:outline-1 focus-visible:outline-offset-2 focus-visible:outline-ring focus-visible:outline-solid"
        >
            <ArrowLeftIcon className="size-5" />
        </a>
    );
}
