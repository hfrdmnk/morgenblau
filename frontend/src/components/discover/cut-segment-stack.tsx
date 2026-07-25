import type { ReactNode } from 'react';
import { motion, MotionConfig } from 'motion/react';

import { DiscoverDivider } from '@/components/discover/discover-divider';
import type { DiscoverCut } from '@/hooks/use-discover-cut';
import {
    cutSnaps,
    mastheadDividerState,
    rowContentState,
    rowDividerState,
    segmentBreaksOut,
    segmentCutStyle,
    type ContentState,
    type CutPhaseName,
    type DividerState,
} from '@/lib/discover-cut';
import type { SourceSegment } from '@/lib/discover-segments';
import { CUT_SNAP, split, splitOpen } from '@/lib/motion-transitions';
import { cn } from '@/lib/utils';

export type CutRowContext = {
    showDivider: boolean;
    dividerState: DividerState;
    contentState: ContentState;
    intentExpanded: boolean;
};

export type CutSegmentStackProps<T extends { key: string }> = {
    cut: DiscoverCut;
    segments: SourceSegment<T>[];
    masthead?: ReactNode;
    renderRow: (item: T, row: CutRowContext) => ReactNode;
};

export function CutSegmentStack<T extends { key: string }>({
    cut,
    segments,
    masthead,
    renderRow,
}: CutSegmentStackProps<T>) {
    const visibleSegments = segments.filter(
        (segment) =>
            segment.sources.length > 0 || Boolean(segment.masthead && masthead),
    );
    return (
        <MotionConfig reducedMotion="user">
            <div className="flex flex-col gap-4">
                {visibleSegments.map((segment) => {
                    const elevated = segmentBreaksOut(cut, segment);
                    return (
                        <motion.article
                            key={segment.key}
                            initial={false}
                            animate={segmentCutStyle(cut, segment.key)}
                            transition={articleTransition(cut.phase)}
                            className={cn(
                                'bg-card motion-reduce:transition-none',
                                articleTransitionClass(cut.phase),
                                elevated ? 'shadow-card-elevated' : 'shadow-card',
                                elevated && '-mx-2 sm:-mx-4 md:-mx-10',
                            )}
                        >
                            <SegmentMasthead
                                cut={cut}
                                segment={segment}
                                masthead={masthead}
                            />
                            <ul className="flex list-none flex-col">
                                {segment.sources.map((item, rowIndex) =>
                                    renderRow(item, {
                                        showDivider: rowIndex > 0,
                                        dividerState: rowDividerState(
                                            cut,
                                            item.key,
                                        ),
                                        contentState: rowContentState(
                                            cut,
                                            item.key,
                                        ),
                                        intentExpanded: cut.intentExpanded(
                                            item.key,
                                        ),
                                    }),
                                )}
                            </ul>
                        </motion.article>
                    );
                })}
            </div>
        </MotionConfig>
    );
}

function SegmentMasthead<T extends { key: string }>({
    cut,
    segment,
    masthead,
}: {
    cut: DiscoverCut;
    segment: SourceSegment<T>;
    masthead?: ReactNode;
}) {
    if (!segment.masthead || !masthead) return null;
    return (
        <div>
            {masthead}
            {segment.sources.length > 0 ? (
                <DiscoverDivider state={mastheadDividerState(cut)} />
            ) : null}
        </div>
    );
}

// Separating releases (ease-out) so the halves move the instant the cut commits; closing settles two-sided.
function articleTransition(phase: CutPhaseName) {
    if (cutSnaps(phase)) return CUT_SNAP;
    return phase === 'separating' ? splitOpen() : split();
}

// One shorthand: ease-[...] would broadcast a single timing function over the shadow's own 180ms clock.
// Full literals, not interpolated: Tailwind's scanner only sees verbatim class candidates.
const ARTICLE_TRANSITION_SEPARATING =
    '[transition:margin-left_var(--cut-split-ms)_cubic-bezier(0.23,1,0.32,1),margin-right_var(--cut-split-ms)_cubic-bezier(0.23,1,0.32,1),box-shadow_180ms_cubic-bezier(0.215,0.61,0.355,1)]';
const ARTICLE_TRANSITION_DEFAULT =
    '[transition:margin-left_var(--cut-split-ms)_cubic-bezier(0.645,0.045,0.355,1),margin-right_var(--cut-split-ms)_cubic-bezier(0.645,0.045,0.355,1),box-shadow_180ms_cubic-bezier(0.215,0.61,0.355,1)]';

function articleTransitionClass(phase: CutPhaseName): string {
    return phase === 'separating'
        ? ARTICLE_TRANSITION_SEPARATING
        : ARTICLE_TRANSITION_DEFAULT;
}
