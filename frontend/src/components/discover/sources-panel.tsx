import type { Dispatch, SetStateAction } from 'react';
import { useEffect, useRef, useState } from 'react';

import { CardMasthead } from '@/components/card-masthead';
import { CutSegmentStack } from '@/components/discover/cut-segment-stack';
import { DiscoverLoadMoreFooter } from '@/components/discover/discover-load-more-footer';
import { DiscoverSourceRow } from '@/components/discover/discover-source-row';
import { DiscoverStackSkeleton } from '@/components/discover/discover-stack-skeleton';
import { SubscribeDialog } from '@/components/discover/subscribe-dialog';
import { Skeleton } from '@/components/ui/skeleton';
import { useDiscoverCut } from '@/hooks/use-discover-cut';
import { useDiscoverSourceActions } from '@/hooks/use-discover-source-actions';
import { api } from '@/lib/api';
import {
    readDiscoverCache,
    writeCachedPost,
    writeCachedProfiles,
    writeCachedSources,
    writeDiscoverCache,
} from '@/lib/discover-cache';
import {
    reduceDiscoverPage,
    type DiscoverPage,
    type DiscoverPageAction,
} from '@/lib/discover-page';
import { loadMoreDiscoverItems } from '@/lib/discover-load-more';
import {
    discoverReasonDids,
    resolveDiscoverReasonDisplay,
    type DiscoverSource,
    type DiscoverSourcePost,
    type PostsState,
} from '@/lib/discover';
import { partitionSourceSegments } from '@/lib/discover-segments';
import { countLabel } from '@/lib/plural';
import { fetchProfiles, type Profile } from '@/lib/profile';

type State =
    | { kind: 'loading' }
    | {
          kind: 'ok';
          sources: DiscoverSource[];
          nextCursor?: string;
          loadingMore: boolean;
      }
    | { kind: 'error' };

type Profiles = Record<string, Profile | undefined>;
type PostsByKey = Record<string, PostsState | undefined>;

async function fetchSources(cursor?: string): Promise<DiscoverPage<DiscoverSource>> {
    // A cold-cache crawl can outlive the server's write timeout; time out to the error state instead of an indefinite skeleton.
    return (
        (await api<DiscoverPage<DiscoverSource> | null>(
            cursor
                ? `/api/discover/sources?cursor=${encodeURIComponent(cursor)}`
                : '/api/discover/sources',
            {
                signal: AbortSignal.timeout(15_000),
            },
        )) ?? { items: [] }
    );
}

async function loadSources(
    isCancelled: () => boolean,
    setState: Dispatch<SetStateAction<State>>,
    setProfiles: Dispatch<SetStateAction<Profiles>>,
) {
    try {
        const page = await fetchSources();
        if (isCancelled()) return;
        setState({
            kind: 'ok',
            sources: page.items,
            nextCursor: page.nextCursor,
            loadingMore: false,
        });
        writeDiscoverCache(page.items, {}, page.nextCursor);
        await loadProfiles(page.items, isCancelled, setProfiles);
    } catch {
        if (!isCancelled()) setState({ kind: 'error' });
    }
}

// Every card's reason (author/top-follower/follower list) feeds one deduped lookup pass.
async function loadProfiles(
    sources: DiscoverSource[],
    isCancelled: () => boolean,
    setProfiles: Dispatch<SetStateAction<Profiles>>,
) {
    const profiles = await fetchProfiles(
        sources.flatMap((s) => discoverReasonDids(s.reason)),
    );
    writeCachedProfiles(profiles);
    if (isCancelled()) return;
    setProfiles(profiles);
}

function updateSourcesPage(
    setState: Dispatch<SetStateAction<State>>,
    action: DiscoverPageAction<DiscoverSource>,
) {
    setState((prev) => {
        if (prev.kind !== 'ok') return prev;
        const next = reduceDiscoverPage(
            {
                items: prev.sources,
                nextCursor: prev.nextCursor,
                loadingMore: prev.loadingMore,
            },
            action,
        );
        return {
            kind: 'ok',
            sources: next.items,
            nextCursor: next.nextCursor,
            loadingMore: next.loadingMore,
        };
    });
}

