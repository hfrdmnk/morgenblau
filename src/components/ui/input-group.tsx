import * as React from 'react';
import { Input as InputPrimitive } from '@base-ui/react/input';

import { cn } from '@/lib/utils';
import { useSurfaceLevel, type SurfaceLevel } from '@/lib/LevelContext';

const INPUT_BY_LEVEL: Record<SurfaceLevel, string> = {
	1: 'bg-gray-100 border-gray-200 dark:bg-gray-800 dark:border-gray-700',
	2: 'bg-gray-50 border-gray-100 dark:bg-gray-700 dark:border-gray-600'
};

function InputGroup({ className, ...props }: React.ComponentProps<'div'>) {
	const level = useSurfaceLevel();
	return (
		<div
			role="group"
			data-slot="input-group"
			className={cn(
				'relative flex h-10 w-full min-w-0 items-center rounded-xl border px-3 transition-colors',
				'focus-within:border-ring focus-within:ring-3 focus-within:ring-ring/50',
				'has-[[aria-invalid=true]]:border-destructive has-[[aria-invalid=true]]:ring-3 has-[[aria-invalid=true]]:ring-destructive/20',
				'has-[[data-slot=input-group-control]:disabled]:pointer-events-none has-[[data-slot=input-group-control]:disabled]:opacity-50',
				INPUT_BY_LEVEL[level],
				className
			)}
			{...props}
		/>
	);
}

function InputGroupInput({ className, ...props }: React.ComponentProps<'input'>) {
	return (
		<InputPrimitive
			data-slot="input-group-control"
			className={cn(
				'min-w-0 flex-1 border-0 bg-transparent p-0 text-base shadow-none outline-none',
				'placeholder:text-muted-foreground disabled:cursor-not-allowed',
				'focus:border-transparent focus:shadow-none focus:ring-0 focus:outline-none',
				'md:text-sm',
				className
			)}
			{...props}
		/>
	);
}

type AddonAlign = 'inline-start' | 'inline-end';

function InputGroupAddon({
	className,
	align = 'inline-start',
	...props
}: React.ComponentProps<'div'> & { align?: AddonAlign }) {
	return (
		<div
			data-slot="input-group-addon"
			data-align={align}
			className={cn(
				'flex shrink-0 items-center',
				align === 'inline-start' ? 'mr-2' : 'ml-2',
				className
			)}
			{...props}
		/>
	);
}

function InputGroupText({ className, ...props }: React.ComponentProps<'span'>) {
	return (
		<span
			data-slot="input-group-text"
			className={cn('text-sm text-muted-foreground select-none', className)}
			{...props}
		/>
	);
}

export { InputGroup, InputGroupInput, InputGroupAddon, InputGroupText };
