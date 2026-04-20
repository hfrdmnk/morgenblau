import type { ReactNode } from 'react';
import { cn } from '../lib/utils';
import { LevelContext } from '../lib/LevelContext';

type WindowProps = {
	children: ReactNode;
	className?: string;
};

export function Window({ children, className }: WindowProps) {
	return (
		<LevelContext.Provider value={1}>
			<div
				className={cn(
					'relative h-full w-full overflow-hidden border',
					'rounded-t-[4rem] rounded-b-[0.5rem]',
					'border-gray-200 bg-gray-50 dark:border-gray-800 dark:bg-gray-900',
					className
				)}
			>
				{children}
			</div>
		</LevelContext.Provider>
	);
}
