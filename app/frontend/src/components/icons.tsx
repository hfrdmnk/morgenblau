import { IconMagicWand as CentralGeneratedIcon } from '@central-icons-react/round-filled-radius-2-stroke-1.5/IconMagicWand';
import { IconStar as CentralPrimaryIcon } from '@central-icons-react/round-filled-radius-2-stroke-1.5/IconStar';
import { IconArrowUp as CentralArrowUpIcon } from '@central-icons-react/round-outlined-radius-2-stroke-1.5/IconArrowUp';
import { IconBubbleText as CentralFeedbackIcon } from '@central-icons-react/round-outlined-radius-2-stroke-1.5/IconBubbleText';
import { IconImages2 as CentralImageIcon } from '@central-icons-react/round-outlined-radius-2-stroke-1.5/IconImages2';
import { IconArrowCornerDownLeft as CentralEnterIcon } from '@central-icons-react/round-outlined-radius-2-stroke-1.5/IconArrowCornerDownLeft';
import { IconArrowDownWall as CentralDownloadIcon } from '@central-icons-react/round-outlined-radius-2-stroke-1.5/IconArrowDownWall';
import { IconArrowRight as CentralArrowRightIcon } from '@central-icons-react/round-outlined-radius-2-stroke-1.5/IconArrowRight';
import { IconArrowUpWall as CentralUploadIcon } from '@central-icons-react/round-outlined-radius-2-stroke-1.5/IconArrowUpWall';
import { IconTag as CentralTagIcon } from '@central-icons-react/round-outlined-radius-2-stroke-1.5/IconTag';
import { IconTrashCan as CentralTrashIcon } from '@central-icons-react/round-outlined-radius-2-stroke-1.5/IconTrashCan';
import { IconBookmark as CentralBookmarkIcon } from '@central-icons-react/round-outlined-radius-2-stroke-1.5/IconBookmark';
import { IconBooks as CentralLibraryActiveIcon } from '@central-icons-react/round-filled-radius-2-stroke-1.5/IconBooks';
import { IconBooks as CentralLibraryIcon } from '@central-icons-react/round-outlined-radius-2-stroke-1.5/IconBooks';
import { IconCheckmark1 as CentralCheckIcon } from '@central-icons-react/round-outlined-radius-2-stroke-1.5/IconCheckmark1';
import { IconChevronDownMedium as CentralChevronDownIcon } from '@central-icons-react/round-outlined-radius-2-stroke-1.5/IconChevronDownMedium';
import { IconCircleInfo as CentralInfoIcon } from '@central-icons-react/round-outlined-radius-2-stroke-1.5/IconCircleInfo';
import { IconCrossLarge as CentralCloseIcon } from '@central-icons-react/round-outlined-radius-2-stroke-1.5/IconCrossLarge';
import { IconDotGrid1x3Horizontal as CentralMoreIcon } from '@central-icons-react/round-outlined-radius-2-stroke-1.5/IconDotGrid1x3Horizontal';
import { IconExclamationTriangle as CentralAlertIcon } from '@central-icons-react/round-outlined-radius-2-stroke-1.5/IconExclamationTriangle';
import { IconInboxEmpty as CentralInboxIcon } from '@central-icons-react/round-outlined-radius-2-stroke-1.5/IconInboxEmpty';
import { IconLayersThree as CentralSourcesActiveIcon } from '@central-icons-react/round-filled-radius-2-stroke-1.5/IconLayersThree';
import { IconLayersThree as CentralSourcesIcon } from '@central-icons-react/round-outlined-radius-2-stroke-1.5/IconLayersThree';
import { IconLoader as CentralLoadingIcon } from '@central-icons-react/round-outlined-radius-2-stroke-1.5/IconLoader';
import { IconPencil as CentralPenIcon } from '@central-icons-react/round-outlined-radius-2-stroke-1.5/IconPencil';
import { IconPlusMedium as CentralPlusIcon } from '@central-icons-react/round-outlined-radius-2-stroke-1.5/IconPlusMedium';
import { IconSettingsGear1 as CentralSettingsIcon } from '@central-icons-react/round-outlined-radius-2-stroke-1.5/IconSettingsGear1';
import { IconShield as CentralShieldIcon } from '@central-icons-react/round-outlined-radius-2-stroke-1.5/IconShield';
import { IconSquareArrowTopRight as CentralExternalLinkIcon } from '@central-icons-react/round-outlined-radius-2-stroke-1.5/IconSquareArrowTopRight';
import { IconSquareBehindSquare1 as CentralCopyIcon } from '@central-icons-react/round-outlined-radius-2-stroke-1.5/IconSquareBehindSquare1';
import { IconSunrise as IconSunriseFilled } from '@central-icons-react/round-filled-radius-2-stroke-1.5/IconSunrise';
import { IconSunrise } from '@central-icons-react/round-outlined-radius-2-stroke-1.5/IconSunrise';
import { IconSunset as IconSunsetFilled } from '@central-icons-react/round-filled-radius-2-stroke-1.5/IconSunset';
import { IconSunset } from '@central-icons-react/round-outlined-radius-2-stroke-1.5/IconSunset';
import type { ComponentProps } from 'react';

import { text } from '@/design/tokens';
import { cn } from 'cn';

type CentralIconProps = ComponentProps<typeof IconSunrise>;

