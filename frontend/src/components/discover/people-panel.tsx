import type { Dispatch, ReactNode, SetStateAction } from 'react';
import { useEffect, useReducer, useRef, useState } from 'react';
import { AnimatePresence, motion, useReducedMotion } from 'motion/react';

import { CardMasthead } from '@/components/card-masthead';
import { CutSegmentStack } from '@/components/discover/cut-segment-stack';
import { DiscoverLoadMoreFooter } from '@/components/discover/discover-load-more-footer';
import { DiscoverPersonRow } from '@/components/discover/discover-person-row';
import { DiscoverStackSkeleton } from '@/components/discover/discover-stack-skeleton';
import { PersonSearch } from '@/components/discover/person-search';
import { SubscribeDialog } from '@/components/discover/subscribe-dialog';
import { Skeleton } from '@/components/ui/skeleton';
import {
    useDiscoverCut,
    type DiscoverCut,
} from '@/hooks/use-discover-cut';
import { useAuthedMe } from '@/hooks/use-authed-me';
import { useDiscoverPersonActions } from '@/hooks/use-discover-person-actions';
import { useSubscribeTarget } from '@/hooks/use-subscribe-target';
import { api } from '@/lib/api';
import { type DiscoverSourceCard } from '@/lib/discover';
import {
    peopleProfileDids,
    resolvePersonReasonDisplay,
    settlePeopleLoad,
    toFollowCard,
    toPersonCard,
    type DiscoverPerson,
    type FollowCard,
    type FollowListState,
    type PersonCard,
    type PeopleLoad,
    type PersonPreview,
    type PersonPreviewSource,
    type PersonSuggestionState,
    type PreviewState,
} from '@/lib/discover-people';
import {
    readPeopleCache,
    writeCachedFollows,
    writeCachedPeople,
    writeCachedPeopleProfiles,
    writeCachedPersonPreview,
    writePeopleCache,
} from '@/lib/discover-people-cache';
import {
    reduceDiscoverPage,
    type DiscoverPage,
    type DiscoverPageAction,
} from '@/lib/discover-page';
import { loadMoreDiscoverItems } from '@/lib/discover-load-more';
import {
    partitionSourceSegments,
    type SourceSegment,
} from '@/lib/discover-segments';
import { type FollowRecord } from '@/lib/follow';
import { splitFade, splitOpenFade } from '@/lib/motion-transitions';
import {
    EMPTY_SEARCH_SLOT,
    personSearchSlotReducer,
    searchResultToCard,
    searchResultToProfile,
    type PersonSearchResult,
} from '@/lib/person-search';
import { countLabel } from '@/lib/plural';
import { fetchProfiles, type Profile } from '@/lib/profile';
import { withoutKey } from '@/lib/utils';

type Profiles = Record<string, Profile | undefined>;
type PreviewsByDid = Record<string, PreviewState | undefined>;

function personSourceToCard(source: PersonPreviewSource): DiscoverSourceCard {
    return {
        key: source.key,
        kind: source.kind,
        title: source.title,
        siteUrl: source.siteUrl,
        feedUrl: source.feedUrl,
        publication: source.publication,
    };
}

// Suggestions and follows degrade independently; the cache is only seeded when both loaded cleanly.
async function loadPeople(
    isCancelled: () => boolean,
    setSuggest: Dispatch<SetStateAction<PersonSuggestionState>>,
    setFollow: Dispatch<SetStateAction<FollowListState>>,
    setProfiles: Dispatch<SetStateAction<Profiles>>,
) {
    const [peopleResult, followsResult] = await Promise.allSettled([
        api<DiscoverPage<DiscoverPerson> | null>('/api/discover/people'),
        api<FollowRecord[] | null>('/api/follows'),
    ]);
    if (isCancelled()) return;

    const load = settlePeopleLoad(peopleResult, followsResult);
    setSuggest(load.suggest);
    setFollow(load.follow);
    cachePeopleLoad(load);

    const profiles = await fetchProfiles(peopleProfileDids(load.people, load.follows));
    cachePeopleLoadProfiles(load, profiles);
    if (isCancelled()) return;
    setProfiles(profiles);
}

