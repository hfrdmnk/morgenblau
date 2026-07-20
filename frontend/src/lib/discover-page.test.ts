import { describe, expect, test } from 'bun:test';

import {
    reduceDiscoverPage,
    type DiscoverPageState,
} from './discover-page';

type Item = { key: string; title: string };

const initial: DiscoverPageState<Item> = {
    items: [{ key: 'a', title: 'A' }],
    nextCursor: 'cursor-1',
    loadingMore: false,
};

describe('reduceDiscoverPage', () => {
    test('starts an incremental request without changing the list or cursor', () => {
        expect(reduceDiscoverPage(initial, { type: 'loadMore' })).toEqual({
            ...initial,
            loadingMore: true,
        });
    });

    test('appends unique items and advances the cursor', () => {
        const loading = reduceDiscoverPage(initial, { type: 'loadMore' });
        expect(
            reduceDiscoverPage(loading, {
                type: 'append',
                page: {
                    items: [
                        { key: 'a', title: 'Duplicate' },
                        { key: 'b', title: 'B' },
                    ],
                    nextCursor: 'cursor-2',
                },
            }),
        ).toEqual({
            items: [
                { key: 'a', title: 'A' },
                { key: 'b', title: 'B' },
            ],
            nextCursor: 'cursor-2',
            loadingMore: false,
        });
    });

    test('an exhausted page removes the cursor', () => {
        expect(
            reduceDiscoverPage(initial, {
                type: 'append',
                page: { items: [{ key: 'b', title: 'B' }] },
            }).nextCursor,
        ).toBeUndefined();
    });

    test('failure keeps the current items and cursor available for retry', () => {
        const loading = reduceDiscoverPage(initial, { type: 'loadMore' });
        expect(reduceDiscoverPage(loading, { type: 'failed' })).toEqual(initial);
    });
});
