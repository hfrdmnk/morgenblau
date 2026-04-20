import type { ReactNode } from 'react';
import { cn } from '../lib/utils';
import { LevelContext } from '../lib/LevelContext';

type CardLevel = 1 | 2;

type CardProps = {
	children: ReactNode;
	level?: CardLevel;
	className?: string;
};

const LEVEL_STYLES: Record<CardLevel, string> = {
	1: 'bg-gray-50 border-gray-200 dark:bg-gray-900 dark:border-gray-800',
	2: 'bg-white border-gray-100 dark:bg-gray-800 dark:border-gray-700'
};

export function Card({ children, level = 2, className }: CardProps) {
	return (
		<LevelContext.Provider value={level}>
			<div className={cn('rounded-4xl border', LEVEL_STYLES[level], className)}>{children}</div>
		</LevelContext.Provider>
	);
}
