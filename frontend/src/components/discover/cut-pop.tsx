import { motion } from 'motion/react';
import type { Transition } from 'motion/react';
import type { ReactNode } from 'react';

import type { ContentState } from '@/lib/discover-cut';
import { CUT_SNAP, cutConcealPop, cutStaggerPop } from '@/lib/motion-transitions';
import { cn } from '@/lib/utils';

// Cut-driven, never mount-driven: re-parented rows remount mid-cut and must not replay an entrance.
// Mounts hidden on the committed cut frame ('hold') and pops when 'open' lands.
export function CutPop({
    contentState,
    order,
    lastOrder = 0,
    className,
    children,
}: {
    contentState: ContentState;
    order: number;
    lastOrder?: number;
    className?: string;
    children: ReactNode;
}) {
    const open = contentState === 'open';
    return (
        <motion.div
            initial={false}
            animate={open ? { scale: 1, opacity: 1 } : { scale: 0.5, opacity: 0 }}
            transition={popTransition(contentState, order, lastOrder)}
            className={cn('flex shrink-0', className)}
        >
            {children}
        </motion.div>
    );
}

// Closing runs the entrance stagger in reverse (last in, first out), staying ahead of the fold.
function popTransition(
    contentState: ContentState,
    order: number,
    lastOrder: number,
): Transition {
    if (contentState === 'hold') return CUT_SNAP;
    if (contentState === 'closing') return cutConcealPop(lastOrder - order);
    return cutStaggerPop(order);
}
