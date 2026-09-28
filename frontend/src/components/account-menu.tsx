import { useState } from 'react';

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

    return (
        <Popover>
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
                    <PopoverTitle className="truncate">{accountName}</PopoverTitle>
                    <PopoverDescription className="truncate">
                        @{profile.handle}
                    </PopoverDescription>
                </PopoverHeader>
                <form method="POST" action={PATHS.oauthLogout}>
                    <Button type="submit" variant="secondary" className="w-full">
                        Log out
                    </Button>
                </form>
            </PopoverContent>
        </Popover>
    );
}
