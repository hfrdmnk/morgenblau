import { motion, type HTMLMotionProps } from 'motion/react';

// One icon of a crossfade: stacked over the other in a 20px box, so the parent that holds both must be `relative`.
function IconSwapLayer(props: Omit<HTMLMotionProps<'span'>, 'className' | 'style'>) {
    return <motion.span className="absolute flex size-5 items-center justify-center" {...props} />;
}

export { IconSwapLayer };