function cachePeopleLoad(load: PeopleLoad) {
    if (!load.complete) return;
    writePeopleCache(load.people, load.follows, {}, load.nextCursor);
}

function cachePeopleLoadProfiles(load: PeopleLoad, profiles: Profiles) {
    if (load.complete) writeCachedPeopleProfiles(profiles);
}

// Best-effort: a failed fetch renders as the empty preview rather than blocking the card.
async function loadPersonPreview(
    did: string,
    isCancelled: () => boolean,
    setPreviews: Dispatch<SetStateAction<PreviewsByDid>>,
) {
    setPreviews((prev) => ({ ...prev, [did]: { status: 'loading' } }));
    const result = await api<PersonPreview | null>(
        `/api/discover/people/preview?did=${encodeURIComponent(did)}`,
    ).catch(() => null);
    if (isCancelled()) return;
    const preview = result ?? { writes: [], reads: [], latestShare: null };
    writeCachedPersonPreview(did, preview);
    setPreviews((prev) => ({ ...prev, [did]: { status: 'loaded', preview } }));
}

function seedPreviews(
    previews: Record<string, PersonPreview> | undefined,
): PreviewsByDid {
    if (!previews) return {};
    return Object.fromEntries(
        Object.entries(previews).map(([did, preview]) => [
            did,
            { status: 'loaded', preview } as const,
        ]),
    );
}

function suggestionCards(state: PersonSuggestionState): PersonCard[] {
    return state.kind === 'ok' ? state.people : [];
}

function followingCards(state: FollowListState): FollowCard[] {
    return state.kind === 'ok' ? state.follows : [];
}

type RemovedFollow = {
    index: number;
    record: FollowCard;
    wasSessionFollow: boolean;
};

async function fetchPeople(cursor: string): Promise<DiscoverPage<DiscoverPerson>> {
    return (
        (await api<DiscoverPage<DiscoverPerson> | null>(
            `/api/discover/people?cursor=${encodeURIComponent(cursor)}`,
        )) ?? { items: [] }
    );
}

function updatePeoplePage(
    setSuggest: Dispatch<SetStateAction<PersonSuggestionState>>,
    action: DiscoverPageAction<PersonCard>,
) {
    setSuggest((prev) => {
        if (prev.kind !== 'ok') return prev;
        const next = reduceDiscoverPage(
            {
                items: prev.people,
                nextCursor: prev.nextCursor,
                loadingMore: prev.loadingMore,
            },
            action,
        );
        return {
            kind: 'ok',
            people: next.items,
            nextCursor: next.nextCursor,
            loadingMore: next.loadingMore,
        };
    });
}

function toPersonCardPage(
    page: DiscoverPage<DiscoverPerson>,
): DiscoverPage<PersonCard> {
    return {
        items: page.items.map(toPersonCard),
        nextCursor: page.nextCursor,
    };
}

async function hydratePeopleProfiles(
    people: DiscoverPerson[],
    isCancelled: () => boolean,
    setProfiles: Dispatch<SetStateAction<Profiles>>,
) {
    const resolved = await fetchProfiles(peopleProfileDids(people, []));
    writeCachedPeopleProfiles(resolved);
    if (isCancelled()) return;
    setProfiles((prev) => ({ ...prev, ...resolved }));
}

