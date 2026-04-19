import * as React from 'react';
import { Input as InputPrimitive } from '@base-ui/react/input';

import { cn } from '@/lib/utils';
import { useSurfaceLevel, type SurfaceLevel } from '@/lib/LevelContext';

const INPUT_BY_LEVEL: Record<SurfaceLevel, string> = {
	1: 'bg-gray-100 border-gray-200 dark:bg-gray-800 dark:border-gray-700',
	2: 'bg-gray-50 border-gray-100 dark:bg-gray-700 dark:border-gray-600'
};

function Input({ className, type, ...props }: React.ComponentProps<'input'>) {
	const level = useSurfaceLevel();
	return (
		<InputPrimitive
			type={type}
			data-slot="input"
			className={cn(
				'h-10 w-full min-w-0 rounded-xl border px-3 py-1 text-base transition-colors outline-none file:inline-flex file:h-6 file:border-0 file:bg-transparent file:text-sm file:font-medium file:text-foreground placeholder:text-muted-foreground focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50 disabled:pointer-events-none disabled:cursor-not-allowed disabled:opacity-50 aria-invalid:border-destructive aria-invalid:ring-3 aria-invalid:ring-destructive/20 md:text-sm dark:aria-invalid:border-destructive/50 dark:aria-invalid:ring-destructive/40',
				INPUT_BY_LEVEL[level],
				className
			)}
			{...props}
		/>
	);
}

export { Input };
