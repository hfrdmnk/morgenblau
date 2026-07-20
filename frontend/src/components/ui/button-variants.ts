import { cva, type VariantProps } from 'class-variance-authority';

export const buttonVariants = cva(
    "group/button inline-flex shrink-0 cursor-pointer items-center justify-center rounded-xl text-sm font-medium whitespace-nowrap transition-all outline-none select-none outline-ring outline-offset-2 focus-visible:outline-solid focus-visible:outline-1 motion-safe:active:not-aria-[haspopup]:scale-[0.97] disabled:pointer-events-none disabled:cursor-not-allowed disabled:opacity-50 disabled:saturate-50 aria-invalid:ring-3 aria-invalid:ring-destructive/20 dark:aria-invalid:ring-destructive/40 [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*='size-'])]:size-4",
    {
        variants: {
            variant: {
                default:
                    'bg-primary text-primary-foreground hover:bg-primary/90',
                secondary: 'bg-overlay-2 text-foreground hover:bg-overlay-3',
                ghost: 'hover:bg-overlay-2 hover:text-foreground aria-expanded:bg-overlay-2 aria-expanded:text-foreground',
                'ghost-on-gradient':
                    'bg-white/10 text-white shadow-[0_0_0_1px_rgba(255,255,255,0.3)] hover:bg-white/20 outline-white/80',
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
            iconTint: {
                none: '',
                primary: '[&_svg]:text-primary',
                success: '[&_svg]:text-success',
                error: '[&_svg]:text-destructive',
            },
        },
        defaultVariants: {
            variant: 'default',
            size: 'default',
            iconTint: 'none',
        },
        compoundVariants: [
            // ghost rest label is muted, warming to foreground on hover (ghost already hovers to foreground)
            {
                variant: 'ghost',
                iconTint: ['primary', 'success', 'error'],
                class: 'text-muted-foreground',
            },
        ],
    },
);

export type ButtonVariant = VariantProps<typeof buttonVariants>['variant'];