function usePeoplePanelState() {
    const [suggestState, setSuggestState] = useState<PersonSuggestionState>(
        () => {
            const cached = readPeopleCache();
            return cached
                ? {
                      kind: 'ok',
                      people: cached.people.map(toPersonCard),
                      nextCursor: cached.nextCursor,
                      loadingMore: false,
                  }
                : { kind: 'loading' };
        },
    );
    const [followState, setFollowState] = useState<FollowListState>(() => {
        const cached = readPeopleCache();
        return cached
            ? { kind: 'ok', follows: cached.follows.map(toFollowCard) }
            : { kind: 'loading' };
    });
    const [profiles, setProfiles] = useState<Profiles>(
        () => readPeopleCache()?.profiles ?? {},
    );
    const [previews, setPreviews] = useState<PreviewsByDid>(() =>
        seedPreviews(readPeopleCache()?.previews),
    );
    const [followedDids, setFollowedDids] = useState<ReadonlySet<string>>(
        () => new Set(),
    );
    const [expandedKeys, setExpandedKeys] = useState<ReadonlySet<string>>(
        () => new Set(),
    );
    const [followExpandedKeys, setFollowExpandedKeys] = useState<
        ReadonlySet<string>
    >(() => new Set());
    const [followingDid, setFollowingDid] = useState<string | undefined>(
        undefined,
    );
    const [searchSlot, dispatchSearchSlot] = useReducer(
        personSearchSlotReducer,
        EMPTY_SEARCH_SLOT,
    );
    const cancelledRef = useRef(false);

    return {
        suggestState,
        setSuggestState,
        followState,
        setFollowState,
        profiles,
        setProfiles,
        previews,
        setPreviews,
        followedDids,
        setFollowedDids,
        expandedKeys,
        setExpandedKeys,
        followExpandedKeys,
        setFollowExpandedKeys,
        followingDid,
        setFollowingDid,
        searchSlot,
        dispatchSearchSlot,
        cancelledRef,
    };
}