async function hydrateSourceProfiles(
    sources: DiscoverSource[],
    isCancelled: () => boolean,
    setProfiles: Dispatch<SetStateAction<Profiles>>,
) {
    const resolved = await fetchProfiles(
        sources.flatMap((source) => discoverReasonDids(source.reason)),
    );
    writeCachedProfiles(resolved);
    if (isCancelled()) return;
    setProfiles((prev) => ({ ...prev, ...resolved }));
}

// Best-effort: a failed fetch renders as the empty state rather than blocking the card.
async function loadSourcePosts(
    key: string,
    isCancelled: () => boolean,
    setPosts: Dispatch<SetStateAction<PostsByKey>>,
) {
    setPosts((prev) => ({ ...prev, [key]: { status: 'loading' } }));
    const result = await api<DiscoverSourcePost[] | null>(
        `/api/discover/sources/posts?key=${encodeURIComponent(key)}`,
    ).catch(() => null);
    if (isCancelled()) return;
    const posts = result ?? [];
    writeCachedPost(key, posts);
    setPosts((prev) => ({ ...prev, [key]: { status: 'loaded', posts } }));
}

function seedPostsState(
    posts: Record<string, DiscoverSourcePost[]> | undefined,
): PostsByKey {
    if (!posts) return {};
    return Object.fromEntries(
        Object.entries(posts).map(([key, list]) => [
            key,
            { status: 'loaded', posts: list } as const,
        ]),
    );
}

function sourceCards(state: State): DiscoverSource[] {
    return state.kind === 'ok' ? state.sources : [];
}

