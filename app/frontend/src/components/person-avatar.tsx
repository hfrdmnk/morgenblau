import BoringAvatar from 'boring-avatars';
import { useState } from 'react';

import { cn } from '@/lib/utils';

// The brand mode colours and two Stone neutrals from index.css, as hex because boring-avatars derives the face's contrast colour from hex.
const COLORS = ['#006cff', '#f34600', '#9754ed', '#d6d3d1', '#292524'];

// Seed with something stable, like a DID or a site's host, so the face survives a changed handle or name.
export function PersonAvatar({ seed, src = null, lazy = false, className }: { seed: string; src?: string | null; lazy?: boolean; className?: string }) {
    const [failed, setFailed] = useState<string | null>(null);
    return (
        <span className={cn('flex size-8 shrink-0 overflow-hidden rounded-full', className)}>
            {src && failed !== src ? (
                <img
                    alt=""
                    className="size-full object-cover"
                    loading={lazy ? 'lazy' : undefined}
                    onError={() => setFailed(src)}
                    referrerPolicy="no-referrer"
                    src={src}
                />
            ) : (
                <BoringAvatar aria-hidden="true" className="size-full" colors={COLORS} name={seed} size={32} variant="beam" />
            )}
        </span>
    );
}
