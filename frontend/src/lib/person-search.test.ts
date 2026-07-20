import { describe, expect, test } from 'bun:test';

import {
    EMPTY_SEARCH_SLOT,
    NO_SEARCH_RESULTS,
    personSearchSlotReducer,
    personSearchView,
    searchResultHint,
    searchResultToCard,
    searchResultToPerson,
    searchResultToProfile,
    type PersonSearchResult,
    type PersonSearchResultsState,
    type PersonSearchSlotState,
} from './person-search';

function result(overrides: Partial<PersonSearchResult> = {}): PersonSearchResult {
    return {
        did: 'did:plc:alice',
        handle: 'alice.example',
        displayName: 'Alice Example',
        avatar: 'https://cdn.example.com/alice.jpg',
        inReaderNetwork: true,
        ...overrides,
    };
}

describe('searchResultToPerson', () => {
    test('carries the did through with no ranking reason', () => {
        const person = searchResultToPerson(result());
        expect(person.did).toBe('did:plc:alice');
        expect(person.reason).toEqual({
            blueskyFollow: false,
            tangledFollow: false,
            sharedSourceCount: 0,
        });
    });

    test('a reader-network member with a taste hint maps it to the taste preview', () => {
        const person = searchResultToPerson(
            result({ tasteHint: ['Example Weekly', 'Example Digest'] }),
        );
        expect(person.tastePreview).toEqual({
            titles: ['Example Weekly', 'Example Digest'],
        });
    });

    test('a presence-less person maps with an empty reason and no taste preview', () => {
        const person = searchResultToPerson(
            result({ inReaderNetwork: false, tasteHint: undefined }),
        );
        expect(person.reason).toEqual({
            blueskyFollow: false,
            tangledFollow: false,
            sharedSourceCount: 0,
        });
        expect(person.tastePreview).toBeUndefined();
    });

    test('a presence-less result with a stray taste hint still gets no taste preview', () => {
        // Defensive: the wire contract only sends tasteHint for reader-network members, but the
        // mapping shouldn't trust that blindly.
        const person = searchResultToPerson(
            result({ inReaderNetwork: false, tasteHint: ['Example Weekly'] }),
        );
        expect(person.tastePreview).toBeUndefined();
    });

    test('an empty taste hint array maps to no taste preview', () => {
        const person = searchResultToPerson(result({ tasteHint: [] }));
        expect(person.tastePreview).toBeUndefined();
    });
});

describe('searchResultToCard', () => {
    test('adds a key mapped from the did, mirroring toPersonCard', () => {
        const card = searchResultToCard(result());
        expect(card.key).toBe('did:plc:alice');
        expect(card.did).toBe('did:plc:alice');
    });
});

describe('searchResultToProfile', () => {
    test('maps the profile-decoration fields the row/preview machinery consumes', () => {
        expect(searchResultToProfile(result())).toEqual({
            did: 'did:plc:alice',
            handle: 'alice.example',
            displayName: 'Alice Example',
            avatar: 'https://cdn.example.com/alice.jpg',
        });
    });

    test('a presence-less result still maps its profile fields', () => {
        const bare = result({
            did: 'did:plc:bob',
            handle: 'bob.example',
            displayName: null,
            avatar: null,
            inReaderNetwork: false,
        });
        expect(searchResultToProfile(bare)).toEqual({
            did: 'did:plc:bob',
            handle: 'bob.example',
            displayName: null,
            avatar: null,
        });
    });
});

describe('personSearchSlotReducer', () => {
    test('select materializes the result into the slot', () => {
        const next = personSearchSlotReducer(EMPTY_SEARCH_SLOT, {
            type: 'select',
            result: result(),
        });
        expect(next.slot).toEqual(result());
    });

    test('selecting a second result replaces the slot rather than stacking', () => {
        const first = personSearchSlotReducer(EMPTY_SEARCH_SLOT, {
            type: 'select',
            result: result(),
        });
        const second = personSearchSlotReducer(first, {
            type: 'select',
            result: result({ did: 'did:plc:carol', handle: 'carol.example' }),
        });
        expect(second.slot?.did).toBe('did:plc:carol');
    });

    test('a query change dismisses an active slot', () => {
        const active: PersonSearchSlotState = { slot: result() };
        expect(personSearchSlotReducer(active, { type: 'queryChanged' })).toEqual(
            EMPTY_SEARCH_SLOT,
        );
    });

    test('a query change with no active slot is a no-op', () => {
        expect(
            personSearchSlotReducer(EMPTY_SEARCH_SLOT, { type: 'queryChanged' }),
        ).toBe(EMPTY_SEARCH_SLOT);
    });
});

describe('personSearchView', () => {
    test('a blank query is idle, regardless of the resolved state', () => {
        expect(personSearchView('   ', NO_SEARCH_RESULTS)).toEqual({
            idle: true,
            pending: false,
            items: [],
        });
    });

    test('pending when the resolved state is for a stale query', () => {
        const state: PersonSearchResultsState = {
            kind: 'ok',
            query: 'al',
            results: [result()],
        };
        expect(personSearchView('alice', state)).toEqual({
            idle: false,
            pending: true,
            items: [],
        });
    });

    test('items populate once the ok state matches the trimmed query', () => {
        const state: PersonSearchResultsState = {
            kind: 'ok',
            query: 'alice',
            results: [result()],
        };
        expect(personSearchView('  alice  ', state)).toEqual({
            idle: false,
            pending: false,
            items: [result()],
        });
    });

    test('an error state resolves to no items even when the query matches', () => {
        const state: PersonSearchResultsState = { kind: 'error', query: 'alice' };
        expect(personSearchView('alice', state)).toEqual({
            idle: false,
            pending: false,
            items: [],
        });
    });
});

describe('searchResultHint', () => {
    test('joins multiple taste hints with a comma', () => {
        expect(
            searchResultHint(
                result({ tasteHint: ['Example Weekly', 'Example Digest'] }),
            ),
        ).toBe('Example Weekly, Example Digest');
    });

    test('undefined when the taste hint is missing', () => {
        expect(searchResultHint(result({ tasteHint: undefined }))).toBeUndefined();
    });

    test('undefined when the taste hint is an empty array', () => {
        expect(searchResultHint(result({ tasteHint: [] }))).toBeUndefined();
    });
});

describe('NO_SEARCH_RESULTS', () => {
    test('is an ok state with an empty query and no results', () => {
        expect(NO_SEARCH_RESULTS).toEqual({ kind: 'ok', query: '', results: [] });
    });
});
