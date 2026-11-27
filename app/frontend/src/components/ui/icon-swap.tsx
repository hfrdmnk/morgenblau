import { motion, type HTMLMotionProps } from 'motion/react';

function IconSwapLayer(props: Omit<HTMLMotionProps<'span'>, 'className' | 'style'>) {
    return <motion.span className="absolute flex size-5 items-center justify-center" {...props} />;
}

export { IconSwapLayer };
