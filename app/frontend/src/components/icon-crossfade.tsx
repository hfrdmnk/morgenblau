import { AnimatePresence } from 'motion/react';
import type { ComponentType } from 'react';

import type { IconProps } from '@/components/icons';
import { IconSwapLayer } from '@/components/ui/icon-swap';

const TIMING = { swap: 0.16 };
const SWAP = (instant: boolean) => ({ duration: instant ? 0 : TIMING.swap, ease: [0.16, 1, 0.3, 1] as const });
const ICON = {
    hidden: (instant: boolean) => ({
        opacity: 0,
        filter: instant ? 'blur(0px)' : 'blur(3px)',
        transform: instant ? 'scale(1)' : 'scale(0.9)',
        transition: SWAP(instant),
    }),
    visible: { opacity: 1, filter: 'blur(0px)', transform: 'scale(1)' },
};

export function IconCrossfade({
    name,
    icon: Icon,
    instant,
    size = 'xl',
}: {
    name: string;
    size?: IconProps['size'];
    icon: ComponentType<IconProps>;
    instant: boolean;
}) {
    return (
        <AnimatePresence initial={false} custom={instant}>
            <IconSwapLayer
                key={name}
                custom={instant}
                variants={ICON}
                initial={instant ? false : 'hidden'}
                animate="visible"
                exit="hidden"
                transition={SWAP(instant)}
            >
                <Icon size={size} />
            </IconSwapLayer>
        </AnimatePresence>
    );
}
