import {
    LogoutSquare01Icon,
    PlusSignIcon,
} from '@hugeicons/core-free-icons';
import { HugeiconsIcon } from '@hugeicons/react';
import { useState } from 'react';

import { useAuthedMe } from '@/components/me-provider';
import { AddSubscriptionDialog } from '@/components/subscriptions/add-dialog';
import {
    Avatar,
    AvatarFallback,
    AvatarImage,
} from '@/components/ui/avatar';
import { Button } from '@/components/ui/button';
import {
    DropdownMenu,
    DropdownMenuContent,
    DropdownMenuItem,
    DropdownMenuSeparator,
    DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import type { Me } from '@/hooks/use-me';
import { initialsFromHandle, truncateDid } from '@/lib/handle';
import { cn } from '@/lib/utils';

type Tab = {
    label: string;
    href: string;
};

const TABS: Tab[] = [
    { label: 'Discover', href: '/discover' },
    { label: 'Consume', href: '/consume' },
    { label: 'Create', href: '/create' },
];

export function WindowChrome() {
    const pathname = window.location.pathname;
    const me = useAuthedMe();
    const [addSourceOpen, setAddSourceOpen] = useState(false);

    const handleLogout = () => {
        const form = document.createElement('form');
        form.method = 'POST';
        form.action = '/oauth/logout';
        document.body.appendChild(form);
        form.submit();
    };

    return (
        <header className="flex h-14 shrink-0 items-center justify-between px-20">
            <AddSubscriptionDialog
                open={addSourceOpen}
                onOpenChange={setAddSourceOpen}
            />
            <nav className="flex items-center gap-6">
                {TABS.map((tab) => {
                    const isActive = pathname === tab.href;

                    return (
                        <a
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
                        </a>
                    );
                })}
            </nav>

            <div className="flex items-center gap-2 text-muted-foreground">
                <Button
                    variant="ghost"
                    size="icon-sm"
                    aria-label="Add source"
                    onClick={() => setAddSourceOpen(true)}
                >
                    <HugeiconsIcon icon={PlusSignIcon} className="size-5" />
                </Button>
                <DropdownMenu>
                    <DropdownMenuTrigger
                        render={
                            <Button
                                variant="ghost"
                                size="icon-sm"
                                aria-label="Account"
                            />
                        }
                    >
                        <TriggerAvatar me={me} />
                    </DropdownMenuTrigger>
                    <DropdownMenuContent align="end" className="w-56">
                        <UserHeader me={me} />
                        <DropdownMenuSeparator />
                        <DropdownMenuItem onClick={handleLogout}>
                            <HugeiconsIcon icon={LogoutSquare01Icon} />
                            Log out
                        </DropdownMenuItem>
                    </DropdownMenuContent>
                </DropdownMenu>
            </div>
        </header>
    );
}

// Trigger avatar — kept tiny so it sits inside the 32px icon-sm button.
function TriggerAvatar({ me }: { me: Me }) {
    return (
        <Avatar size="sm" className="size-5">
            {me.avatar && <AvatarImage src={me.avatar} alt="" />}
            <AvatarFallback className="bg-neutral-200 text-[10px] text-black dark:bg-neutral-700 dark:text-white">
                {initialsFromHandle(me.handle, me.did)}
            </AvatarFallback>
        </Avatar>
    );
}

function UserHeader({ me }: { me: Me }) {
    const handleLine = me.handle ? `@${me.handle}` : truncateDid(me.did);
    const displayName = me.displayName?.trim();
    const showDisplayName = Boolean(displayName) && displayName !== handleLine;

    return (
        <div className="flex items-center gap-2 px-2 py-1.5">
            <Avatar className="size-8 shrink-0">
                {me.avatar && <AvatarImage src={me.avatar} alt="" />}
                <AvatarFallback className="bg-neutral-200 text-black dark:bg-neutral-700 dark:text-white">
                    {initialsFromHandle(me.handle, me.did)}
                </AvatarFallback>
            </Avatar>
            <div className="flex min-w-0 flex-col leading-tight">
                {showDisplayName ? (
                    <>
                        <span className="truncate text-sm font-medium text-foreground">
                            {displayName}
                        </span>
                        <span className="truncate text-xs font-light text-muted-foreground">
                            {handleLine}
                        </span>
                    </>
                ) : (
                    <span className="truncate text-sm font-medium text-foreground">
                        {handleLine}
                    </span>
                )}
            </div>
        </div>
    );
}
