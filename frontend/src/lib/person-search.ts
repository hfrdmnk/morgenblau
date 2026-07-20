import { toPersonCard, type DiscoverPerson, type PersonCard, type PersonReason } from '@/lib/discover-people';
import type { Profile } from '@/lib/profile';

// SPEC <discovery> line 558: whole-network person search. Reader-network members rank first and
// carry a taste hint; anyone else stays followable but presence-less.
export type PersonSearchResult = {
    did: string;
    handle: string;
    displayName?: string | null;
    avatar?: string | null;
    inReaderNetwork: boolean;
    // Capped at 2 by the server; present only for reader-network members.
    tasteHint?: string[];
};

const NO_REASON: PersonReason = {
    blueskyFollow: false,
    tangledFollow: false,
    sharedSourceCount: 0,
};

// A searched person carries no ranking reason (search finds, it never ranks) and, per SPEC, a
// presence-less result gets no taste preview either.
export function searchResultToPerson(result: PersonSearchResult): DiscoverPerson {
    const tastePreview =
        result.inReaderNetwork && result.tasteHint && result.tasteHint.length > 0
            ? { titles: result.tasteHint }
            : undefined;
    return { did: result.did, reason: NO_REASON, tastePreview };
}

export function searchResultToCard(result: PersonSearchResult): PersonCard {
    return toPersonCard(searchResultToPerson(result));
}

// The search endpoint already carries profile fields, so the materialized card decorates
// immediately with no extra /api/profiles round trip.
export function searchResultToProfile(result: PersonSearchResult): Profile {
    return {
        did: result.did,
        handle: result.handle,
        displayName: result.displayName,
        avatar: result.avatar,
    };
}

// The query a result (or error) resolved for, so a caller can tell a fresh keystroke from a
// completed search by simple comparison — no separate 'loading' state.
export type PersonSearchResultsState =
    | { kind: 'ok'; query: string; results: PersonSearchResult[] }
    | { kind: 'error'; query: string };

export const NO_SEARCH_RESULTS: PersonSearchResultsState = {
    kind: 'ok',
    query: '',
    results: [],
};

export type PersonSearchView = {
    idle: boolean;
    // A query the resolved state doesn't match is either still debouncing or in flight.
    pending: boolean;
    items: PersonSearchResult[];
};

export function personSearchView(
    query: string,
    state: PersonSearchResultsState,
): PersonSearchView {
    const trimmed = query.trim();
    const idle = trimmed.length === 0;
    const pending = !idle && state.query !== trimmed;
    const items =
        !idle && !pending && state.kind === 'ok' ? state.results : [];
    return { idle, pending, items };
}

export function searchResultHint(result: PersonSearchResult): string | undefined {
    if (!result.tasteHint || result.tasteHint.length === 0) return undefined;
    return result.tasteHint.join(', ');
}

// One materialized result at a time. Selecting replaces it; any query edit — typing or
// clearing alike — dismisses it (SPEC <discovery> line 558: "editing or clearing the query
// dismisses the slot").
export type PersonSearchSlotState = {
    slot: PersonSearchResult | null;
};

export const EMPTY_SEARCH_SLOT: PersonSearchSlotState = { slot: null };

export type PersonSearchSlotAction =
    | { type: 'select'; result: PersonSearchResult }
    | { type: 'queryChanged' };

export function personSearchSlotReducer(
    state: PersonSearchSlotState,
    action: PersonSearchSlotAction,
): PersonSearchSlotState {
    switch (action.type) {
        case 'select':
            return { slot: action.result };
        case 'queryChanged':
            return state.slot === null ? state : EMPTY_SEARCH_SLOT;
    }
}
