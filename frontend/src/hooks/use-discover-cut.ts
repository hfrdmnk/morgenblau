import { useEffect, useState } from 'react';
import type { Dispatch, SetStateAction } from 'react';
import { useReducedMotion } from 'motion/react';

import {
    cutStart,
    cutStep,
    IDLE_CUT,
    pendingFlip,
    planCut,
    type CutPhaseName,
    type CutPlan,
    type CutState,
} from '@/lib/discover-cut';

type UseDiscoverCutOptions<T extends { key: string }> = {
    sources: T[];
    expandedKeys: ReadonlySet<string>;
    setExpandedKeys: Dispatch<SetStateAction<ReadonlySet<string>>>;
};

export type DiscoverCut = {
    phase: CutPhaseName;
    plan: CutPlan | null;
    toggle: (key: string) => boolean;
    settle: () => void;
    intentExpanded: (key: string) => boolean;
};

function flipKey(
    prev: ReadonlySet<string>,
    key: string,
    opening: boolean,
): ReadonlySet<string> {
    const next = new Set(prev);
    if (opening) next.add(key);
    else next.delete(key);
    return next;
}

export function useDiscoverCut<T extends { key: string }>(opts: UseDiscoverCutOptions<T>): DiscoverCut {
    const { sources, expandedKeys, setExpandedKeys } = opts;
    const reducedMotion = useReducedMotion();
    const [cutState, setCutState] = useState<CutState>(IDLE_CUT);

    useEffect(() => {
        const step = cutStep(cutState);
        if (!step) return;

        // A committed 'cut' frame lets consumers paint pre-tween targets before 'separating' retargets them.
        if (step.kind === 'frame') {
            let raf2 = 0;
            const raf1 = requestAnimationFrame(() => {
                raf2 = requestAnimationFrame(() => setCutState(step.next));
            });
            return () => {
                cancelAnimationFrame(raf1);
                cancelAnimationFrame(raf2);
            };
        }

        const id = setTimeout(() => {
            if (step.flip) {
                const { key, opening } = step.flip;
                setExpandedKeys((prev) => flipKey(prev, key, opening));
            }
            setCutState(step.next);
        }, step.ms);
        return () => clearTimeout(id);
    }, [cutState, setExpandedKeys]);

    // Land any in-flight sequence at idle now: commit its deferred flip and drop the plan so its timer's cleanup fires.
    const settle = () => {
        if (cutState.phase === 'idle') return;
        const pending = pendingFlip(cutState);
        if (pending) {
            setExpandedKeys((prev) => flipKey(prev, pending.key, pending.opening));
        }
        setCutState(IDLE_CUT);
    };

    const toggle = (key: string): boolean => {
        if (reducedMotion) {
            const opening = !expandedKeys.has(key);
            setExpandedKeys((prev) => flipKey(prev, key, opening));
            return opening;
        }

        // Props are stale after the queued setExpandedKeys below, so track the settled set locally instead.
        let effectiveKeys = expandedKeys;
        const pending = pendingFlip(cutState);
        if (pending) {
            effectiveKeys = flipKey(effectiveKeys, pending.key, pending.opening);
            setExpandedKeys((prev) => flipKey(prev, pending.key, pending.opening));
        }

        const plan = planCut(sources, effectiveKeys, key);
        const start = cutStart(plan);
        if (start.flipNow) {
            setExpandedKeys((prev) => flipKey(prev, key, true));
        }
        setCutState(start.state);
        return plan.opening;
    };

    const intentExpanded = (key: string): boolean => {
        const { phase, plan } = cutState;
        if (plan && phase !== 'idle' && plan.key === key) return plan.opening;
        return expandedKeys.has(key);
    };

    return { phase: cutState.phase, plan: cutState.plan, toggle, settle, intentExpanded };
}
