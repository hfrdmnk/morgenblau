import type { ReactNode } from 'react';
import { cn } from '@/lib/utils';

type WindowVariant = 'plain' | 'sunrise';

type WindowProps = {
    children?: ReactNode;
    variant?: WindowVariant;
    className?: string;
};

const VARIANT_STYLES: Record<WindowVariant, string> = {
    plain: 'border border-gray-200 bg-gray-50 dark:border-gray-800 dark:bg-gray-900',
    sunrise: 'bg-sunrise shadow-[inset_0_0_0_1px_rgba(255,255,255,0.4)]',
};

export function Window({
    children,
    variant = 'plain',
    className,
}: WindowProps) {
    return (
        <div
            data-slot="window"
            className={cn(
                'overflow-hidden rounded-tl-[4rem] rounded-tr-[4rem] rounded-br-[0.5rem] rounded-bl-[0.5rem]',
                VARIANT_STYLES[variant],
                className,
            )}
        >
            {children}
        </div>
    );
}
