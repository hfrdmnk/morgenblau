import type { ReactNode } from 'react';

import { GeneratedIcon, PrimaryIcon, type IconProps } from '@/components/icons';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import { cn } from 'cn';

type MarkSize = NonNullable<IconProps['size']>;

function PrimaryMark({ size }: { size: MarkSize }) {
    return (
        <Mark label="Primary">
            <PrimaryIcon aria-hidden size={size} tone="source-primary" />
        </Mark>
    );
}

function GeneratedMark({ size, tucked = false }: { size: MarkSize; tucked?: boolean }) {
    return (
        <Mark label="Generated feed" tucked={tucked}>
            <GeneratedIcon aria-hidden size={size} tone="source-generated" />
        </Mark>
    );
}

function Mark({ label, tucked = false, children }: { label: string; tucked?: boolean; children: ReactNode }) {
    return (
        <Tooltip>
            <TooltipTrigger
                render={
                    <span aria-label={label} className={cn('inline-flex shrink-0', tucked && '-ms-1')} role="img" />
                }
            >
                {children}
            </TooltipTrigger>
            <TooltipContent>{label}</TooltipContent>
        </Tooltip>
    );
}

const gutters = { row: 'sm:-left-7', title: 'sm:-left-8' };

function GutterMark({ offset, children }: { offset: keyof typeof gutters; children: ReactNode }) {
    return (
        <span className={cn('flex shrink-0 sm:absolute sm:top-1/2 sm:-translate-y-1/2', gutters[offset])}>
            {children}
        </span>
    );
}

export { GeneratedMark, GutterMark, PrimaryMark };
