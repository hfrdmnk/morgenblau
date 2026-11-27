import type { ReactNode } from 'react';

import { MorgenblauMark } from '@/components/icons';
function MarkLine({ label, children, actions }: { label: string; children?: ReactNode; actions?: ReactNode }) {
    return (
        <div data-slot="mark-line" className="flex min-h-10 items-center gap-3">
            <div className="flex min-w-0 grow items-baseline gap-2.5 text-sm text-subtle-foreground">
                <span className="shrink-0 mark-on-horizon">
                    <MorgenblauMark aria-label={label} role="img" size="md" />
                </span>
                {children ? <span className="min-w-0 truncate">{children}</span> : null}
            </div>
            {actions ? <div className="-me-1.5 flex shrink-0 items-center gap-0.5">{actions}</div> : null}
        </div>
    );
}

export { MarkLine };
