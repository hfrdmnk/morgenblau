import { AnimatePresence, motion } from 'motion/react';
import type { ComponentType } from 'react';

/* Icon swap storyboard, one stage:
 *   0ms  outgoing icon blurs to 3px and scales to 0.9 while the next one resolves in place, over 160ms
 */
const TIMING = { swap: 0.16 };
const EASE = [0.16, 1, 0.3, 1] as const;
const ICON = {
    hidden: (instant: boolean) => ({
        opacity: 0,
        filter: instant ? 'blur(0px)' : 'blur(3px)',
        transform: instant ? 'scale(1)' : 'scale(0.9)',
        transition: { duration: instant ? 0 : TIMING.swap, ease: EASE },
    }),
    visible: { opacity: 1, filter: 'blur(0px)', transform: 'scale(1)' },
};

// The parent positions the icons, so it must be `relative`; both icons overlap during the swap.
export function IconCrossfade({
    name,
    icon: Icon,
    instant,
    className = 'size-5',
}: {
    name: string;
    className?: string;
    icon: ComponentType<{ className?: string }>;
    instant: boolean;
}) {
    return (
        <AnimatePresence initial={false} custom={instant}>
            <motion.span
                key={name}
                className="absolute flex size-5 items-center justify-center"
                custom={instant}
                variants={ICON}
                initial={instant ? false : 'hidden'}
                animate="visible"
                exit="hidden"
                transition={{ duration: instant ? 0 : TIMING.swap, ease: EASE }}
            >
                <Icon className={className} />
            </motion.span>
        </AnimatePresence>
    );
}
