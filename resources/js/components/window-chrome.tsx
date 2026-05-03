import {
    LogoutSquare01Icon,
    PlusSignIcon,
    Settings03Icon,
    UserCircleIcon,
} from '@hugeicons/core-free-icons';
import { HugeiconsIcon } from '@hugeicons/react';
import { Deferred, Link, router, usePage } from '@inertiajs/react';
import { useState } from 'react';

import { AddSubscriptionDialog } from '@/components/subscriptions/add-dialog';
import { Avatar, AvatarFallback, AvatarImage } from '@/components/ui/avatar';
import { Button } from '@/components/ui/button';
import {
    DropdownMenu,
    DropdownMenuContent,
    DropdownMenuItem,
    DropdownMenuSeparator,
    DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { Skeleton } from '@/components/ui/skeleton';
import { initialsFromHandle, truncateDid } from '@/lib/handle';
import { cn } from '@/lib/utils';
import { consume, create, discover, logout } from '@/routes';
import { edit as editAppearance } from '@/routes/appearance';
import type { Profile } from '@/types/auth';

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
    const { url, props } = usePage();
    const auth = props.auth;
    const [addSourceOpen, setAddSourceOpen] = useState(false);

    const handleLogout = () => {
        router.flushAll();
        router.post(logout().url);
    };

    return (
        <header className="flex h-14 shrink-0 items-center justify-between px-20">
            <AddSubscriptionDialog
                open={addSourceOpen}
                onOpenChange={setAddSourceOpen}
            />
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
                        <HugeiconsIcon
                            icon={UserCircleIcon}
                            className="size-5"
                        />
                    </DropdownMenuTrigger>
                    <DropdownMenuContent align="end" className="w-56">
                        {auth.user && (
                            <>
                                <Deferred
                                    data="auth.profile"
                                    fallback={<UserHeaderSkeleton />}
                                >
                                    <UserHeader
                                        handle={auth.handle}
                                        did={auth.user.did}
                                        profile={auth.profile ?? null}
                                    />
                                </Deferred>
                                <DropdownMenuSeparator />
                            </>
                        )}
                        <DropdownMenuItem
                            render={<Link href={editAppearance().url} />}
                        >
                            <HugeiconsIcon icon={Settings03Icon} />
                            Settings
                        </DropdownMenuItem>
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

function UserHeader({
    handle,
    did,
    profile,
}: {
    handle: string | null;
    did: string;
    profile: Profile | null;
}) {
    const handleLine = handle ? `@${handle}` : truncateDid(did);
    const displayName = profile?.displayName?.trim() || handleLine;

    return (
        <div className="flex items-center gap-2 px-2 py-1.5 transition-opacity duration-200">
            <Avatar className="size-8 shrink-0">
                {profile?.avatar && <AvatarImage src={profile.avatar} alt="" />}
                <AvatarFallback className="bg-neutral-200 text-black dark:bg-neutral-700 dark:text-white">
                    {initialsFromHandle(handle, did)}
                </AvatarFallback>
            </Avatar>
            <div className="flex min-w-0 flex-col leading-tight">
                <span className="truncate text-sm font-medium text-foreground">
                    {displayName}
                </span>
                <span className="truncate text-xs font-light text-muted-foreground">
                    {handleLine}
                </span>
            </div>
        </div>
    );
}

function UserHeaderSkeleton() {
    return (
        <div className="flex items-center gap-2 px-2 py-1.5">
            <Skeleton className="size-8 shrink-0 animate-none rounded-full" />
            <div className="flex min-w-0 flex-col gap-1.5">
                <Skeleton className="h-3.5 w-24 animate-none" />
                <Skeleton className="h-3 w-16 animate-none" />
            </div>
        </div>
    );
}
