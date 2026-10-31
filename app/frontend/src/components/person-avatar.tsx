import BoringAvatar from 'boring-avatars';
import { useState } from 'react';

import { cn } from '@/lib/utils';

// Tailwind rose-500, orange-500, amber-400, lime-500, emerald-500, sky-500, violet-500 and fuchsia-500, as hex because boring-avatars derives the face's contrast colour from hex.
const COLORS = ['#ff2056', '#ff6900', '#ffb900', '#7ccf00', '#00bc7d', '#00a6f4', '#8e51ff', '#e12afb'];

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
