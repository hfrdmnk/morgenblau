import { Button as ButtonPrimitive } from '@base-ui/react/button';
import { cva, type VariantProps } from 'class-variance-authority';

import { cn } from '@/lib/utils';
import { useSurfaceLevel, type SurfaceLevel } from '@/lib/level-context';

const SECONDARY_BY_LEVEL: Record<SurfaceLevel, string> = {
    0: 'bg-gray-100 text-gray-900 hover:bg-gray-200 dark:bg-gray-600 dark:text-gray-50 dark:hover:bg-gray-500',
    1: 'bg-gray-200 text-gray-900 hover:bg-gray-300 dark:bg-gray-700 dark:text-gray-50 dark:hover:bg-gray-600',
    2: 'bg-gray-100 text-gray-900 hover:bg-gray-200 dark:bg-gray-600 dark:text-gray-50 dark:hover:bg-gray-500',
};

const buttonVariants = cva(
    "group/button inline-flex shrink-0 cursor-pointer items-center justify-center rounded-xl border border-transparent bg-clip-padding text-sm font-medium whitespace-nowrap transition-all outline-none select-none outline-ring outline-offset-2 focus-visible:outline-solid focus-visible:outline-1 motion-safe:active:not-aria-[haspopup]:scale-[0.97] disabled:pointer-events-none disabled:cursor-not-allowed disabled:opacity-50 disabled:saturate-50 aria-invalid:border-destructive aria-invalid:ring-3 aria-invalid:ring-destructive/20 dark:aria-invalid:border-destructive/50 dark:aria-invalid:ring-destructive/40 [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*='size-'])]:size-4",
    {
        variants: {
            variant: {
                default:
                    'bg-primary text-primary-foreground hover:bg-primary/90',
                secondary: '',
                ghost: 'hover:bg-muted hover:text-foreground aria-expanded:bg-muted aria-expanded:text-foreground dark:hover:bg-muted/50',
                'ghost-on-gradient':
                    'border-white/30 bg-white/10 text-white hover:bg-white/20 outline-white/80',
                destructive:
                    'bg-destructive/10 text-destructive hover:bg-destructive/20 focus-visible:border-destructive/40 focus-visible:ring-destructive/20 dark:bg-destructive/20 dark:hover:bg-destructive/30 dark:focus-visible:ring-destructive/40',
                link: 'text-primary underline-offset-4 hover:underline',
            },
            size: {
                default:
                    'h-10 gap-1.5 px-3.5 has-data-[icon=inline-end]:pr-3 has-data-[icon=inline-start]:pl-3',
                xs: "h-6 gap-1 rounded-lg px-2 text-xs in-data-[slot=button-group]:rounded-lg has-data-[icon=inline-end]:pr-1.5 has-data-[icon=inline-start]:pl-1.5 [&_svg:not([class*='size-'])]:size-3",
                sm: "h-8 gap-1 rounded-lg px-2.5 text-[0.8rem] in-data-[slot=button-group]:rounded-lg has-data-[icon=inline-end]:pr-1.5 has-data-[icon=inline-start]:pl-1.5 [&_svg:not([class*='size-'])]:size-3.5",
                lg: 'h-11 gap-1.5 px-4 has-data-[icon=inline-end]:pr-3.5 has-data-[icon=inline-start]:pl-3.5',
                icon: 'size-10',
                'icon-xs':
                    "size-6 rounded-lg in-data-[slot=button-group]:rounded-lg [&_svg:not([class*='size-'])]:size-3",
                'icon-sm':
                    'size-8 rounded-lg in-data-[slot=button-group]:rounded-lg',
                'icon-lg': 'size-11',
            },
        },
        defaultVariants: {
            variant: 'default',
            size: 'default',
        },
    },
);

type ButtonVariant = VariantProps<typeof buttonVariants>['variant'];

function Button({
    className,
    variant = 'default',
    size = 'default',
    ...props
}: ButtonPrimitive.Props & VariantProps<typeof buttonVariants>) {
    const level = useSurfaceLevel();
    const variantClass: ButtonVariant = variant ?? 'default';

    return (
        <ButtonPrimitive
            data-slot="button"
            className={cn(
                buttonVariants({ variant: variantClass, size }),
                variantClass === 'secondary' && SECONDARY_BY_LEVEL[level],
                className,
            )}
            {...props}
        />
    );
}

export { Button, buttonVariants };
