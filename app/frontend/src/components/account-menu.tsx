import { useState } from 'react';
import { Link } from 'wouter';

import { DownloadIcon, SettingsIcon, ShieldIcon, UploadIcon } from '@/components/icons';
import { PersonAvatar } from '@/components/person-avatar';
import { Button } from '@/components/ui/button';
import {
    Popover,
    PopoverContent,
    PopoverDescription,
    PopoverHeader,
    PopoverTitle,
    PopoverTrigger,
} from '@/components/ui/popover';
import type { AppProfile } from '@/lib/app-profile-context';
import { PATHS } from '@/lib/paths';

export function AccountMenu({ profile }: { profile: AppProfile }) {
    const accountName = profile.displayName?.trim() || `@${profile.handle}`;
    const [open, setOpen] = useState(false);

    return (
        <Popover open={open} onOpenChange={setOpen}>
            <PopoverTrigger
                render={
                    <Button
                        variant="ghost"
                        size="icon"
                        className="rounded-full p-1"
                        aria-label="Open account menu"
                    />
                }
            >
                <PersonAvatar seed={profile.did} src={profile.avatar} />
            </PopoverTrigger>
            <PopoverContent align="end" className="w-64">
                <PopoverHeader>
                    <PopoverTitle className="truncate">
                        {accountName}
                    </PopoverTitle>
                    <PopoverDescription className="truncate">
                        @{profile.handle}
                    </PopoverDescription>
                </PopoverHeader>
                <nav aria-label="Account" className="-mx-2">
                    <Link
                        href={PATHS.general}
                        className="flex min-h-10 items-center gap-3 rounded-md px-2 text-sm hover:bg-secondary/50"
                        onClick={() => setOpen(false)}
                    >
                        <SettingsIcon className="size-4" />
                        General
                    </Link>
                    <Link
                        href={PATHS.import}
                        className="flex min-h-10 items-center gap-3 rounded-md px-2 text-sm hover:bg-secondary/50"
                        onClick={() => setOpen(false)}
                    >
                        <UploadIcon className="size-4" />
                        Import
                    </Link>
                    <Link
                        href={PATHS.export}
                        className="flex min-h-10 items-center gap-3 rounded-md px-2 text-sm hover:bg-secondary/50"
                        onClick={() => setOpen(false)}
                    >
                        <DownloadIcon className="size-4" />
                        Export
                    </Link>
                    {profile.isAdmin ? (
                        <Link
                            href={PATHS.admin}
                            className="flex min-h-10 items-center gap-3 rounded-md px-2 text-sm hover:bg-secondary/50"
                            onClick={() => setOpen(false)}
                        >
                            <ShieldIcon className="size-4" />
                            Admin
                        </Link>
                    ) : null}
                </nav>
                <form method="POST" action={PATHS.oauthLogout}>
                    <Button
                        type="submit"
                        variant="secondary"
                        className="w-full"
                    >
                        Log out
                    </Button>
                </form>
            </PopoverContent>
        </Popover>
    );
}
