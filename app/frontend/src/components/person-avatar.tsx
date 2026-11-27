import { useState } from 'react';

import { Avatar, AvatarFace, AvatarImage, type AvatarSize } from '@/components/ui/avatar';

export function PersonAvatar({
    seed,
    src = null,
    lazy = false,
    size = 'md',
}: {
    seed: string;
    src?: string | null;
    lazy?: boolean;
    size?: AvatarSize;
}) {
    const [failed, setFailed] = useState<string | null>(null);
    return (
        <Avatar size={size}>
            {src && failed !== src ? (
                <AvatarImage
                    loading={lazy ? 'lazy' : undefined}
                    onError={() => setFailed(src)}
                    referrerPolicy="no-referrer"
                    src={src}
                />
            ) : (
                <AvatarFace seed={seed} />
            )}
        </Avatar>
    );
}
