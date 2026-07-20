import type { FollowRecord } from '@/lib/follow';
import type { DiscoverPage } from '@/lib/discover-page';
import { hostnameOf } from '@/lib/utils';

// SPEC <discovery> People. Saves confer no eligibility and appear nowhere here (see <saving-sharing>).
export type PersonReason = {
    blueskyFollow: boolean;
    tangledFollow: boolean;
    followedByDid?: string;
    sharedSourceCount: number;
    trending?: boolean;
};

// SPEC <discovery> Cards: a small taste preview.
export type PersonTastePreview = {
    titles?: string[];
    latestShareItemUrl?: string;
    latestShareComment?: string;
};

export type DiscoverPerson = {
    did: string;
    reason: PersonReason;
    tastePreview?: PersonTastePreview;
};

// The cut machine (discover-cut.ts / discover-segments.ts) keys every row on `key`.
export type PersonCard = DiscoverPerson & { key: string };

export function toPersonCard(person: DiscoverPerson): PersonCard {
    return { ...person, key: person.did };
}

// SPEC <discovery> People: priority is taste > followed-by > Bluesky > Tangled, since taste is the most persuasive basis for a follow decision.
export function formatPersonReason(
    reason: PersonReason,
    followedByLabel?: string,
): string {
    if (reason.sharedSourceCount > 0) {
        return `reads ${reason.sharedSourceCount} of your sources`;
    }
    if (reason.followedByDid) {
        return followedByLabel
            ? `followed by ${followedByLabel}`
            : 'followed by someone you follow';
    }
    if (reason.blueskyFollow) {
        return 'you follow on Bluesky';
    }
    if (reason.tangledFollow) {
        return 'you follow on Tangled';
    }
    return 'active in the reader network';
}

export type PersonReasonDisplay =
    | { kind: 'text'; text: string }
    | { kind: 'followedBy'; did: string }
    | { kind: 'trending' };

// SPEC <discovery> line 576: any personal signal suppresses the trending line. followedBy stays
// unresolved (raw did, no pre-baked text) so a caller can render an avatar and inject a resolved label.
export function resolvePersonReasonDisplay(
    reason: PersonReason,
): PersonReasonDisplay {
    if (
        reason.sharedSourceCount > 0 ||
        reason.blueskyFollow ||
        reason.tangledFollow
    ) {
        return { kind: 'text', text: formatPersonReason(reason) };
    }
    if (reason.followedByDid) {
        return { kind: 'followedBy', did: reason.followedByDid };
    }
    if (reason.trending) {
        return { kind: 'trending' };
    }
    return { kind: 'text', text: formatPersonReason(reason) };
}

// SPEC <discovery> Cards: source names, or a fallback describing the latest share.
export function personTastePreviewLabel(
    preview: PersonTastePreview | undefined,
): string | undefined {
    if (!preview) return undefined;
    if (preview.titles && preview.titles.length > 0) {
        return `Reads ${preview.titles.join(', ')}`;
    }
    if (preview.latestShareComment) {
        return `Shared “${preview.latestShareComment}”`;
    }
    if (preview.latestShareItemUrl) {
        const hostname = hostnameOf(preview.latestShareItemUrl);
        return hostname ? `Shared ${hostname}` : 'Shared item';
    }
    return undefined;
}

// SPEC <discovery>: hide works identically to sources, keyed by DID.
export function personHidePayload(did: string): {
    targetKind: 'person';
    targetKey: string;
} {
    return { targetKind: 'person', targetKey: did };
}

// SPEC <discovery> Cards: expansion answers writes / reads / shares.
export type PersonPreviewSource = {
    key: string;
    kind: 'rss' | 'standardfeed';
    title: string;
    siteUrl: string;
    feedUrl?: string;
    publication?: string;
    subscribed: boolean;
};

export type PersonPreviewShare = {
    itemUrl?: string;
    document?: string;
    title?: string;
    targetUrl?: string;
    entrySlug?: string;
    comment?: string;
    createdAt: string;
};

export type PersonPreview = {
    writes: PersonPreviewSource[];
    writesTotal?: number;
    reads: PersonPreviewSource[];
    readsTotal?: number;
    latestShare: PersonPreviewShare | null;
};

export function personPreviewHiddenCount(
    total: number | undefined,
    visible: number,
): number {
    return Math.max(0, (total ?? visible) - visible);
}

// Mirrors PostsState (discover.ts): lazily fetched on expand, server-cached, best-effort.
export type PreviewState =
    | { status: 'loading' }
    | { status: 'loaded'; preview: PersonPreview };

export function personPreviewEmpty(preview: PersonPreview): boolean {
    return (
        preview.writes.length === 0 &&
        preview.reads.length === 0 &&
        !preview.latestShare
    );
}

// Every DID a people card can reference: the person, their introducer, and follows.
export function peopleProfileDids(
    people: DiscoverPerson[],
    follows: FollowRecord[],
): string[] {
    return [
        ...people.map((p) => p.did),
        ...people
            .map((p) => p.reason.followedByDid)
            .filter((did): did is string => Boolean(did)),
        ...follows.map((f) => f.subjectDid),
    ];
}

export type PersonSuggestionState =
    | { kind: 'loading' }
    | {
          kind: 'ok';
          people: PersonCard[];
          nextCursor?: string;
          loadingMore: boolean;
      }
    | { kind: 'error' };

// The cut machine keys every row on `key`; follows key on their rkey.
export type FollowCard = FollowRecord & { key: string };

export type FollowListState =
    | { kind: 'loading' }
    | { kind: 'ok'; follows: FollowCard[] }
    | { kind: 'error' };

export function toFollowCard(row: FollowRecord): FollowCard {
    return { ...row, key: row.rkey };
}

export type PeopleLoad = {
    suggest: PersonSuggestionState;
    follow: FollowListState;
    people: DiscoverPerson[];
    nextCursor?: string;
    follows: FollowRecord[];
    // Suggestions and follows degrade independently; only a fully clean load may seed the cache.
    complete: boolean;
};

function settledList<T>(result: PromiseSettledResult<T[] | null>): T[] | null {
    if (result.status !== 'fulfilled') return null;
    return result.value ?? [];
}

function settledPage<T>(
    result: PromiseSettledResult<DiscoverPage<T> | null>,
): DiscoverPage<T> | null {
    if (result.status !== 'fulfilled') return null;
    return result.value ?? { items: [] };
}

export function settlePeopleLoad(
    peopleResult: PromiseSettledResult<DiscoverPage<DiscoverPerson> | null>,
    followsResult: PromiseSettledResult<FollowRecord[] | null>,
): PeopleLoad {
    const peoplePage = settledPage(peopleResult);
    const people = peoplePage?.items ?? [];
    const follows = settledList(followsResult);
    return {
        suggest: peoplePage
            ? {
                  kind: 'ok',
                  people: people.map(toPersonCard),
                  nextCursor: peoplePage.nextCursor,
                  loadingMore: false,
              }
            : { kind: 'error' },
        follow: follows
            ? { kind: 'ok', follows: follows.map(toFollowCard) }
            : { kind: 'error' },
        people,
        nextCursor: peoplePage?.nextCursor,
        follows: follows ?? [],
        complete: Boolean(peoplePage && follows),
    };
}
