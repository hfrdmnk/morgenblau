import type { ComponentProps, CSSProperties } from 'react';
import { Toaster as SonnerToaster } from 'sonner';

// Sonner hardcodes chrome outside CSS vars, so unvarred properties need !important overrides; varred colors get repointed.
function Toaster(props: ComponentProps<typeof SonnerToaster>) {
    return (
        <SonnerToaster
            theme="system"
            toastOptions={{
                style: { boxShadow: 'var(--shadow-popover)' },
                classNames: {
                    title: '!font-normal',
                    description: '!font-light !text-muted-foreground',
                    actionButton:
                        '!h-auto !rounded-none !bg-transparent !px-0 !text-sm !font-light !text-primary underline underline-offset-4',
                },
            }}
            style={
                {
                    '--normal-bg': 'var(--popover)',
                    '--normal-text': 'var(--popover-foreground)',
                    '--normal-border': 'transparent',
                    '--border-radius': 'var(--radius)',
                    fontFamily: 'var(--font-sans)',
                } as CSSProperties
            }
            {...props}
        />
    );
}

export { Toaster };
