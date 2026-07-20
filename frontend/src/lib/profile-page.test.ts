import { describe, expect, test } from 'bun:test';

import {
    appendAction,
    canUnfollow,
    defaultSegment,
    initialListsState,
    isShareItem,
    loadedAction,
    metaLine,
    PENDING_FOLLOW,
    profileDisplayName,
    profileItemKey,
    profileListsReducer,
    profileTitle,
    visibleSegments,
    type ProfileShareItem,
    type SegmentPage,
} from './profile-page';
import type { PersonPreviewSource } from './discover-people';

const WRITE: PersonPreviewSource = {
    key: 'example.com/feed.xml',
    kind: 'rss',
    title: 'Example Publication',
    siteUrl: 'https://example.com',
    feedUrl: 'https://example.com/feed.xml',
    subscribed: false,
};

const SHARE: ProfileShareItem = {
    itemUrl: 'https://example.com/post',
    comment: 'good read',
    createdAt: '2026-07-01T00:00:00Z',
};

describe('visibleSegments', () => {
    test('includes writes when the person authors something', () => {
        expect(
            visibleSegments({ writes: 2, reads: 0, shares: 0 }),
        ).toEqual(['writes', 'shares', 'reads']);
    });

    test('hides writes when the person authors nothing', () => {
        expect(
            visibleSegments({ writes: 0, reads: 3, shares: 1 }),
        ).toEqual(['shares', 'reads']);
    });

    test('shares and reads always show, even at zero, since they are always-present archives', () => {
        expect(
            visibleSegments({ writes: 0, reads: 0, shares: 0 }),
        ).toEqual(['shares', 'reads']);
    });
});

describe('defaultSegment', () => {
    test('writes when visible', () => {
        expect(defaultSegment({ writes: 1, reads: 0, shares: 0 })).toBe(
            'writes',
        );
    });

    test('falls back to shares when writes is hidden', () => {
        expect(defaultSegment({ writes: 0, reads: 5, shares: 0 })).toBe(
            'shares',
        );
    });
});

describe('metaLine', () => {
    test('all-zero counts read as not-in-the-network', () => {
        expect(metaLine({ writes: 0, reads: 0, shares: 0 })).toBe(
            'Not in the reader network yet',
        );
    });

    test('joins all three categories, capitalizing the leading phrase', () => {
        expect(metaLine({ writes: 2, reads: 14, shares: 31 })).toBe(
            'Writes 2 publications · reads 14 sources · 31 shares',
        );
    });

    test('singular counts stay singular', () => {
        expect(metaLine({ writes: 1, reads: 1, shares: 1 })).toBe(
            'Writes 1 publication · reads 1 source · 1 share',
        );
    });

    test('omits a zero category and capitalizes whichever phrase leads', () => {
        expect(metaLine({ writes: 0, reads: 14, shares: 31 })).toBe(
            'Reads 14 sources · 31 shares',
        );
    });

    test('a single non-zero category stands alone', () => {
        expect(metaLine({ writes: 0, reads: 0, shares: 31 })).toBe(
            '31 shares',
        );
    });
});

describe('profileItemKey', () => {
    test('keys a source item by its canonical key', () => {
        expect(profileItemKey(WRITE)).toBe('example.com/feed.xml');
    });

    test('keys a share item by itemUrl', () => {
        expect(profileItemKey(SHARE)).toBe('https://example.com/post');
    });

    test('keys a standardfeed share by document when itemUrl is absent', () => {
        expect(
            profileItemKey({
                document:
                    'at://did:plc:publisher/site.standard.document/3example',
                createdAt: '2026-07-01T00:00:00Z',
            }),
        ).toBe('at://did:plc:publisher/site.standard.document/3example');
    });
});

describe('isShareItem', () => {
    test('true for a share item', () => {
        expect(isShareItem(SHARE)).toBe(true);
    });

    test('false for a source item', () => {
        expect(isShareItem(WRITE)).toBe(false);
    });

    test('true for a standardfeed share without itemUrl', () => {
        expect(
            isShareItem({
                document:
                    'at://did:plc:publisher/site.standard.document/3example',
                createdAt: '2026-07-01T00:00:00Z',
            }),
        ).toBe(true);
    });
});

