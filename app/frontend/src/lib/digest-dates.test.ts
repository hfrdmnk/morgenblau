import { describe, expect, test } from 'bun:test';

import {
    addCalendarDays,
    digestDateRange,
    parseDigestDate,
    selectedDigestDate,
    toDateKey,
} from './digest-dates';

describe('digest dates', () => {
    test('today shows only today and the preceding three days', () => {
        const today = new Date(2026, 8, 12);

        expect(digestDateRange(today, today).map(toDateKey)).toEqual([
            '2026-09-09',
            '2026-09-10',
            '2026-09-11',
            '2026-09-12',
        ]);
    });

    test('an older day shows three days on each side', () => {
        const selected = new Date(2026, 8, 8);
        const today = new Date(2026, 8, 12);

        expect(digestDateRange(selected, today).map(toDateKey)).toEqual([
            '2026-09-05',
            '2026-09-06',
            '2026-09-07',
            '2026-09-08',
            '2026-09-09',
            '2026-09-10',
            '2026-09-11',
        ]);
    });

    test('dates near today stop at today instead of showing the future', () => {
        const selected = new Date(2026, 8, 10);
        const today = new Date(2026, 8, 12);

        expect(digestDateRange(selected, today).map(toDateKey)).toEqual([
            '2026-09-07',
            '2026-09-08',
            '2026-09-09',
            '2026-09-10',
            '2026-09-11',
            '2026-09-12',
        ]);
    });

    test('the digest shows the requested day, or today for a future or malformed one', () => {
        const today = new Date(2026, 8, 12);

        expect(toDateKey(selectedDigestDate('?date=2026-09-08', today))).toBe('2026-09-08');
        expect(toDateKey(selectedDigestDate('?date=2026-09-13', today))).toBe('2026-09-12');
        expect(toDateKey(selectedDigestDate('?date=2026-09-31', today))).toBe('2026-09-12');
        expect(toDateKey(selectedDigestDate('', today))).toBe('2026-09-12');
    });

    test('calendar arithmetic crosses month boundaries', () => {
        expect(toDateKey(addCalendarDays(new Date(2026, 2, 1), -1))).toBe(
            '2026-02-28',
        );
    });

    test('parsing accepts calendar dates and rejects normalized or malformed input', () => {
        expect(toDateKey(parseDigestDate('2026-02-28')!)).toBe('2026-02-28');
        expect(parseDigestDate('2026-02-30')).toBeNull();
        expect(parseDigestDate('28-02-2026')).toBeNull();
    });
});
