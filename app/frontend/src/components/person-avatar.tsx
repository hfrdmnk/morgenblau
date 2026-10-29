import BoringAvatar from 'boring-avatars';
import { useState } from 'react';

// The brand mode colours and two Stone neutrals from index.css, as hex because boring-avatars derives the face's contrast colour from hex.
const COLORS = ['#006cff', '#f34600', '#9754ed', '#d6d3d1', '#292524'];

// Seeded by DID so a person keeps the same face when their handle or name changes.
export function PersonAvatar({ did, src, lazy = false }: { did: string; src: string | null; lazy?: boolean }) {
    const [failed, setFailed] = useState<string | null>(null);
    return (
        <span className="flex size-8 shrink-0 overflow-hidden rounded-full">
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
                <BoringAvatar aria-hidden="true" className="size-full" colors={COLORS} name={did} size={32} variant="beam" />
            )}
        </span>
    );
}
