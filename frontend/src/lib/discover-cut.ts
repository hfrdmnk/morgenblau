import { partitionSourceSegments, type SourceSegment } from '@/lib/discover-segments';
import { DIVIDER_MS, SPLIT_DURATION_MS } from '@/lib/motion-transitions';

// gap-4 (16px) minus the 1px in-flow divider a fresh cut consumes
export const SEAM_OVERLAP_PX = 15;

// rounded-xl, kept as a number so a seamed corner can tween to square
export const CARD_RADIUS_PX = 12;

export type SeamEdges = { topCut: boolean; bottomCut: boolean };

export type CutPlan = {
    key: string;
    opening: boolean;
    hasSeams: boolean;
    seams: ReadonlyMap<string, SeamEdges>;
    fullBleedRowKeys: ReadonlySet<string>;
    mastheadDivider: boolean;
};

export type CutPhaseName =
    | 'idle'
    | 'dividing'
    | 'cut'
    | 'separating'
    | 'closing'
    | 'merging';

// phase and plan travel together so the timer effect keyed on this object gets a fresh identity per
// transition; a bare [phase] key would not re-run on a same-phase restart and would leak the old timer.
export type CutState = { phase: CutPhaseName; plan: CutPlan | null };

export const IDLE_CUT: CutState = { phase: 'idle', plan: null };

type CutFlip = { key: string; opening: boolean };

export type DividerState = 'inset' | 'full-bleed' | 'retracting';

export type ContentState = 'hold' | 'open' | 'closing';

type MergedPosition = { segmentIndex: number; row: number };

function markEdge(seams: Map<string, SeamEdges>, key: string, edge: keyof SeamEdges): void {
    const edges = seams.get(key) ?? { topCut: false, bottomCut: false };
    edges[edge] = true;
    seams.set(key, edges);
}

// Masthead sits at merged index 0 on both partitions even when its run is empty, so its
// container never comes from a source lookup the way every other segment's does.
function mergedContainerAbove<T extends { key: string }>(
    segment: SourceSegment<T>,
    positions: ReadonlyMap<string, MergedPosition>,
): number {
    if (segment.masthead) return 0;
    const last = segment.sources[segment.sources.length - 1];
    const position = positions.get(last.key);
    if (!position) throw new Error(`source not found in merged partition: ${last.key}`);
    return position.segmentIndex;
}

export function planCut<T extends { key: string }>(
    sources: T[],
    expandedKeys: ReadonlySet<string>,
    key: string,
): CutPlan {
    const opening = !expandedKeys.has(key);

    const splitKeys = new Set(expandedKeys);
    splitKeys.add(key);
    const mergedKeys = new Set(expandedKeys);
    mergedKeys.delete(key);

    const split = partitionSourceSegments(sources, splitKeys);
    const merged = partitionSourceSegments(sources, mergedKeys);

    const positions = new Map<string, MergedPosition>();
    merged.forEach((segment, segmentIndex) => {
        segment.sources.forEach((source, row) => {
            positions.set(source.key, { segmentIndex, row });
        });
    });

    const seams = new Map<string, SeamEdges>();
    const fullBleedRowKeys = new Set<string>();
    let mastheadDivider = false;

    for (let i = 0; i < split.length - 1; i++) {
        const above = split[i];
        const below = split[i + 1];

        // below is never the masthead segment (that's always index 0), so it always leads with a real source.
        const dividerKey = below.sources[0].key;
        const belowPosition = positions.get(dividerKey);
        if (!belowPosition) throw new Error(`source not found in merged partition: ${dividerKey}`);

        if (mergedContainerAbove(above, positions) !== belowPosition.segmentIndex) continue;

        markEdge(seams, above.key, 'bottomCut');
        markEdge(seams, below.key, 'topCut');

        if (belowPosition.row > 0) {
            fullBleedRowKeys.add(dividerKey);
        } else {
            mastheadDivider = true;
        }
    }

    return { key, opening, hasSeams: seams.size > 0, seams, fullBleedRowKeys, mastheadDivider };
}

