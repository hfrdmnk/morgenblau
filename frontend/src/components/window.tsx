import type { ReactNode } from 'react';
import { LevelContext } from '@/hooks/use-surface-level';
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
        <LevelContext.Provider value={1}>
            <div
                data-slot="window"
                className={cn(
                    'overflow-hidden rounded-tl-[var(--radius-window-top)] rounded-tr-[var(--radius-window-top)] rounded-br-[var(--radius-window-bottom)] rounded-bl-[var(--radius-window-bottom)]',
                    VARIANT_STYLES[variant],
                    className,
                )}
            >
                {children}
            </div>
        </LevelContext.Provider>
    );
}