// SourcesPanel: SPEC <discovery> unified sources list, personal cards then trending-only; cold start still shows trending.
export function SourcesPanel() {
    const [state, setState] = useState<State>(() => {
        const cached = readDiscoverCache();
        return cached
            ? {
                  kind: 'ok',
                  sources: cached.sources,
                  nextCursor: cached.nextCursor,
                  loadingMore: false,
              }
            : { kind: 'loading' };
    });
    const [profiles, setProfiles] = useState<Profiles>(
        () => readDiscoverCache()?.profiles ?? {},
    );
    const [posts, setPosts] = useState<PostsByKey>(() =>
        seedPostsState(readDiscoverCache()?.posts),
    );
    const [expandedKeys, setExpandedKeys] = useState<ReadonlySet<string>>(
        () => new Set(),
    );
    const cancelledRef = useRef(false);
    const sources = sourceCards(state);
    const cut = useDiscoverCut({
        sources,
        expandedKeys,
        setExpandedKeys,
    });
    const {
        dialogSource,
        dialogOpen,
        onDialogOpenChange,
        onSubscribe,
        onSubscribed,
        isSubscribed,
        onHide,
    } = useDiscoverSourceActions(state, setState);

    useEffect(() => {
        // Reset on every run: StrictMode's dev remount would otherwise leave the ref poisoned true.
        cancelledRef.current = false;
        if (readDiscoverCache()) return;
        void loadSources(() => cancelledRef.current, setState, setProfiles);
        return () => {
            cancelledRef.current = true;
        };
    }, []);

    // Write-through keeps the cache in sync with hide/rollback without owning state itself.
    useEffect(() => {
        if (state.kind === 'ok') {
            writeCachedSources(state.sources, state.nextCursor);
        }
    }, [state]);

    const onLoadMore = () => {
        if (state.kind !== 'ok') return;
        loadMoreDiscoverItems({
            cursor: state.nextCursor,
            loading: state.loadingMore,
            items: state.sources,
            keyOfItem: (source) => source.key,
            keyOfWire: (source) => source.key,
            fetchPage: fetchSources,
            start: () => updateSourcesPage(setState, { type: 'loadMore' }),
            append: (page) =>
                updateSourcesPage(setState, { type: 'append', page }),
            fail: () => updateSourcesPage(setState, { type: 'failed' }),
            hydrate: (sources) =>
                hydrateSourceProfiles(
                    sources,
                    () => cancelledRef.current,
                    setProfiles,
                ),
            cancelled: () => cancelledRef.current,
        });
    };

    const onToggleExpanded = (key: string) => {
        const opening = cut.toggle(key);
        if (opening && !posts[key]) {
            void loadSourcePosts(key, () => cancelledRef.current, setPosts);
        }
    };

    // A hidden source loses its expanded flag so re-suggesting it later reopens collapsed.
    const onHideSource = (source: DiscoverSource) => {
        cut.settle();
        setExpandedKeys((prev) => {
            if (!prev.has(source.key)) return prev;
            const next = new Set(prev);
            next.delete(source.key);
            return next;
        });
        void onHide(source);
    };

    if (state.kind !== 'ok') return <SourcesPanelFallback kind={state.kind} />;
    if (state.sources.length === 0) return null;

    const segments = partitionSourceSegments(state.sources, expandedKeys);
    const meta = countLabel(state.sources.length, 'source', 'sources');

    return (
        <>
            <div className="flex flex-col gap-4">
                <CutSegmentStack
                    cut={cut}
                    segments={segments}
                    masthead={
                        <CardMasthead
                            eyebrow="Discover"
                            heading="Worth adding to your reading"
                            meta={meta}
                        />
                    }
                    renderRow={(source, row) => (
                        <DiscoverSourceRow
                            key={source.key}
                            source={source}
                            postsState={posts[source.key]}
                            expanded={expandedKeys.has(source.key)}
                            intentExpanded={row.intentExpanded}
                            contentState={row.contentState}
                            onToggle={() => onToggleExpanded(source.key)}
                            showDivider={row.showDivider}
                            dividerState={row.dividerState}
                            reasonDisplay={resolveDiscoverReasonDisplay(
                                source.reason,
                            )}
                            profiles={profiles}
                            subscribed={isSubscribed(source.key)}
                            onSubscribe={() => onSubscribe(source)}
                            onHide={() => onHideSource(source)}
                        />
                    )}
                />
                <SourcesLoadMore state={state} onLoadMore={onLoadMore} />
            </div>
            <SubscribeDialog
                source={dialogSource}
                open={dialogOpen}
                onOpenChange={onDialogOpenChange}
                onSubscribed={onSubscribed}
            />
        </>
    );
}

function SourcesLoadMore({
    state,
    onLoadMore,
}: {
    state: Extract<State, { kind: 'ok' }>;
    onLoadMore: () => void;
}) {
    if (!state.nextCursor) return null;
    return (
        <DiscoverLoadMoreFooter
            loading={state.loadingMore}
            onLoadMore={onLoadMore}
        />
    );
}

function SourcesPanelFallback({ kind }: { kind: 'loading' | 'error' }) {
    return kind === 'loading' ? <SourcesPanelSkeleton /> : <SourcesPanelError />;
}

function SourcesPanelSkeleton() {
    return (
        <DiscoverStackSkeleton
            label="Loading sources"
            row={
                <>
                    <div className="flex items-start gap-3">
                        <Skeleton className="size-[34px] shrink-0 rounded-lg" />
                        <Skeleton className="h-5 flex-1 rounded-md" />
                    </div>
                    <div className="mt-4 flex items-center gap-2 pt-3">
                        <Skeleton className="h-4 w-40" />
                    </div>
                </>
            }
        />
    );
}

function SourcesPanelError() {
    return (
        <p className="text-sm font-light text-muted-foreground">
            Couldn't load your sources. Try refreshing in a moment.
        </p>
    );
}
