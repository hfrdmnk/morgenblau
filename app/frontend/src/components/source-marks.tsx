import type { ReactNode } from 'react';

import { GeneratedIcon, PrimaryIcon } from '@/components/icons';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import { cn } from '@/lib/utils';

export function PrimaryMark({ className }: { className?: string }) {
    return <Mark label="Primary"><PrimaryIcon aria-hidden className={cn('text-source-primary', className)} /></Mark>;
}

export function GeneratedMark({ className }: { className?: string }) {
    return <Mark label="Generated feed"><GeneratedIcon aria-hidden className={cn('text-source-generated', className)} /></Mark>;
}

function Mark({ label, children }: { label: string; children: ReactNode }) {
    return (
        <Tooltip>
            <TooltipTrigger render={<span aria-label={label} className="inline-flex shrink-0" role="img" />}>{children}</TooltipTrigger>
            <TooltipContent>{label}</TooltipContent>
        </Tooltip>
    );
}
