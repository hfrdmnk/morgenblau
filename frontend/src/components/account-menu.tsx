import { useState } from 'react';
import { Link } from 'wouter';

import { DownloadIcon, SettingsIcon, UploadIcon } from '@/components/icons';
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

function profileInitial(profile: AppProfile): string {
    const source = profile.displayName?.trim() || profile.handle || profile.did;
    return source.charAt(0).toLocaleUpperCase();
}

function ProfileAvatar({ profile }: { profile: AppProfile }) {
    const [failedAvatar, setFailedAvatar] = useState<string | null>(null);
    const avatar = profile.avatar;

    return (
        <span className="flex size-8 items-center justify-center overflow-hidden rounded-full bg-secondary text-sm font-medium text-secondary-foreground">
            {avatar && failedAvatar !== avatar ? (
                <img
                    src={avatar}
                    alt=""
                    className="size-full object-cover"
                    onError={() => setFailedAvatar(avatar)}
                    referrerPolicy="no-referrer"
                />
            ) : (
                profileInitial(profile)
            )}
        </span>
    );
}

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
                <ProfileAvatar profile={profile} />
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
