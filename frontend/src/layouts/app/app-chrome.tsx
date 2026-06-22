import {
    LogoutSquare01Icon,
    PlusSignIcon,
    Refresh04Icon,
} from '@hugeicons/core-free-icons';
import { HugeiconsIcon } from '@hugeicons/react';

import { useAuthedMe } from '@/hooks/use-authed-me';
import {
    Avatar,
    AvatarFallback,
    AvatarImage,
} from '@/components/ui/avatar';
import { CalendarStrip } from '@/components/calendar-strip';
import { Button } from '@/components/ui/button';
import {
    DropdownMenu,
    DropdownMenuContent,
    DropdownMenuItem,
    DropdownMenuSeparator,
    DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { useChromeCalendar, useChromeRefresh } from '@/hooks/use-chrome-refresh';
import type { Me } from '@/hooks/use-me';
import { initialsFromHandle, truncateDid } from '@/lib/handle';
import { PATHS } from '@/lib/paths';
import { cn } from '@/lib/utils';

type Tab = {
    label: string;
    href: string;
};

const TABS: Tab[] = [
    { label: 'Discover', href: PATHS.discover },
    { label: 'Sources', href: PATHS.sources },
    { label: 'Library', href: PATHS.library },
    { label: 'Digest', href: PATHS.digest },
];

type Props = {
    onAddSourceClick: () => void;
};

const ICON_ACTION_CLASS = 'hover:bg-transparent hover:text-primary';

export function AppChrome({ onAddSourceClick }: Props) {
    const pathname = window.location.pathname;
    const me = useAuthedMe();
    const refresh = useChromeRefresh();
    const calendar = useChromeCalendar();

    return (
        <header className="sticky top-0 z-20 flex h-20 shrink-0 items-center justify-between bg-background px-4 sm:px-6 lg:px-20">
            <nav className="flex items-center gap-6">
                {TABS.map((tab) => {
                    const isActive =
                        pathname === tab.href ||
                        pathname.startsWith(`${tab.href}/`);

                    return (
                        <a
                            key={tab.href}
                            href={tab.href}
                            aria-current={isActive ? 'page' : undefined}
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

            {calendar && (
                <div className="pointer-events-none absolute inset-y-0 left-1/2 flex w-[26rem] max-w-[42vw] -translate-x-1/2 items-center">
                    <div className="pointer-events-auto w-full">
                        <CalendarStrip
                            selected={calendar.selected}
                            today={calendar.today}
                            onSelect={calendar.onSelect}
                        />
                    </div>
                </div>
            )}

            <div className="flex items-center gap-2 text-muted-foreground">
                <Button
                    variant="ghost"
                    size="icon-sm"
                    className={ICON_ACTION_CLASS}
                    aria-label="Add source"
                    onClick={onAddSourceClick}
                >
                    <HugeiconsIcon icon={PlusSignIcon} className="size-5" />
                </Button>
                <Button
                    variant="ghost"
                    size="icon-sm"
                    className={ICON_ACTION_CLASS}
                    aria-label="Refresh"
                    disabled={!refresh || refresh.busy}
                    onClick={() => refresh?.onRefresh()}
                >
                    <HugeiconsIcon
                        icon={Refresh04Icon}
                        className={cn(
                            'size-5',
                            refresh?.busy && 'motion-safe:animate-spin',
                        )}
                    />
                </Button>
                <DropdownMenu>
                    <DropdownMenuTrigger
                        render={
                            <Button
                                variant="ghost"
                                size="icon-sm"
                                aria-label="Account"
                                className="rounded-full"
                            />
                        }
                    >
                        <TriggerAvatar me={me} />
                    </DropdownMenuTrigger>
                    <DropdownMenuContent align="end" className="w-56">
                        <UserHeader me={me} />
                        <DropdownMenuSeparator />
                        <DropdownMenuItem
                            render={
                                <form
                                    method="POST"
                                    action={PATHS.oauthLogout}
                                />
                            }
                        >
                            <button
                                type="submit"
                                className="flex w-full items-center gap-2"
                            >
                                <HugeiconsIcon icon={LogoutSquare01Icon} />
                                Log out
                            </button>
                        </DropdownMenuItem>
                    </DropdownMenuContent>
                </DropdownMenu>
            </div>

            <div
                aria-hidden
                className="pointer-events-none absolute inset-x-0 top-full h-8 bg-linear-to-b from-background to-transparent"
            />
        </header>
    );
}

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
