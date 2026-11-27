import type { ComponentProps } from 'react';

import { cn } from 'cn';

function PlayerSlot({ playing, ...props }: Omit<ComponentProps<'div'>, 'className' | 'style'> & { playing: boolean }) {
    return (
        <div
            data-slot="player-slot"
            className={cn('-mt-3 bg-background pt-3', playing && 'sticky top-0 z-10')}
            {...props}
        />
    );
}

function VideoEmbed(props: Omit<ComponentProps<'iframe'>, 'className' | 'style'>) {
    return (
        <div data-slot="video-embed" className="aspect-video overflow-hidden rounded-lg bg-muted">
            <iframe className="h-full w-full border-0" {...props} />
        </div>
    );
}

function VideoPlayer(props: Omit<ComponentProps<'video'>, 'className' | 'style'>) {
    return <video data-slot="video-player" className="aspect-video w-full rounded-lg bg-muted" {...props} />;
}

export { PlayerSlot, VideoEmbed, VideoPlayer };