// PeoplePanel: SPEC <discovery> People — unified suggestions (personal then trending-only) then the
// follow list, both cut cards on the sources grammar. The follow list is the only unfollow surface.
export function PeoplePanel() {
    const me = useAuthedMe();
    const {
        suggestState,
        setSuggestState,
        followState,
        setFollowState,
        profiles,
        setProfiles,
        previews,
        setPreviews,
        followedDids,
        setFollowedDids,
        expandedKeys,
        setExpandedKeys,
        followExpandedKeys,
        setFollowExpandedKeys,
        followingDid,
        setFollowingDid,
        searchSlot,
        dispatchSearchSlot,
        cancelledRef,
    } = usePeoplePanelState();
    const dialog = useSubscribeTarget<DiscoverSourceCard>();

    const suggestPeople = suggestionCards(suggestState);
    const followCards = followingCards(followState);

    const suggestCut = useDiscoverCut({
        sources: suggestPeople,
        expandedKeys,
        setExpandedKeys,
    });
    const followCut = useDiscoverCut({
        sources: followCards,
        expandedKeys: followExpandedKeys,
        setExpandedKeys: setFollowExpandedKeys,
    });

    // Following marks the suggestion inert in place and seeds the follow list; the profile is already resolved.
    const onFollowed = (did: string, record: FollowRecord) => {
        setFollowedDids((prev) => new Set(prev).add(did));
        setFollowState((prev) => {
            const follows = prev.kind === 'ok' ? prev.follows : [];
            const withoutExisting = follows.filter((f) => f.rkey !== record.rkey);
            return { kind: 'ok', follows: [toFollowCard(record), ...withoutExisting] };
        });
    };

    const suggestionActions = useDiscoverPersonActions(
        suggestState,
        setSuggestState,
        { followingDid, setFollowingDid, onFollowed },
    );

    // SPEC <discovery> line 558: the search endpoint already carries profile fields, so the slot
    // decorates immediately; a presence-less result has nothing to fetch (PreviewBody renders the
    // honest empty state straight from `presenceless`, no request needed).
    const onSearchSelect = (result: PersonSearchResult) => {
        dispatchSearchSlot({ type: 'select', result });
        setProfiles((prev) => ({ ...prev, [result.did]: searchResultToProfile(result) }));
        if (result.inReaderNetwork) {
            void loadPersonPreview(result.did, () => cancelledRef.current, setPreviews);
        }
    };

    const onSearchQueryChange = () => dispatchSearchSlot({ type: 'queryChanged' });

    // Following from the slot is a suggestion follow in every way but where it started: same
    // in-flight gate, same list prepend, same inert flip in place.
    const onFollowSlot = () => {
        if (!searchSlot.slot || searchSlot.slot.did === me.did) return;
        suggestionActions.onFollow(searchResultToCard(searchSlot.slot), searchSlot.slot.handle);
    };

    useEffect(() => {
        // Reset on every run: StrictMode's dev remount would otherwise leave the ref poisoned true.
        cancelledRef.current = false;
        if (readPeopleCache()) return;
        void loadPeople(
            () => cancelledRef.current,
            setSuggestState,
            setFollowState,
            setProfiles,
        );
        return () => {
            cancelledRef.current = true;
        };
    }, [cancelledRef, setFollowState, setProfiles, setSuggestState]);

    // Write-through keeps the cache in sync with follow/hide/rollback without owning state itself.
    useEffect(() => {
        if (suggestState.kind === 'ok') {
            writeCachedPeople(
                suggestState.people,
                suggestState.nextCursor,
            );
        }
    }, [suggestState]);
    useEffect(() => {
        if (followState.kind === 'ok') writeCachedFollows(followState.follows);
    }, [followState]);

    const onToggleSuggest = (key: string) => {
        const opening = suggestCut.toggle(key);
        if (opening && !previews[key]) {
            void loadPersonPreview(key, () => cancelledRef.current, setPreviews);
        }
    };

    const onLoadMore = () => {
        if (suggestState.kind !== 'ok') return;
        loadMoreDiscoverItems({
            cursor: suggestState.nextCursor,
            loading: suggestState.loadingMore,
            items: suggestState.people,
            keyOfItem: (person) => person.did,
            keyOfWire: (person) => person.did,
            fetchPage: fetchPeople,
            start: () =>
                updatePeoplePage(setSuggestState, { type: 'loadMore' }),
            append: (page) =>
                updatePeoplePage(setSuggestState, {
                    type: 'append',
                    page: toPersonCardPage(page),
                }),
            fail: () => updatePeoplePage(setSuggestState, { type: 'failed' }),
            hydrate: (people) =>
                hydratePeopleProfiles(
                    people,
                    () => cancelledRef.current,
                    setProfiles,
                ),
            cancelled: () => cancelledRef.current,
        });
    };

    const onToggleFollow = (card: FollowCard) => {
        const opening = followCut.toggle(card.key);
        if (opening && !previews[card.subjectDid]) {
            void loadPersonPreview(
                card.subjectDid,
                () => cancelledRef.current,
                setPreviews,
            );
        }
    };

    // A hidden suggestion loses its expanded flag so re-suggesting it later reopens collapsed.
    const onHideSuggest = (person: PersonCard) => {
        suggestCut.settle();
        setExpandedKeys((prev) => withoutKey(prev, person.key));
        void suggestionActions.onHide(person);
    };

    const removeFollow = (card: FollowCard): RemovedFollow | undefined => {
        if (followState.kind !== 'ok') return undefined;
        const index = followState.follows.findIndex((f) => f.rkey === card.rkey);
        if (index === -1) return undefined;
        const removed: RemovedFollow = {
            index,
            record: followState.follows[index],
            wasSessionFollow: followedDids.has(card.subjectDid),
        };

        followCut.settle();
        setFollowExpandedKeys((prev) => withoutKey(prev, card.key));
        setFollowState((prev) =>
            prev.kind === 'ok'
                ? {
                      kind: 'ok',
                      follows: prev.follows.filter((f) => f.rkey !== card.rkey),
                  }
                : prev,
        );
        // Unfollowing a person followed this session returns their suggestion to a followable state.
        if (removed.wasSessionFollow) {
            setFollowedDids((prev) => withoutKey(prev, card.subjectDid));
        }
        return removed;
    };

    const restoreFollow = (card: FollowCard, removed: RemovedFollow) => {
        setFollowState((prev) => {
            if (prev.kind !== 'ok') return prev;
            const follows = prev.follows.slice();
            follows.splice(removed.index, 0, removed.record);
            return { kind: 'ok', follows };
        });
        if (removed.wasSessionFollow) {
            setFollowedDids((prev) => new Set(prev).add(card.subjectDid));
        }
    };

    const onUnfollow = async (card: FollowCard) => {
        const removed = removeFollow(card);
        if (!removed) return;
        try {
            await api(`/api/follows/${encodeURIComponent(card.rkey)}`, {
                method: 'DELETE',
            });
        } catch {
            restoreFollow(card, removed);
        }
    };

    const onSubscribeSource = (source: PersonPreviewSource) => {
        dialog.onSubscribe(personSourceToCard(source));
    };

    // The wire flag is the de-dup truth; the session set covers sources subscribed just now.
    const isSubscribed = (source: PersonPreviewSource) =>
        source.subscribed || dialog.subscribedKeys.has(source.key);

    const suggestSegments = partitionSourceSegments(suggestPeople, expandedKeys);
    const followSegments = partitionSourceSegments(followCards, followExpandedKeys);

    return (
        <div className="flex flex-col gap-4">
            <PersonSearch onSelect={onSearchSelect} onQueryChange={onSearchQueryChange} />

            <SearchSlot
                result={searchSlot.slot}
                profiles={profiles}
                previews={previews}
                followedDids={followedDids}
                followingDid={followingDid}
                viewerDid={me.did}
                onFollow={onFollowSlot}
                isSubscribed={isSubscribed}
                onSubscribeSource={onSubscribeSource}
            />

            <SuggestionSection
                state={suggestState}
                people={suggestPeople}
                cut={suggestCut}
                segments={suggestSegments}
                profiles={profiles}
                previews={previews}
                expandedKeys={expandedKeys}
                followedDids={followedDids}
                followingDid={followingDid}
                onToggle={onToggleSuggest}
                onFollow={suggestionActions.onFollow}
                onHide={onHideSuggest}
                onLoadMore={onLoadMore}
                isSubscribed={isSubscribed}
                onSubscribeSource={onSubscribeSource}
            />

            <FollowingSection
                state={followState}
                cards={followCards}
                cut={followCut}
                segments={followSegments}
                profiles={profiles}
                previews={previews}
                expandedKeys={followExpandedKeys}
                onToggle={onToggleFollow}
                onUnfollow={onUnfollow}
                isSubscribed={isSubscribed}
                onSubscribeSource={onSubscribeSource}
            />

            <SubscribeDialog
                source={dialog.dialogSource}
                open={dialog.dialogOpen}
                onOpenChange={dialog.onDialogOpenChange}
                onSubscribed={dialog.onSubscribed}
            />
        </div>
    );
}

