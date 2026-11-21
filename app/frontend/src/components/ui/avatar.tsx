import BoringAvatar from 'boring-avatars';
import type { ComponentProps } from 'react';

import { cn } from 'cn';

const sizes = { sm: 'size-7', md: 'size-8', lg: 'size-10', xl: 'size-14' };

// A round frame its image or generated face fills; blank paints the empty circle that stands in when neither can show.
function Avatar({
    size = 'md',
    blank = false,
    ...props
}: Omit<ComponentProps<'span'>, 'className' | 'style'> & {
    size?: keyof typeof sizes;
    blank?: boolean;
}) {
    return (
        <span
            className={cn('flex shrink-0 overflow-hidden rounded-full', sizes[size], blank && 'bg-muted')}
            {...props}
        />
    );
}

function AvatarImage(props: Omit<ComponentProps<'img'>, 'className' | 'style'>) {
    return <img alt="" className="size-full object-cover" {...props} />;
}

// Tailwind rose-500, orange-500, amber-400, lime-500, emerald-500, sky-500, violet-500 and fuchsia-500, as hex because boring-avatars derives the face's contrast colour from hex.
const FACE_COLORS = ['#ff2056', '#ff6900', '#ffb900', '#7ccf00', '#00bc7d', '#00a6f4', '#8e51ff', '#e12afb'];

function AvatarFace({ seed }: { seed: string }) {
    return (
        <BoringAvatar
            aria-hidden="true"
            className="size-full"
            colors={FACE_COLORS}
            name={seed}
            size={32}
            variant="beam"
        />
    );
}

export { Avatar, AvatarFace, AvatarImage };
export type AvatarSize = keyof typeof sizes;
