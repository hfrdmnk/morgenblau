import { describe, expect, test } from 'bun:test';

import { feedFacts, groupByInitial, newsletterFacts } from './source-display';

describe('source facts', () => {
    test('a feed lists its primary status, cadence, post count and last post', () => {
        const facts = feedFacts({ primary: true, frequency: 'weekly', totalEntries: 1, lastPublishedAt: '2026-10-12T09:00:00Z' });
        expect(facts.slice(0, 3)).toEqual(['Primary', 'Weekly', '1 post']);
        expect(facts[3]).toStartWith('Last post ');
        expect(feedFacts({ primary: false, frequency: 'noPosts', totalEntries: 0 })).toEqual(['Nothing yet']);
    });

    test('a stopped newsletter drops its cadence and primary status', () => {
        const source = { primary: true, frequency: 'daily', issueCount: 12, lastReceivedAt: null };
        expect(newsletterFacts({ ...source, status: 'active' })).toEqual(['Primary', 'Daily', '12 issues']);
        expect(newsletterFacts({ ...source, status: 'stopped' })).toEqual(['Stopped', '12 issues']);
    });
});

describe('groupByInitial', () => {
    const group = (labels: string[]) => groupByInitial(labels, (label) => label)
        .map(({ initial, items }) => [initial, items]);

    test('groups by first letter in collation order, folding case and accents', () => {
        expect(group(['beta.example.com', 'Zeta Weekly', 'Ärzte Blog', 'alpha', 'Amp News'])).toEqual([
            ['A', ['alpha', 'Amp News', 'Ärzte Blog']],
            ['B', ['beta.example.com']],
            ['Z', ['Zeta Weekly']],
        ]);
    });

    test('collects labels that start with a non-letter under a trailing #', () => {
        expect(group(['42 Example', 'Example Publication', '“Quoted” Example'])).toEqual([
            ['E', ['Example Publication']],
            ['#', ['“Quoted” Example', '42 Example']],
        ]);
    });
});
