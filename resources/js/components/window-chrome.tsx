import {
    PlusSignIcon,
    Settings03Icon,
    UserCircleIcon,
} from '@hugeicons/core-free-icons';
import { HugeiconsIcon } from '@hugeicons/react';
import { Link, usePage } from '@inertiajs/react';

import { Button } from '@/components/ui/button';
import { cn } from '@/lib/utils';
import { consume, create, discover } from '@/routes';

type Tab = {
    label: string;
    href: string;
};

const TABS: Tab[] = [
    { label: 'Discover', href: discover().url },
    { label: 'Consume', href: consume().url },
    { label: 'Create', href: create().url },
];

export function WindowChrome() {
    const { url } = usePage();

    return (
        <header className="flex h-14 shrink-0 items-center justify-between px-20">
            <nav className="flex items-center gap-6">
                {TABS.map((tab) => {
                    const isActive = url === tab.href;

                    return (
                        <Link
                            key={tab.href}
                            href={tab.href}
                            className={cn(
                                'relative text-sm font-medium transition-colors outline-none focus-visible:outline-1 focus-visible:outline-offset-4 focus-visible:outline-ring focus-visible:outline-solid',
                                isActive
                                    ? 'text-foreground'
                                    : 'text-muted-foreground hover:text-foreground',
                            )}
                        >
                            {tab.label}
                            {isActive && (
                                <span
                                    aria-hidden
                                    className="absolute -bottom-2 left-1/2 size-1 -translate-x-1/2 rounded-full bg-primary"
                                />
                            )}
                        </Link>
                    );
                })}
            </nav>

            <div className="flex items-center gap-2 text-muted-foreground">
                <Button variant="ghost" size="icon-sm" aria-label="Add source">
                    <HugeiconsIcon icon={PlusSignIcon} className="size-5" />
                </Button>
                <Button variant="ghost" size="icon-sm" aria-label="Profile">
                    <HugeiconsIcon icon={UserCircleIcon} className="size-5" />
                </Button>
                <Button variant="ghost" size="icon-sm" aria-label="Settings">
                    <HugeiconsIcon icon={Settings03Icon} className="size-5" />
                </Button>
            </div>
        </header>
    );
}
