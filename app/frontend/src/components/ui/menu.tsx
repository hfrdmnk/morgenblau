import type { ComponentProps } from 'react';
import { Link as RouterLink } from 'wouter';

import { PopoverContent } from '@/components/ui/popover';

function MenuPopup(props: Omit<ComponentProps<typeof PopoverContent>, 'className' | 'style'>) {
    return <PopoverContent className="w-64" {...props} />;
}

function MenuList(props: Omit<ComponentProps<'nav'>, 'className' | 'style'>) {
    return <nav className="-mx-2 select-none" {...props} />;
}

function MenuLink(props: Omit<ComponentProps<'a'>, 'className' | 'style'> & { href: string }) {
    return (
        <RouterLink
            className="flex min-h-10 items-center gap-3 rounded-md px-2 text-sm hover:bg-secondary"
            {...props}
        />
    );
}

export { MenuLink, MenuList, MenuPopup };
