import { Globe02Icon } from '@hugeicons/core-free-icons';
import { HugeiconsIcon } from '@hugeicons/react';
import { useState } from 'react';

import { cn } from '@/lib/utils';

export function Favicon({
    src,
    className,
}: {
    src: string | null | undefined;
    className?: string;
}) {
    const [errored, setErrored] = useState(false);

    if (!src || errored) {
        return (
            <HugeiconsIcon
                icon={Globe02Icon}
                className={cn('size-4 text-muted-foreground', className)}
            />
        );
    }

    return (
        <img
            src={src}
            alt=""
            className={cn('size-4 rounded-sm', className)}
            onError={() => setErrored(true)}
            loading="lazy"
        />
    );
}
