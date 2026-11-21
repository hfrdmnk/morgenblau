import type { ComponentProps } from 'react';

import { cn } from 'cn';

function Card({ className, ...props }: ComponentProps<'div'>) {
    return (
        <div
            data-slot="card"
            className={cn('flex flex-col gap-6 bg-card py-6 text-card-foreground', className)}
            {...props}
        />
    );
}

function CardHeader({ className, ...props }: ComponentProps<'div'>) {
    return (
        <div
            data-slot="card-header"
            className={cn(
                'grid auto-rows-min items-start gap-1 px-6 has-data-[slot=card-action]:grid-cols-(--grid-trailing)',
                className,
            )}
            {...props}
        />
    );
}

function CardTitle({ className, ...props }: ComponentProps<'div'>) {
    return (
        <div
            data-slot="card-title"
            className={cn('text-base font-medium', className)}
            {...props}
        />
    );
}

function CardDescription({ className, ...props }: ComponentProps<'div'>) {
    return (
        <div
            data-slot="card-description"
            className={cn('text-sm text-muted-foreground', className)}
            {...props}
        />
    );
}

function CardAction({ className, ...props }: ComponentProps<'div'>) {
    return (
        <div
            data-slot="card-action"
            className={cn(
                'col-start-2 row-span-2 row-start-1 self-start justify-self-end',
                className,
            )}
            {...props}
        />
    );
}

function CardContent({ className, ...props }: ComponentProps<'div'>) {
    return (
        <div
            data-slot="card-content"
            className={cn('flex flex-col gap-3 px-6', className)}
            {...props}
        />
    );
}

function CardFooter({ className, ...props }: ComponentProps<'div'>) {
    return (
        <div
            data-slot="card-footer"
            className={cn('flex items-center px-6', className)}
            {...props}
        />
    );
}

export {
    Card,
    CardAction,
    CardContent,
    CardDescription,
    CardFooter,
    CardHeader,
    CardTitle,
};