const sizes = { xs: 'size-3', sm: 'size-3.5', md: 'size-4', lg: 'size-4.5', xl: 'size-5', '2xl': 'size-6' };

const tones = {
    ...text.tone,
    primary: 'text-primary',
    'success-icon': 'text-success-icon',
    'error-icon': 'text-error-icon',
    warning: 'text-warning',
    neutral: 'text-neutral',
    'source-primary': 'text-source-primary',
    'source-generated': 'text-source-generated',
};

// Without a size, an icon takes the one its control sets, such as Button's 16px.
export type IconProps = Omit<CentralIconProps, 'className' | 'style' | 'size'> & {
    size?: keyof typeof sizes;
    tone?: keyof typeof tones;
    spin?: boolean;
    rotate?: 90 | 180 | 270;
};

const rotations = { 90: 'rotate-90', 180: 'rotate-180', 270: '-rotate-90' };

function iconClass({ size, tone, spin, rotate }: Pick<IconProps, 'size' | 'tone' | 'spin' | 'rotate'>) {
    return cn(
        'shrink-0',
        size && sizes[size],
        tone && tones[tone],
        spin && 'motion-safe:animate-spin',
        rotate && rotations[rotate],
    );
}

function icon(Glyph: typeof IconSunrise) {
    return function Icon({ size, tone, spin, rotate, ...props }: IconProps) {
        return <Glyph className={iconClass({ size, tone, spin, rotate })} {...props} />;
    };
}

export const LibraryActiveIcon = icon(CentralLibraryActiveIcon);
export const SourcesActiveIcon = icon(CentralSourcesActiveIcon);
export const GeneratedIcon = icon(CentralGeneratedIcon);
export const PrimaryIcon = icon(CentralPrimaryIcon);
export const ArrowUpIcon = icon(CentralArrowUpIcon);
export const FeedbackIcon = icon(CentralFeedbackIcon);
export const ImageIcon = icon(CentralImageIcon);
export const EnterIcon = icon(CentralEnterIcon);
export const DownloadIcon = icon(CentralDownloadIcon);
export const ArrowRightIcon = icon(CentralArrowRightIcon);
export const UploadIcon = icon(CentralUploadIcon);
export const BookmarkIcon = icon(CentralBookmarkIcon);
export const TagIcon = icon(CentralTagIcon);
export const TrashIcon = icon(CentralTrashIcon);
export const LibraryIcon = icon(CentralLibraryIcon);
export const CheckIcon = icon(CentralCheckIcon);
export const ChevronDownIcon = icon(CentralChevronDownIcon);
export const InfoIcon = icon(CentralInfoIcon);
export const CloseIcon = icon(CentralCloseIcon);
export const MoreIcon = icon(CentralMoreIcon);
export const AlertIcon = icon(CentralAlertIcon);
export const SourcesIcon = icon(CentralSourcesIcon);
export const InboxIcon = icon(CentralInboxIcon);
export const LoadingIcon = icon(CentralLoadingIcon);
export const PenIcon = icon(CentralPenIcon);
export const PlusIcon = icon(CentralPlusIcon);
export const SettingsIcon = icon(CentralSettingsIcon);
export const ShieldIcon = icon(CentralShieldIcon);
export const ExternalLinkIcon = icon(CentralExternalLinkIcon);
export const CopyIcon = icon(CentralCopyIcon);

export function DigestIcon({ size, tone, spin, rotate, ...props }: IconProps) {
    const className = iconClass({ size, tone, spin, rotate });
    return (
        <>
            <IconSunrise className={cn(className, 'dark:hidden')} {...props} />
            <IconSunset className={cn(className, 'hidden dark:block')} {...props} />
        </>
    );
}

export function DigestActiveIcon({ size, tone, spin, rotate, ...props }: IconProps) {
    const className = iconClass({ size, tone, spin, rotate });
    return (
        <>
            <IconSunriseFilled className={cn(className, 'dark:hidden')} {...props} />
            <IconSunsetFilled className={cn(className, 'hidden dark:block')} {...props} />
        </>
    );
}

const markHeights = { md: 'h-4', xl: 'h-5', '2xl': 'h-6' };

// Sized by height, on the icon scale; the width follows the mark's proportions.
export function MorgenblauMark({
    size = '2xl',
    ...props
}: Omit<ComponentProps<'svg'>, 'className' | 'style' | 'size'> & { size?: keyof typeof markHeights }) {
    return (
        <svg aria-hidden className={markHeights[size]} focusable={false} viewBox="0 0 224 144" {...props}>
            <path
                d="M124 63.7156C124 70.8428 132.617 74.4122 137.657 69.3724L182.711 24.3184L199.682 41.2891L156.004 84.9663C150.77 90.2004 154.878 100.034 162.28 100.02C169.352 100.007 175.552 100 176 100H224V124H144C132.954 124 124 132.954 124 144H100C100 132.954 91.0457 124 80 124H0V100H48C48.4476 100 54.6478 100.007 61.7203 100.02C69.1224 100.034 73.2302 90.2009 67.9962 84.9669L24.3184 41.2891L41.2891 24.3184L86.3431 69.3724C91.3829 74.4122 100 70.8428 100 63.7156V0H124V63.7156Z"
                fill="currentColor"
            />
        </svg>
    );
}
