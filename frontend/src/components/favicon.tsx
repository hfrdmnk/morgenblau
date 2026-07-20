import { GlobeIcon } from '@proicons/react';
import { useState } from 'react';

import { cn } from '@/lib/utils';

export function Favicon({
    src,
    className,
}: {
    src: string | null | undefined;
    className?: string;
}) {
    const [prevSrc, setPrevSrc] = useState(src);
    const [errored, setErrored] = useState(false);
    if (src !== prevSrc) {
        setPrevSrc(src);
        setErrored(false);
    }

    if (!src || errored) {
        return (
            <GlobeIcon
                className={cn('size-4 text-muted-foreground', className)}
            />
        );
    }

    return (
        <img
            src={src}
            alt=""
            className={cn('size-4 rounded-sm object-cover', className)}
            onError={() => setErrored(true)}
            loading="lazy"
        />
    );
}
