import { describe, expect, test } from 'bun:test';

import type { DiscoverSource } from '@/lib/discover';
import { partitionSourceSegments, type SourceSegment } from './discover-segments';

function source(key: string): DiscoverSource {
    return { key, kind: 'rss', reason: { strongCount: 0, weakCount: 0 } };
}

function keysOf(sources: DiscoverSource[]): string[] {
    return sources.map((s) => s.key);
}

function assertInvariants(segments: SourceSegment[]) {
    segments.forEach((segment, index) => {
        if (index === 0) return;
        expect(segment.masthead).toBe(false);
        expect(segment.key).not.toBe('masthead');
        expect(segment.key).toBe(segment.sources[0]?.key);
    });
}

describe('partitionSourceSegments', () => {
    test('empty input yields no segments', () => {
        expect(partitionSourceSegments([], new Set())).toEqual([]);
    });

    test('all collapsed coalesce into a single masthead segment', () => {
        const sources = [source('a'), source('b'), source('c')];
        const segments = partitionSourceSegments(sources, new Set());
        expect(segments).toHaveLength(1);
        expect(segments[0]).toMatchObject({ key: 'masthead', expanded: false, masthead: true });
        expect(keysOf(segments[0].sources)).toEqual(['a', 'b', 'c']);
        assertInvariants(segments);
    });

    test('an expanded head source strips the masthead down to an empty segment', () => {
        const sources = [source('a'), source('b'), source('c')];
        const segments = partitionSourceSegments(sources, new Set(['a']));
        expect(segments).toHaveLength(3);

        expect(segments[0]).toMatchObject({ key: 'masthead', expanded: false, masthead: true });
        expect(segments[0].sources).toEqual([]);

        expect(segments[1]).toMatchObject({ key: 'a', expanded: true, masthead: false });
        expect(keysOf(segments[1].sources)).toEqual(['a']);

        expect(segments[2]).toMatchObject({ key: 'b', expanded: false, masthead: false });
        expect(keysOf(segments[2].sources)).toEqual(['b', 'c']);

        assertInvariants(segments);
    });

    test('a middle expanded source splits the masthead run into three segments', () => {
        const sources = [source('a'), source('b'), source('c')];
        const segments = partitionSourceSegments(sources, new Set(['b']));
        expect(segments).toHaveLength(3);

        expect(segments[0]).toMatchObject({ key: 'masthead', expanded: false, masthead: true });
        expect(keysOf(segments[0].sources)).toEqual(['a']);

        expect(segments[1]).toMatchObject({ key: 'b', expanded: true, masthead: false });
        expect(keysOf(segments[1].sources)).toEqual(['b']);

        expect(segments[2]).toMatchObject({ key: 'c', expanded: false, masthead: false });
        expect(keysOf(segments[2].sources)).toEqual(['c']);

        assertInvariants(segments);
    });

    test('adjacent expanded sources become separate singleton segments', () => {
        const sources = [source('a'), source('b'), source('c'), source('d')];
        const segments = partitionSourceSegments(sources, new Set(['b', 'c']));
        expect(segments).toHaveLength(4);
        expect(segments.map((s) => s.key)).toEqual(['masthead', 'b', 'c', 'd']);
        expect(segments.map((s) => s.expanded)).toEqual([false, true, true, false]);
        expect(segments.map((s) => s.masthead)).toEqual([true, false, false, false]);
        expect(keysOf(segments[0].sources)).toEqual(['a']);
        expect(keysOf(segments[1].sources)).toEqual(['b']);
        expect(keysOf(segments[2].sources)).toEqual(['c']);
        expect(keysOf(segments[3].sources)).toEqual(['d']);
        assertInvariants(segments);
    });

    test('an expanded tail source trails the masthead run', () => {
        const sources = [source('a'), source('b'), source('c')];
        const segments = partitionSourceSegments(sources, new Set(['c']));
        expect(segments).toHaveLength(2);
        expect(segments[0]).toMatchObject({ key: 'masthead', expanded: false, masthead: true });
        expect(keysOf(segments[0].sources)).toEqual(['a', 'b']);
        expect(segments[1]).toMatchObject({ key: 'c', expanded: true, masthead: false });
        expect(keysOf(segments[1].sources)).toEqual(['c']);
        assertInvariants(segments);
    });

    test('masthead: true and key "masthead" only ever appear on segment 0', () => {
        const sources = [source('a'), source('b'), source('c')];
        for (const expandedKeys of [new Set<string>(), new Set(['a']), new Set(['b']), new Set(['c'])]) {
            const segments = partitionSourceSegments(sources, expandedKeys);
            expect(segments[0].masthead).toBe(true);
            expect(segments[0].key).toBe('masthead');
            assertInvariants(segments);
        }
    });
});