describe('profileListsReducer', () => {
    test('loaded replaces items and clears loading status', () => {
        const state = profileListsReducer(initialListsState(), {
            type: 'loaded',
            segment: 'reads',
            items: [WRITE],
            nextCursor: 'cursor-1',
        });
        expect(state.reads).toEqual({
            items: [WRITE],
            nextCursor: 'cursor-1',
            status: 'loaded',
        });
        // Other segments are untouched.
        expect(state.writes.status).toBe('loading');
    });

    test('loadMore flips status without touching items', () => {
        const loaded = profileListsReducer(initialListsState(), {
            type: 'loaded',
            segment: 'reads',
            items: [WRITE],
            nextCursor: 'cursor-1',
        });
        const state = profileListsReducer(loaded, {
            type: 'loadMore',
            segment: 'reads',
        });
        expect(state.reads.status).toBe('loadingMore');
        expect(state.reads.items).toEqual([WRITE]);
    });

    test('append merges new items after existing ones and advances the cursor', () => {
        const second: PersonPreviewSource = { ...WRITE, key: 'second-key' };
        const loaded = profileListsReducer(initialListsState(), {
            type: 'loaded',
            segment: 'reads',
            items: [WRITE],
            nextCursor: 'cursor-1',
        });
        const state = profileListsReducer(loaded, {
            type: 'append',
            segment: 'reads',
            items: [second],
            nextCursor: 'cursor-2',
        });
        expect(state.reads).toEqual({
            items: [WRITE, second],
            nextCursor: 'cursor-2',
            status: 'loaded',
        });
    });

    test('append dedupes items already present by key', () => {
        const loaded = profileListsReducer(initialListsState(), {
            type: 'loaded',
            segment: 'reads',
            items: [WRITE],
            nextCursor: 'cursor-1',
        });
        const state = profileListsReducer(loaded, {
            type: 'append',
            segment: 'reads',
            items: [WRITE],
            nextCursor: undefined,
        });
        expect(state.reads.items).toEqual([WRITE]);
    });

    test('append dedupes share items already present by itemUrl', () => {
        const loaded = profileListsReducer(initialListsState(), {
            type: 'loaded',
            segment: 'shares',
            items: [SHARE],
            nextCursor: 'cursor-1',
        });
        const state = profileListsReducer(loaded, {
            type: 'append',
            segment: 'shares',
            items: [SHARE],
        });
        expect(state.shares.items).toEqual([SHARE]);
    });

    test('error on an empty segment marks it errored', () => {
        const state = profileListsReducer(initialListsState(), {
            type: 'error',
            segment: 'writes',
        });
        expect(state.writes.status).toBe('error');
    });

    test('error during a load-more (items already present) reverts to loaded, not error', () => {
        const loaded = profileListsReducer(initialListsState(), {
            type: 'loaded',
            segment: 'reads',
            items: [WRITE],
            nextCursor: 'cursor-1',
        });
        const loadingMore = profileListsReducer(loaded, {
            type: 'loadMore',
            segment: 'reads',
        });
        const state = profileListsReducer(loadingMore, {
            type: 'error',
            segment: 'reads',
        });
        expect(state.reads.status).toBe('loaded');
        expect(state.reads.items).toEqual([WRITE]);
        expect(state.reads.nextCursor).toBe('cursor-1');
    });
});

describe('profileDisplayName', () => {
    test('a trimmed display name wins', () => {
        expect(
            profileDisplayName({ displayName: '  Alice Example  ', handle: 'alice.example' }),
        ).toBe('Alice Example');
    });

    test('a blank display name falls back to the handle', () => {
        expect(
            profileDisplayName({ displayName: '   ', handle: 'alice.example' }),
        ).toBe('@alice.example');
    });

    test('a missing display name falls back to the handle', () => {
        expect(profileDisplayName({ handle: 'alice.example' })).toBe(
            '@alice.example',
        );
    });
});

describe('profileTitle', () => {
    test('a loaded profile titles by its display name', () => {
        expect(
            profileTitle({ displayName: 'Alice Example', handle: 'alice.example' }),
        ).toBe('Alice Example');
    });

    test('an undefined profile falls back to "Profile"', () => {
        expect(profileTitle(undefined)).toBe('Profile');
    });
});

describe('loadedAction', () => {
    test('wraps a page into a loaded action', () => {
        const page: SegmentPage = { items: [WRITE], nextCursor: 'cursor-1' };
        expect(loadedAction('reads', page)).toEqual({
            type: 'loaded',
            segment: 'reads',
            items: [WRITE],
            nextCursor: 'cursor-1',
        });
    });

    test('a null body yields empty items and no cursor', () => {
        expect(loadedAction('reads', null)).toEqual({
            type: 'loaded',
            segment: 'reads',
            items: [],
            nextCursor: undefined,
        });
    });
});

describe('appendAction', () => {
    test('wraps a page into an append action', () => {
        const page: SegmentPage = { items: [WRITE], nextCursor: 'cursor-2' };
        expect(appendAction('reads', page)).toEqual({
            type: 'append',
            segment: 'reads',
            items: [WRITE],
            nextCursor: 'cursor-2',
        });
    });

    test('a null body yields empty items and no cursor', () => {
        expect(appendAction('reads', null)).toEqual({
            type: 'append',
            segment: 'reads',
            items: [],
            nextCursor: undefined,
        });
    });
});

describe('canUnfollow', () => {
    test('false for null', () => {
        expect(canUnfollow(null)).toBe(false);
    });

    test('false for the pending placeholder rkey', () => {
        expect(canUnfollow(PENDING_FOLLOW)).toBe(false);
    });

    test('true for a real rkey', () => {
        expect(canUnfollow('3jzcut4wsuw2n')).toBe(true);
    });
});
