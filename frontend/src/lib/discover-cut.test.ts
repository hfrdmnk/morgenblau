import { describe, expect, test } from 'bun:test';

import type { DiscoverSource } from '@/lib/discover';
import { DIVIDER_MS, SPLIT_DURATION_MS } from '@/lib/motion-transitions';
import {
    CARD_RADIUS_PX,
    cutSnaps,
    cutStart,
    cutStep,
    IDLE_CUT,
    mastheadDividerState,
    pendingFlip,
    planCut,
    rowContentState,
    rowDividerState,
    SEAM_OVERLAP_PX,
    segmentBreaksOut,
    segmentCutStyle,
    type CutPlan,
    type SeamEdges,
} from './discover-cut';

function source(key: string): DiscoverSource {
    return { key, kind: 'rss', reason: { strongCount: 0, weakCount: 0 } };
}

function seamsObject(plan: CutPlan): Record<string, SeamEdges> {
    return Object.fromEntries(plan.seams);
}

describe('planCut', () => {
    test('empty source list has no seams', () => {
        const plan = planCut([], new Set(), 'a');
        expect(plan.hasSeams).toBe(false);
        expect(plan.seams.size).toBe(0);
        expect(plan.fullBleedRowKeys.size).toBe(0);
        expect(plan.mastheadDivider).toBe(false);
    });

    test('expanding a middle source seams both new dividers', () => {
        const sources = [source('a'), source('b'), source('c')];
        const plan = planCut(sources, new Set(), 'b');
        expect(plan.opening).toBe(true);
        expect(plan.hasSeams).toBe(true);
        expect(seamsObject(plan)).toEqual({
            masthead: { topCut: false, bottomCut: true },
            b: { topCut: true, bottomCut: true },
            c: { topCut: true, bottomCut: false },
        });
        expect(plan.fullBleedRowKeys).toEqual(new Set(['b', 'c']));
        expect(plan.mastheadDivider).toBe(false);
    });

    test('expanding the first source treats the masthead run head as the divider', () => {
        const sources = [source('a'), source('b'), source('c')];
        const plan = planCut(sources, new Set(), 'a');
        expect(seamsObject(plan)).toEqual({
            masthead: { topCut: false, bottomCut: true },
            a: { topCut: true, bottomCut: true },
            b: { topCut: true, bottomCut: false },
        });
        expect(plan.fullBleedRowKeys).toEqual(new Set(['b']));
        expect(plan.mastheadDivider).toBe(true);
    });

    test('expanding the last source seams a single boundary', () => {
        const sources = [source('a'), source('b'), source('c')];
        const plan = planCut(sources, new Set(), 'c');
        expect(seamsObject(plan)).toEqual({
            masthead: { topCut: false, bottomCut: true },
            c: { topCut: true, bottomCut: false },
        });
        expect(plan.fullBleedRowKeys).toEqual(new Set(['c']));
        expect(plan.mastheadDivider).toBe(false);
    });

    test('expanding a source below an already-expanded neighbor only seams the new boundary', () => {
        const sources = [source('a'), source('b'), source('c')];
        const plan = planCut(sources, new Set(['a']), 'b');
        expect(seamsObject(plan)).toEqual({
            b: { topCut: false, bottomCut: true },
            c: { topCut: true, bottomCut: false },
        });
        expect(plan.fullBleedRowKeys).toEqual(new Set(['c']));
        expect(plan.mastheadDivider).toBe(false);
    });

    test('a source alone between two expanded neighbors has no seams', () => {
        const sources = [source('a'), source('b'), source('c')];
        const plan = planCut(sources, new Set(['a', 'c']), 'b');
        expect(plan.hasSeams).toBe(false);
        expect(plan.seams.size).toBe(0);
        expect(plan.fullBleedRowKeys.size).toBe(0);
        expect(plan.mastheadDivider).toBe(false);
    });

    test('expanding a run head deeper in the list seams only the new boundary', () => {
        const sources = [source('a'), source('b'), source('c'), source('d')];
        const plan = planCut(sources, new Set(['b']), 'c');
        expect(seamsObject(plan)).toEqual({
            c: { topCut: false, bottomCut: true },
            d: { topCut: true, bottomCut: false },
        });
        expect(plan.fullBleedRowKeys).toEqual(new Set(['d']));
        expect(plan.mastheadDivider).toBe(false);
    });

    test('collapsing mirrors the matching expand exactly, aside from opening', () => {
        const abc = [source('a'), source('b'), source('c')];
        const scenarios: { sources: DiscoverSource[]; expandedKeys: Set<string>; key: string }[] = [
            { sources: abc, expandedKeys: new Set(), key: 'b' },
            { sources: abc, expandedKeys: new Set(), key: 'a' },
            { sources: abc, expandedKeys: new Set(), key: 'c' },
            { sources: abc, expandedKeys: new Set(['a']), key: 'b' },
        ];
        for (const { sources, expandedKeys, key } of scenarios) {
            const expandPlan = planCut(sources, expandedKeys, key);

            const collapsedKeys = new Set(expandedKeys);
            collapsedKeys.add(key);
            const collapsePlan = planCut(sources, collapsedKeys, key);

            expect(collapsePlan).toEqual({ ...expandPlan, opening: false });
        }
    });
});

