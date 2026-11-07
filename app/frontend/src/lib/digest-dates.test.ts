import { describe, expect, test } from 'bun:test';

import {
    addCalendarDays,
    digestDateRange,
    parseDayQuery,
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

    describe('typed days', () => {
        // Saturday, 7 November 2026.
        const saturday = new Date(2026, 10, 7);
        const read = (query: string, today = saturday) => {
            const day = parseDayQuery(query, today);
            return day && toDateKey(day);
        };

        test('relative words count back from today', () => {
            expect(read('today')).toBe('2026-11-07');
            expect(read('Yest')).toBe('2026-11-06');
            expect(read('2 days ago')).toBe('2026-11-05');
            expect(read('2d')).toBe('2026-11-05');
            expect(read('a week ago')).toBe('2026-10-31');
            expect(read('last week')).toBe('2026-10-31');
            expect(read('3 weeks ago')).toBe('2026-10-17');
        });

        test('a weekday is the latest one before today, never today itself', () => {
            expect(read('friday')).toBe('2026-11-06');
            expect(read('monday')).toBe('2026-11-02');
            expect(read('last mon')).toBe('2026-11-02');
            expect(read('saturday')).toBe('2026-10-31');
            expect(read('sun')).toBe('2026-11-01');
        });

        test('a weekday crosses the year boundary', () => {
            expect(read('monday', new Date(2027, 0, 3))).toBe('2026-12-28');
        });

        test('a written day without a year is the latest one that is not in the future', () => {
            expect(read('14.10')).toBe('2026-10-14');
            expect(read('7.11.')).toBe('2026-11-07');
            expect(read('20.12')).toBe('2025-12-20');
            expect(read('14 oct')).toBe('2026-10-14');
            expect(read('Oct 14')).toBe('2026-10-14');
            expect(read('30.12', new Date(2027, 0, 3))).toBe('2026-12-30');
        });

        test('a written year is kept', () => {
            expect(read('14.10.2025')).toBe('2025-10-14');
            expect(read('14.10.25')).toBe('2025-10-14');
            expect(read('October 14, 2025')).toBe('2025-10-14');
            expect(read('2026-10-14')).toBe('2026-10-14');
        });

        test('future, impossible and unrelated input reads as no day', () => {
            expect(read('8.11.2026')).toBeNull();
            expect(read('2026-11-08')).toBeNull();
            expect(read('31.02')).toBeNull();
            expect(read('mo')).toBeNull();
            expect(read('mondays')).toBeNull();
            expect(read('sources')).toBeNull();
            expect(read('14 days of rain')).toBeNull();
            expect(read('')).toBeNull();
        });
    });
});
