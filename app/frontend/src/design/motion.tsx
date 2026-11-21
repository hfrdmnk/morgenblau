import { motion } from 'motion/react';

import { Frame } from '@/design/frame';

// An animated Frame: the same token props, plus motion's animation props.
export const MotionFrame = motion.create(Frame);
