import type { ReactNode } from 'react';
import { LevelContext } from '@/lib/level-context';
import { cn } from '@/lib/utils';

type SurfaceLevelChoice = 1 | 2;

const LEVEL_STYLES: Record<SurfaceLevelChoice, string> = {
    1: 'bg-gray-50 border-gray-200 dark:bg-gray-900 dark:border-gray-800',
    2: 'bg-white border-gray-100 dark:bg-gray-800 dark:border-gray-700',
};

type SurfaceProps = {
    children: ReactNode;
    level?: SurfaceLevelChoice;
    className?: string;
};

export function Surface({ children, level = 2, className }: SurfaceProps) {
    return (
        <LevelContext.Provider value={level}>
            <div
                className={cn(
                    'rounded-4xl border',
                    LEVEL_STYLES[level],
                    className,
                )}
            >
                {children}
            </div>
        </LevelContext.Provider>
    );
}
