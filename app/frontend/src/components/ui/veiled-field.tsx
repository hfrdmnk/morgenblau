import type { ReactNode } from 'react';
import { Link as RouterLink } from 'wouter';

// A field drawn blurred under a label, linking to the app page where the real value lives.
function VeiledFieldLink({ href, onClick, value, icon, children }: {
    href: string;
    onClick?: () => void;
    value: string;
    icon?: ReactNode;
    children: ReactNode;
}) {
    return (
        <RouterLink className="relative block h-10 overflow-hidden rounded-md bg-input text-sm" data-slot="veiled-field" href={href} onClick={onClick}>
            <span aria-hidden="true" className="flex h-full items-center justify-between gap-2 px-3 text-subtle-foreground blur-veil select-none">
                {value}
                {icon}
            </span>
            <span className="absolute inset-0 flex items-center justify-center bg-input-soft px-3 text-center font-medium">{children}</span>
        </RouterLink>
    );
}

export { VeiledFieldLink };