const abc = [source('a'), source('b'), source('c')];

describe('cutStart', () => {
    test('opening with seams starts at dividing and defers the flip', () => {
        const plan = planCut(abc, new Set(), 'b');
        expect(cutStart(plan)).toEqual({
            state: { phase: 'dividing', plan },
            flipNow: false,
        });
    });

    test('a seamless open flips at once and jumps to the cut frame', () => {
        const plan = planCut(abc, new Set(['a', 'c']), 'b');
        expect(plan.hasSeams).toBe(false);
        expect(cutStart(plan)).toEqual({
            state: { phase: 'cut', plan },
            flipNow: true,
        });
    });

    test('collapsing starts at closing and defers the flip', () => {
        const plan = planCut(abc, new Set(['b']), 'b');
        expect(cutStart(plan)).toEqual({
            state: { phase: 'closing', plan },
            flipNow: false,
        });
    });
});

describe('pendingFlip', () => {
    test('dividing holds the opening flip', () => {
        const plan = planCut(abc, new Set(), 'b');
        expect(pendingFlip({ phase: 'dividing', plan })).toEqual({
            key: 'b',
            opening: true,
        });
    });

    test('closing holds the collapsing flip', () => {
        const plan = planCut(abc, new Set(['b']), 'b');
        expect(pendingFlip({ phase: 'closing', plan })).toEqual({
            key: 'b',
            opening: false,
        });
    });

    test('idle and tween phases hold nothing', () => {
        const plan = planCut(abc, new Set(), 'b');
        expect(pendingFlip(IDLE_CUT)).toBeNull();
        expect(pendingFlip({ phase: 'cut', plan })).toBeNull();
        expect(pendingFlip({ phase: 'separating', plan })).toBeNull();
        expect(pendingFlip({ phase: 'merging', plan })).toBeNull();
    });
});

describe('cutStep', () => {
    const plan = planCut(abc, new Set(), 'b');

    test('idle schedules nothing', () => {
        expect(cutStep(IDLE_CUT)).toBeNull();
    });

    test('dividing flips open after the divider and commits the cut frame', () => {
        expect(cutStep({ phase: 'dividing', plan })).toEqual({
            kind: 'timer',
            ms: DIVIDER_MS,
            flip: { key: 'b', opening: true },
            next: { phase: 'cut', plan },
        });
    });

    test('cut advances to separating on a fresh frame', () => {
        expect(cutStep({ phase: 'cut', plan })).toEqual({
            kind: 'frame',
            next: { phase: 'separating', plan },
        });
    });

    test('separating lands at idle after the split', () => {
        expect(cutStep({ phase: 'separating', plan })).toEqual({
            kind: 'timer',
            ms: SPLIT_DURATION_MS,
            flip: null,
            next: IDLE_CUT,
        });
    });

    test('closing with seams flips closed and merges', () => {
        const closing = planCut(abc, new Set(['b']), 'b');
        expect(cutStep({ phase: 'closing', plan: closing })).toEqual({
            kind: 'timer',
            ms: SPLIT_DURATION_MS,
            flip: { key: 'b', opening: false },
            next: { phase: 'merging', plan: closing },
        });
    });

    test('closing without seams lands straight at idle', () => {
        const closing = planCut(abc, new Set(['a', 'b', 'c']), 'b');
        expect(closing.hasSeams).toBe(false);
        expect(cutStep({ phase: 'closing', plan: closing })).toEqual({
            kind: 'timer',
            ms: SPLIT_DURATION_MS,
            flip: { key: 'b', opening: false },
            next: IDLE_CUT,
        });
    });

    test('merging lands at idle after the divider retract', () => {
        expect(cutStep({ phase: 'merging', plan })).toEqual({
            kind: 'timer',
            ms: DIVIDER_MS,
            flip: null,
            next: IDLE_CUT,
        });
    });
});

