import type { PersonPreviewSource } from '@/lib/discover-people';
import { countLabel } from '@/lib/plural';

// SPEC <discovery> Profile page: segmented Writes | Shares | Reads.
export type Segment = 'writes' | 'shares' | 'reads';

export type ProfileCounts = { writes: number; reads: number; shares: number };

export const SEGMENT_LABEL: Record<Segment, string> = {
    writes: 'Writes',
    shares: 'Shares',
    reads: 'Reads',
};

// Shares and Reads are archives that always resolve (possibly empty); Writes is the only dead-tab risk.
export function visibleSegments(counts: ProfileCounts): Segment[] {
    const segments: Segment[] = [];
    if (counts.writes > 0) segments.push('writes');
    segments.push('shares', 'reads');
    return segments;
}

export function defaultSegment(counts: ProfileCounts): Segment {
    return visibleSegments(counts)[0];
}

// The reader-network meta line under the header. Omits zero categories; an all-zero profile
// gets the same honest-emptiness phrasing as SPEC <discovery> line 558's presence-less search result.
export function metaLine(counts: ProfileCounts): string {
    const parts: string[] = [];
    if (counts.writes > 0) {
        parts.push(
            `Writes ${countLabel(counts.writes, 'publication', 'publications')}`,
        );
    }
    if (counts.reads > 0) {
        parts.push(`reads ${countLabel(counts.reads, 'source', 'sources')}`);
    }
    if (counts.shares > 0) {
        parts.push(countLabel(counts.shares, 'share', 'shares'));
    }
    if (parts.length === 0) return 'Not in the reader network yet';
    const joined = parts.join(' · ');
    return joined.charAt(0).toUpperCase() + joined.slice(1);
}

type ProfileNames = { displayName?: string | null; handle: string };

export function profileDisplayName(profile: ProfileNames): string {
    return profile.displayName?.trim() || `@${profile.handle}`;
}

// Document title for the page; undefined covers the loading and error states.
export function profileTitle(profile: ProfileNames | undefined): string {
    return profile ? profileDisplayName(profile) : 'Profile';
}

export type ProfileShareItem = {
    itemUrl?: string;
    document?: string;
    title?: string;
    targetUrl?: string;
    entrySlug?: string;
    comment?: string;
    createdAt: string;
};

export type ProfileListItem = PersonPreviewSource | ProfileShareItem;

// Writes/reads items key on the source's canonical key; shares fall back through their available target identity.
export function profileItemKey(item: ProfileListItem): string {
    return isShareItem(item)
        ? (item.itemUrl ?? item.document ?? item.createdAt)
        : item.key;
}

export function isShareItem(item: ProfileListItem): item is ProfileShareItem {
    return 'createdAt' in item;
}

export type SegmentStatus = 'loading' | 'loadingMore' | 'loaded' | 'error';

export type SegmentState = {
    items: ProfileListItem[];
    nextCursor?: string;
    status: SegmentStatus;
};

export type ProfileListsState = Record<Segment, SegmentState>;

// All three start 'loading': only the active segment fetches immediately, the others adopt
// this as their pre-fetch state since their content never renders until selected.
export function initialListsState(): ProfileListsState {
    const empty = (): SegmentState => ({ items: [], status: 'loading' });
    return { writes: empty(), shares: empty(), reads: empty() };
}

export type SegmentAction =
    | {
          type: 'loaded';
          segment: Segment;
          items: ProfileListItem[];
          nextCursor?: string;
      }
    | { type: 'loadMore'; segment: Segment }
    | {
          type: 'append';
          segment: Segment;
          items: ProfileListItem[];
          nextCursor?: string;
      }
    | { type: 'error'; segment: Segment };

export type SegmentPage = {
    items: ProfileListItem[];
    nextCursor?: string;
} | null;

// Normalize a wire page (possibly null) into the reducer's actions.
export function loadedAction(segment: Segment, body: SegmentPage): SegmentAction {
    return {
        type: 'loaded',
        segment,
        items: body?.items ?? [],
        nextCursor: body?.nextCursor,
    };
}

export function appendAction(segment: Segment, body: SegmentPage): SegmentAction {
    return {
        type: 'append',
        segment,
        items: body?.items ?? [],
        nextCursor: body?.nextCursor,
    };
}

export function profileListsReducer(
    state: ProfileListsState,
    action: SegmentAction,
): ProfileListsState {
    const current = state[action.segment];
    switch (action.type) {
        case 'loaded':
            return {
                ...state,
                [action.segment]: {
                    items: action.items,
                    nextCursor: action.nextCursor,
                    status: 'loaded',
                },
            };
        case 'loadMore':
            return {
                ...state,
                [action.segment]: { ...current, status: 'loadingMore' },
            };
        case 'append': {
            const seen = new Set(current.items.map(profileItemKey));
            const fresh = action.items.filter(
                (item) => !seen.has(profileItemKey(item)),
            );
            return {
                ...state,
                [action.segment]: {
                    items: [...current.items, ...fresh],
                    nextCursor: action.nextCursor,
                    status: 'loaded',
                },
            };
        }
        case 'error':
            // A failure with items already on screen (a load-more retry) reverts to 'loaded'
            // rather than replacing the list with an error state; only a bare first load errors out.
            return {
                ...state,
                [action.segment]: {
                    ...current,
                    status: current.items.length > 0 ? 'loaded' : 'error',
                },
            };
    }
}

// A placeholder rkey, never sent to the server, that flips the follow button to its followed
// state immediately so both directions of the toggle feel optimistic.
export const PENDING_FOLLOW = '__pending__';

export function canUnfollow(rkey: string | null): rkey is string {
    return Boolean(rkey) && rkey !== PENDING_FOLLOW;
}