function SuggestionSection({
    state,
    people,
    cut,
    segments,
    profiles,
    previews,
    expandedKeys,
    followedDids,
    followingDid,
    onToggle,
    onFollow,
    onHide,
    onLoadMore,
    isSubscribed,
    onSubscribeSource,
}: {
    state: PersonSuggestionState;
    people: PersonCard[];
    cut: DiscoverCut;
    segments: SourceSegment<PersonCard>[];
    profiles: Profiles;
    previews: PreviewsByDid;
    expandedKeys: ReadonlySet<string>;
    followedDids: ReadonlySet<string>;
    followingDid: string | undefined;
    onToggle: (key: string) => void;
    onFollow: (person: PersonCard, handle: string | undefined) => void;
    onHide: (person: PersonCard) => void;
    onLoadMore: () => void;
    isSubscribed: (source: PersonPreviewSource) => boolean;
    onSubscribeSource: (source: PersonPreviewSource) => void;
}) {
    if (state.kind === 'loading') {
        return <PersonStackSkeleton label="Loading suggestions" />;
    }
    if (state.kind === 'error') return <SuggestionsError />;
    if (people.length === 0) return null;

    return (
        <>
            <CutSegmentStack
                cut={cut}
                segments={segments}
                masthead={
                    <CardMasthead
                        eyebrow="Discover"
                        heading="Worth adding to your reading"
                        meta={countLabel(people.length, 'person', 'people')}
                    />
                }
                renderRow={(person, row) => (
                    <DiscoverPersonRow
                        key={person.key}
                        variant="suggestion"
                        did={person.did}
                        profile={profiles[person.did]}
                        previewState={previews[person.did]}
                        expanded={expandedKeys.has(person.key)}
                        intentExpanded={row.intentExpanded}
                        contentState={row.contentState}
                        onToggle={() => onToggle(person.key)}
                        showDivider={row.showDivider}
                        dividerState={row.dividerState}
                        reasonDisplay={resolvePersonReasonDisplay(person.reason)}
                        profiles={profiles}
                        following={followedDids.has(person.did)}
                        followPending={followingDid === person.did}
                        canFollow={Boolean(profiles[person.did]?.handle)}
                        onFollow={() =>
                            onFollow(person, profiles[person.did]?.handle)
                        }
                        onHide={() => onHide(person)}
                        isSubscribed={isSubscribed}
                        onSubscribeSource={onSubscribeSource}
                    />
                )}
            />
            <SuggestionLoadMore state={state} onLoadMore={onLoadMore} />
        </>
    );
}

