import type { ComponentProps } from 'react';

import { cn } from 'cn';
import '@/pages/reader.css';

// Article HTML in the reader's typography (src/pages/reader.css); clamped shows the first five lines.
function ReaderBody({ clamped = false, ...props }: Omit<ComponentProps<'div'>, 'className' | 'style'> & { clamped?: boolean }) {
    return <div data-slot="reader-body" className={cn('reader-body', clamped && 'line-clamp-5')} {...props} />;
}

export { ReaderBody };