// Opening with seams runs the divider pre-phase first; a seamless open flips at once and
// jumps straight to the committed cut frame.
export function cutStart(plan: CutPlan): { state: CutState; flipNow: boolean } {
    if (!plan.opening) return { state: { phase: 'closing', plan }, flipNow: false };
    if (plan.hasSeams) return { state: { phase: 'dividing', plan }, flipNow: false };
    return { state: { phase: 'cut', plan }, flipNow: true };
}

// dividing and closing hold their key flip until their timer fires, so a fast-forward must apply it.
export function pendingFlip(state: CutState): CutFlip | null {
    if (!state.plan) return null;
    if (state.phase === 'dividing' || state.phase === 'closing') {
        return { key: state.plan.key, opening: state.plan.opening };
    }
    return null;
}

type CutStep =
    | { kind: 'frame'; next: CutState }
    | { kind: 'timer'; ms: number; flip: CutFlip | null; next: CutState };

// One scheduled advance per state: what to wait for, the deferred key flip that lands with it, and the next state.
export function cutStep(state: CutState): CutStep | null {
    const { phase, plan } = state;
    if (phase === 'idle' || !plan) return null;
    if (phase === 'dividing') {
        return { kind: 'timer', ms: DIVIDER_MS, flip: { key: plan.key, opening: true }, next: { phase: 'cut', plan } };
    }
    if (phase === 'cut') return { kind: 'frame', next: { phase: 'separating', plan } };
    if (phase === 'separating') return { kind: 'timer', ms: SPLIT_DURATION_MS, flip: null, next: IDLE_CUT };
    if (phase === 'closing') {
        return {
            kind: 'timer',
            ms: SPLIT_DURATION_MS,
            flip: { key: plan.key, opening: false },
            next: plan.hasSeams ? { phase: 'merging', plan } : IDLE_CUT,
        };
    }
    return { kind: 'timer', ms: DIVIDER_MS, flip: null, next: IDLE_CUT };
}

type SegmentCutStyle = {
    marginTop: number;
    borderTopLeftRadius: number;
    borderTopRightRadius: number;
    borderBottomLeftRadius: number;
    borderBottomRightRadius: number;
};

// Fresh-mounted articles have no layout snapshot; real margins/radii keep the 1px shadow ring crisp where FLIP would scale it.
export function segmentCutStyle(state: CutState, segmentKey: string): SegmentCutStyle {
    const seamed = state.phase === 'cut' || state.phase === 'closing';
    const edges = seamed ? state.plan?.seams.get(segmentKey) : undefined;
    const top = edges?.topCut ? 0 : CARD_RADIUS_PX;
    const bottom = edges?.bottomCut ? 0 : CARD_RADIUS_PX;
    return {
        marginTop: edges?.topCut ? -SEAM_OVERLAP_PX : 0,
        borderTopLeftRadius: top,
        borderTopRightRadius: top,
        borderBottomLeftRadius: bottom,
        borderBottomRightRadius: bottom,
    };
}

// Breakout width is withheld across the cut so it tweens out on separating and back in on closing.
export function segmentBreaksOut(
    state: CutState,
    segment: Pick<SourceSegment, 'key' | 'expanded'>,
): boolean {
    if (!segment.expanded) return false;
    const withheld =
        state.plan?.key === segment.key &&
        (state.phase === 'cut' || state.phase === 'closing');
    return !withheld;
}

export function cutSnaps(phase: CutPhaseName): boolean {
    return phase === 'cut' || phase === 'merging';
}

function dividerStateFor(fullBleed: boolean, phase: CutPhaseName): DividerState {
    if (!fullBleed) return 'inset';
    if (phase === 'dividing') return 'full-bleed';
    if (phase === 'merging') return 'retracting';
    return 'inset';
}

export function mastheadDividerState(state: CutState): DividerState {
    return dividerStateFor(state.plan?.mastheadDivider ?? false, state.phase);
}

export function rowDividerState(state: CutState, rowKey: string): DividerState {
    return dividerStateFor(state.plan?.fullBleedRowKeys.has(rowKey) ?? false, state.phase);
}

// 'hold' pins the opening row's content at its pre-tween target through the committed cut frame.
export function rowContentState(state: CutState, rowKey: string): ContentState {
    if (state.plan?.key !== rowKey) return 'open';
    if (state.phase === 'cut' && state.plan.opening) return 'hold';
    if (state.phase === 'closing') return 'closing';
    return 'open';
}