function SuggestionLoadMore({
    state,
    onLoadMore,
}: {
    state: Extract<PersonSuggestionState, { kind: 'ok' }>;
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

function FollowingSection({
    state,
    cards,
    cut,
    segments,
    profiles,
    previews,
    expandedKeys,
    onToggle,
    onUnfollow,
    isSubscribed,
    onSubscribeSource,
}: {
    state: FollowListState;
    cards: FollowCard[];
    cut: DiscoverCut;
    segments: SourceSegment<FollowCard>[];
    profiles: Profiles;
    previews: PreviewsByDid;
    expandedKeys: ReadonlySet<string>;
    onToggle: (card: FollowCard) => void;
    onUnfollow: (card: FollowCard) => void;
    isSubscribed: (source: PersonPreviewSource) => boolean;
    onSubscribeSource: (source: PersonPreviewSource) => void;
}) {
    if (state.kind === 'loading') {
        return <PersonStackSkeleton label="Loading who you follow" />;
    }
    if (state.kind === 'error') {
        return (
            <FollowStatusCard>
                <p className="px-6 py-5 text-sm font-light text-muted-foreground">
                    Couldn’t load who you follow.
                </p>
            </FollowStatusCard>
        );
    }
    if (cards.length === 0) {
        return (
            <FollowStatusCard>
                <p className="px-6 py-5 text-sm font-light text-muted-foreground">
                    You’re not following anyone yet. Follow people from the
                    suggestions above.
                </p>
            </FollowStatusCard>
        );
    }

    return (
        <CutSegmentStack
            cut={cut}
            segments={segments}
            masthead={
                <CardMasthead
                    eyebrow="People"
                    heading="Following"
                    meta={countLabel(cards.length, 'person', 'people')}
                />
            }
            renderRow={(card, row) => (
                <DiscoverPersonRow
                    key={card.key}
                    variant="follow"
                    did={card.subjectDid}
                    profile={profiles[card.subjectDid]}
                    previewState={previews[card.subjectDid]}
                    expanded={expandedKeys.has(card.key)}
                    intentExpanded={row.intentExpanded}
                    contentState={row.contentState}
                    onToggle={() => onToggle(card)}
                    showDivider={row.showDivider}
                    dividerState={row.dividerState}
                    onUnfollow={() => onUnfollow(card)}
                    isSubscribed={isSubscribed}
                    onSubscribeSource={onSubscribeSource}
                />
            )}
        />
    );
}

function SearchSlot({
    result,
    profiles,
    previews,
    followedDids,
    followingDid,
    viewerDid,
    onFollow,
    isSubscribed,
    onSubscribeSource,
}: {
    result: PersonSearchResult | null;
    profiles: Profiles;
    previews: PreviewsByDid;
    followedDids: ReadonlySet<string>;
    followingDid: string | undefined;
    viewerDid: string;
    onFollow: () => void;
    isSubscribed: (source: PersonPreviewSource) => boolean;
    onSubscribeSource: (source: PersonPreviewSource) => void;
}) {
    return (
        <AnimatePresence>
            {result ? (
                <SearchSlotCard
                    key={result.did}
                    result={result}
                    profile={profiles[result.did]}
                    previewState={previews[result.did]}
                    following={followedDids.has(result.did)}
                    followPending={followingDid === result.did}
                    isSelf={result.did === viewerDid}
                    onFollow={onFollow}
                    isSubscribed={isSubscribed}
                    onSubscribeSource={onSubscribeSource}
                />
            ) : null}
        </AnimatePresence>
    );
}

// SPEC <discovery> line 558: a materialized search result — always expanded, outside the cut
// machine entirely (it was never a collapsed neighbor, so there's no seam to cut). Only this
// wrapper animates height/opacity on mount/unmount; the row's own PreviewDisclosure is told not
// to (revealOnMount=false) so the two auto-height tweens don't race each other.
function SearchSlotCard({
    result,
    profile,
    previewState,
    following,
    followPending,
    isSelf,
    onFollow,
    isSubscribed,
    onSubscribeSource,
}: {
    result: PersonSearchResult;
    profile: Profile | undefined;
    previewState: PreviewState | undefined;
    following: boolean;
    followPending: boolean;
    isSelf: boolean;
    onFollow: () => void;
    isSubscribed: (source: PersonPreviewSource) => boolean;
    onSubscribeSource: (source: PersonPreviewSource) => void;
}) {
    const reducedMotion = useReducedMotion();
    return (
        <motion.article
            initial={reducedMotion ? false : { height: 0, opacity: 0 }}
            animate={{ height: 'auto', opacity: 1, transition: splitOpenFade() }}
            exit={reducedMotion ? undefined : { height: 0, opacity: 0, transition: splitFade() }}
            className="overflow-hidden rounded-xl bg-card shadow-card"
        >
            <ul className="flex list-none flex-col">
                <DiscoverPersonRow
                    variant="searched"
                    did={result.did}
                    profile={profile}
                    previewState={previewState}
                    expanded
                    intentExpanded
                    contentState="open"
                    onToggle={() => {}}
                    showDivider={false}
                    dividerState="inset"
                    presenceless={!result.inReaderNetwork}
                    isSelf={isSelf}
                    following={following}
                    followPending={followPending}
                    canFollow={!isSelf}
                    onFollow={onFollow}
                    isSubscribed={isSubscribed}
                    onSubscribeSource={onSubscribeSource}
                />
            </ul>
        </motion.article>
    );
}

function FollowStatusCard({ children }: { children: ReactNode }) {
    return (
        <article className="overflow-hidden rounded-xl bg-card shadow-card">
            <CardMasthead eyebrow="People" heading="Following" />
            <div aria-hidden className="mx-6 border-t border-border" />
            {children}
        </article>
    );
}

function SuggestionsError() {
    return (
        <p className="text-sm font-light text-muted-foreground">
            Couldn’t load suggestions. Try refreshing in a moment.
        </p>
    );
}

function PersonStackSkeleton({ label }: { label: string }) {
    return (
        <DiscoverStackSkeleton
            label={label}
            row={
                <>
                    <div className="flex items-center gap-3">
                        <Skeleton className="size-9 shrink-0 rounded-full" />
                        <div className="min-w-0 flex-1 space-y-1.5">
                            <Skeleton className="h-4 w-40" />
                            <Skeleton className="h-3 w-24" />
                        </div>
                    </div>
                    <div className="mt-4 flex items-center justify-between gap-2 pt-3">
                        <Skeleton className="h-4 w-40" />
                        <Skeleton className="h-8 w-20 rounded-lg" />
                    </div>
                </>
            }
        />
    );
}
