import type { ComponentProps } from 'react';

import { cn } from 'cn';

// The player's slot. While it plays it sticks to the top over what scrolls beneath it; the band above the player, pulled into the
// space before it, covers that text too.
function PlayerSlot({ playing, ...props }: Omit<ComponentProps<'div'>, 'className' | 'style'> & { playing: boolean }) {
    return <div data-slot="player-slot" className={cn('-mt-3 bg-background pt-3', playing && 'sticky top-0 z-10')} {...props} />;
}

// An embedded player in a 16:9 frame.
function VideoEmbed(props: Omit<ComponentProps<'iframe'>, 'className' | 'style'>) {
    return (
        <div data-slot="video-embed" className="aspect-video overflow-hidden rounded-lg bg-muted">
            <iframe className="h-full w-full border-0" {...props} />
        </div>
    );
}

// A video file in a 16:9 frame, with the browser's own controls.
function VideoPlayer(props: Omit<ComponentProps<'video'>, 'className' | 'style'>) {
    return <video data-slot="video-player" className="aspect-video w-full rounded-lg bg-muted" {...props} />;
}

export { PlayerSlot, VideoEmbed, VideoPlayer };
