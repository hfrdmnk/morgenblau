import { motion } from 'motion/react';
import type { MouseEvent, ReactNode } from 'react';

import { DiscoverDivider } from '@/components/discover/discover-divider';
import type { ContentState, DividerState } from '@/lib/discover-cut';
import { CUT_SNAP, split, splitOpen } from '@/lib/motion-transitions';
import { cn } from '@/lib/utils';

// Convenience hit area shared by discover rows; the head control owns the toggle semantics,
// inner links and buttons keep their own.
export function DiscoverRowShell({
    showDivider,
    dividerState,
    clickable,
    onToggle,
    children,
}: {
    showDivider: boolean;
    dividerState: DividerState;
    clickable: boolean;
    onToggle: () => void;
    children: ReactNode;
}) {
    const onRowClick = (event: MouseEvent<HTMLDivElement>) => {
        if (event.target instanceof Element && event.target.closest('a, button')) return;
        onToggle();
    };

    return (
        <li className="list-none">
            {showDivider ? <DiscoverDivider state={dividerState} /> : null}
            <div
                className={cn('px-5 py-4', clickable && 'cursor-pointer')}
                onClick={onRowClick}
            >
                {children}
            </div>
        </li>
    );
}

export function DiscoverRowFooter({
    expanded,
    contentState,
    reason,
    action,
}: {
    expanded: boolean;
    contentState: ContentState;
    reason: ReactNode;
    action: ReactNode;
}) {
    const open = expanded && contentState === 'open';
    return (
        <>
            {expanded ? <FooterRule contentState={contentState} /> : null}
            <motion.div
                initial={false}
                animate={
                    open
                        ? { marginTop: 0, paddingTop: 12 }
                        : { marginTop: 8, paddingTop: 0 }
                }
                transition={footerTransition(contentState)}
                className="flex items-center justify-between gap-4"
            >
                <div className="min-w-0">{reason}</div>
                {action}
            </motion.div>
        </>
    );
}

// Draws in with the release on open, folds two-sided with the merge on close.
function FooterRule({ contentState }: { contentState: ContentState }) {
    const open = contentState === 'open';
    return (
        <motion.div
            initial={open ? false : { scaleX: 0, marginTop: 0 }}
            animate={{
                scaleX: open ? 1 : 0,
                marginTop: open ? 16 : 0,
            }}
            transition={footerTransition(contentState)}
            aria-hidden
            className="border-t border-border"
        />
    );
}

function footerTransition(contentState: ContentState) {
    if (contentState === 'hold') return CUT_SNAP;
    if (contentState === 'closing') return split();
    return splitOpen();
}
