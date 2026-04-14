import type { ReactNode } from 'react';
import { cn } from '../lib/utils';

type WindowProps = {
	children: ReactNode;
	className?: string;
};

export function Window({ children, className }: WindowProps) {
	return (
		<div
			className={cn(
				'fixed top-14 right-2 bottom-2 left-2 overflow-hidden',
				'rounded-t-[4rem] rounded-b-[0.5rem]',
				'bg-bg-front-1',
				className
			)}
		>
			{children}
		</div>
	);
}