describe('segmentCutStyle', () => {
    const plan = planCut(abc, new Set(), 'b');
    const resting = {
        marginTop: 0,
        borderTopLeftRadius: CARD_RADIUS_PX,
        borderTopRightRadius: CARD_RADIUS_PX,
        borderBottomLeftRadius: CARD_RADIUS_PX,
        borderBottomRightRadius: CARD_RADIUS_PX,
    };

    test('a fully seamed segment squares off and pulls up over the gap', () => {
        expect(segmentCutStyle({ phase: 'cut', plan }, 'b')).toEqual({
            marginTop: -SEAM_OVERLAP_PX,
            borderTopLeftRadius: 0,
            borderTopRightRadius: 0,
            borderBottomLeftRadius: 0,
            borderBottomRightRadius: 0,
        });
    });

    test('a bottom-only cut keeps its top radius and stays in place', () => {
        expect(segmentCutStyle({ phase: 'cut', plan }, 'masthead')).toEqual({
            ...resting,
            borderBottomLeftRadius: 0,
            borderBottomRightRadius: 0,
        });
    });

    test('unseamed segments rest at the card shape', () => {
        expect(segmentCutStyle({ phase: 'cut', plan }, 'zzz')).toEqual(resting);
        expect(segmentCutStyle(IDLE_CUT, 'b')).toEqual(resting);
    });

    test('seams only apply during cut and closing', () => {
        expect(segmentCutStyle({ phase: 'closing', plan }, 'b').marginTop).toBe(
            -SEAM_OVERLAP_PX,
        );
        expect(segmentCutStyle({ phase: 'separating', plan }, 'b')).toEqual(resting);
        expect(segmentCutStyle({ phase: 'dividing', plan }, 'b')).toEqual(resting);
    });
});

describe('segmentBreaksOut', () => {
    const plan = planCut(abc, new Set(), 'b');

    test('collapsed segments never break out', () => {
        expect(segmentBreaksOut({ phase: 'cut', plan }, { key: 'a', expanded: false })).toBe(false);
        expect(segmentBreaksOut(IDLE_CUT, { key: 'a', expanded: false })).toBe(false);
    });

    test('the planned segment withholds breakout across cut and closing', () => {
        expect(segmentBreaksOut({ phase: 'cut', plan }, { key: 'b', expanded: true })).toBe(false);
        expect(segmentBreaksOut({ phase: 'closing', plan }, { key: 'b', expanded: true })).toBe(false);
        expect(segmentBreaksOut({ phase: 'separating', plan }, { key: 'b', expanded: true })).toBe(true);
        expect(segmentBreaksOut(IDLE_CUT, { key: 'b', expanded: true })).toBe(true);
    });

    test('expanded segments outside the plan break out during the cut', () => {
        expect(segmentBreaksOut({ phase: 'cut', plan }, { key: 'c', expanded: true })).toBe(true);
    });
});

describe('cutSnaps', () => {
    test('only the cut and merge commits snap', () => {
        expect(cutSnaps('cut')).toBe(true);
        expect(cutSnaps('merging')).toBe(true);
        expect(cutSnaps('idle')).toBe(false);
        expect(cutSnaps('dividing')).toBe(false);
        expect(cutSnaps('separating')).toBe(false);
        expect(cutSnaps('closing')).toBe(false);
    });
});

describe('divider states', () => {
    // Expanding 'a' marks the masthead divider and full-bleeds the divider above 'b'.
    const plan = planCut(abc, new Set(), 'a');

    test('the masthead divider goes full-bleed while dividing and retracts while merging', () => {
        expect(mastheadDividerState({ phase: 'dividing', plan })).toBe('full-bleed');
        expect(mastheadDividerState({ phase: 'merging', plan })).toBe('retracting');
        expect(mastheadDividerState({ phase: 'cut', plan })).toBe('inset');
        expect(mastheadDividerState(IDLE_CUT)).toBe('inset');
    });

    test('row dividers only move for rows on the plan full-bleed list', () => {
        expect(rowDividerState({ phase: 'dividing', plan }, 'b')).toBe('full-bleed');
        expect(rowDividerState({ phase: 'merging', plan }, 'b')).toBe('retracting');
        expect(rowDividerState({ phase: 'dividing', plan }, 'c')).toBe('inset');
        expect(rowDividerState(IDLE_CUT, 'b')).toBe('inset');
    });
});

describe('rowContentState', () => {
    const opening = planCut(abc, new Set(), 'b');
    const closing = planCut(abc, new Set(['b']), 'b');

    test('the opening row holds through the committed cut frame', () => {
        expect(rowContentState({ phase: 'cut', plan: opening }, 'b')).toBe('hold');
        expect(rowContentState({ phase: 'separating', plan: opening }, 'b')).toBe('open');
    });

    test('the closing row collapses through closing', () => {
        expect(rowContentState({ phase: 'closing', plan: closing }, 'b')).toBe('closing');
        expect(rowContentState({ phase: 'merging', plan: closing }, 'b')).toBe('open');
    });

    test('rows outside the plan stay open', () => {
        expect(rowContentState({ phase: 'cut', plan: opening }, 'a')).toBe('open');
        expect(rowContentState(IDLE_CUT, 'b')).toBe('open');
    });
});
