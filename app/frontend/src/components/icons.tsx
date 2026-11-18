import { IconSunrise as IconSunriseFilled } from '@central-icons-react/round-filled-radius-2-stroke-1.5/IconSunrise';
import { IconSunset as IconSunsetFilled } from '@central-icons-react/round-filled-radius-2-stroke-1.5/IconSunset';
import { IconSunrise } from '@central-icons-react/round-outlined-radius-2-stroke-1.5/IconSunrise';
import { IconSunset } from '@central-icons-react/round-outlined-radius-2-stroke-1.5/IconSunset';
import type { ComponentProps } from 'react';

import { cn } from '@/lib/utils';

export { IconBooks as LibraryActiveIcon } from '@central-icons-react/round-filled-radius-2-stroke-1.5/IconBooks';
export { IconLayersThree as SourcesActiveIcon } from '@central-icons-react/round-filled-radius-2-stroke-1.5/IconLayersThree';
export { IconArrowCornerDownLeft as EnterIcon } from '@central-icons-react/round-outlined-radius-2-stroke-1.5/IconArrowCornerDownLeft';
export { IconArrowDownWall as DownloadIcon } from '@central-icons-react/round-outlined-radius-2-stroke-1.5/IconArrowDownWall';
export { IconArrowRight as ArrowRightIcon } from '@central-icons-react/round-outlined-radius-2-stroke-1.5/IconArrowRight';
export { IconArrowUp as ArrowUpIcon } from '@central-icons-react/round-outlined-radius-2-stroke-1.5/IconArrowUp';
export { IconArrowUpWall as UploadIcon } from '@central-icons-react/round-outlined-radius-2-stroke-1.5/IconArrowUpWall';
export { IconBookmark as BookmarkIcon } from '@central-icons-react/round-outlined-radius-2-stroke-1.5/IconBookmark';
export { IconBooks as LibraryIcon } from '@central-icons-react/round-outlined-radius-2-stroke-1.5/IconBooks';
export { IconBubbleText as FeedbackIcon } from '@central-icons-react/round-outlined-radius-2-stroke-1.5/IconBubbleText';
export { IconCheckmark1 as CheckIcon } from '@central-icons-react/round-outlined-radius-2-stroke-1.5/IconCheckmark1';
export { IconChevronDownMedium as ChevronDownIcon } from '@central-icons-react/round-outlined-radius-2-stroke-1.5/IconChevronDownMedium';
export { IconCircleInfo as InfoIcon } from '@central-icons-react/round-outlined-radius-2-stroke-1.5/IconCircleInfo';
export { IconCrossLarge as CloseIcon } from '@central-icons-react/round-outlined-radius-2-stroke-1.5/IconCrossLarge';
export { IconDotGrid1x3Horizontal as MoreIcon } from '@central-icons-react/round-outlined-radius-2-stroke-1.5/IconDotGrid1x3Horizontal';
export { IconExclamationTriangle as AlertIcon } from '@central-icons-react/round-outlined-radius-2-stroke-1.5/IconExclamationTriangle';
export { IconImages2 as ImageIcon } from '@central-icons-react/round-outlined-radius-2-stroke-1.5/IconImages2';
export { IconLayersThree as SourcesIcon } from '@central-icons-react/round-outlined-radius-2-stroke-1.5/IconLayersThree';
export { IconInboxEmpty as InboxIcon } from '@central-icons-react/round-outlined-radius-2-stroke-1.5/IconInboxEmpty';
export { IconLoader as LoadingIcon } from '@central-icons-react/round-outlined-radius-2-stroke-1.5/IconLoader';
export { IconPencil as PenIcon } from '@central-icons-react/round-outlined-radius-2-stroke-1.5/IconPencil';
export { IconPlusMedium as PlusIcon } from '@central-icons-react/round-outlined-radius-2-stroke-1.5/IconPlusMedium';
export { IconSettingsGear1 as SettingsIcon } from '@central-icons-react/round-outlined-radius-2-stroke-1.5/IconSettingsGear1';
export { IconShield as ShieldIcon } from '@central-icons-react/round-outlined-radius-2-stroke-1.5/IconShield';
export { IconSquareArrowTopRight as ExternalLinkIcon } from '@central-icons-react/round-outlined-radius-2-stroke-1.5/IconSquareArrowTopRight';
export { IconSquareBehindSquare1 as CopyIcon } from '@central-icons-react/round-outlined-radius-2-stroke-1.5/IconSquareBehindSquare1';

type CentralIconProps = ComponentProps<typeof IconSunrise>;

export function DigestIcon({ className, ...props }: CentralIconProps) {
    return (
        <>
            <IconSunrise className={cn(className, 'dark:hidden')} {...props} />
            <IconSunset className={cn(className, 'hidden dark:block')} {...props} />
        </>
    );
}

export function DigestActiveIcon({ className, ...props }: CentralIconProps) {
    return (
        <>
            <IconSunriseFilled className={cn(className, 'dark:hidden')} {...props} />
            <IconSunsetFilled className={cn(className, 'hidden dark:block')} {...props} />
        </>
    );
}

export function MorgenblauMark(props: ComponentProps<'svg'>) {
    return (
        <svg aria-hidden focusable={false} viewBox="0 0 224 144" {...props}>
            <path
                d="M124 63.7156C124 70.8428 132.617 74.4122 137.657 69.3724L182.711 24.3184L199.682 41.2891L156.004 84.9663C150.77 90.2004 154.878 100.034 162.28 100.02C169.352 100.007 175.552 100 176 100H224V124H144C132.954 124 124 132.954 124 144H100C100 132.954 91.0457 124 80 124H0V100H48C48.4476 100 54.6478 100.007 61.7203 100.02C69.1224 100.034 73.2302 90.2009 67.9962 84.9669L24.3184 41.2891L41.2891 24.3184L86.3431 69.3724C91.3829 74.4122 100 70.8428 100 63.7156V0H124V63.7156Z"
                fill="currentColor"
            />
        </svg>
    );
}
