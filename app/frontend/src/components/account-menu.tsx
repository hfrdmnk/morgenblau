import { useState, type KeyboardEvent } from 'react';

import { DownloadIcon, SettingsIcon, ShieldIcon, UploadIcon } from '@/components/icons';
import { PersonAvatar } from '@/components/person-avatar';
import { Button } from '@/components/ui/button';
import { MenuLink, MenuList, MenuPopup } from '@/components/ui/menu';
import { Popover, PopoverDescription, PopoverHeader, PopoverTitle, PopoverTrigger } from '@/components/ui/popover';
import { Frame } from '@/design/frame';
import { Text } from '@/design/text';
import type { AppProfile } from '@/lib/app-profile-context';
import { PATHS } from '@/lib/paths';

export function AccountMenu({ profile }: { profile: AppProfile }) {
    const accountName = profile.displayName?.trim() || `@${profile.handle}`;
    const [open, setOpen] = useState(false);

    return (
        <Popover open={open} onOpenChange={setOpen}>
            <PopoverTrigger render={<Button variant="ghost" size="avatar" aria-label="Open account menu" />}>
                <PersonAvatar seed={profile.did} src={profile.avatar} />
            </PopoverTrigger>
            <MenuPopup align="end" onKeyDown={moveThroughItems}>
                <PopoverHeader>
                    <PopoverTitle>
                        <Text as="span" display="block" truncate>
                            {accountName}
                        </Text>
                    </PopoverTitle>
                    <PopoverDescription>
                        <Text as="span" display="block" truncate>
                            @{profile.handle}
                        </Text>
                    </PopoverDescription>
                </PopoverHeader>
                <MenuList aria-label="Account">
                    <MenuLink href={PATHS.general} onClick={() => setOpen(false)}>
                        <SettingsIcon size="md" />
                        <Text as="span">General</Text>
                    </MenuLink>
                    <MenuLink href={PATHS.import} onClick={() => setOpen(false)}>
                        <UploadIcon size="md" />
                        <Text as="span">Import</Text>
                    </MenuLink>
                    <MenuLink href={PATHS.export} onClick={() => setOpen(false)}>
                        <DownloadIcon size="md" />
                        <Text as="span">Export</Text>
                    </MenuLink>
                    {profile.isAdmin ? (
                        <MenuLink href={PATHS.admin} onClick={() => setOpen(false)}>
                            <ShieldIcon size="md" />
                            <Text as="span">Admin</Text>
                        </MenuLink>
                    ) : null}
                </MenuList>
                <Frame as="form" method="POST" action={PATHS.oauthLogout}>
                    <Button type="submit" variant="secondary">
                        Log out
                    </Button>
                </Frame>
            </MenuPopup>
        </Popover>
    );
}

const STEPS: Partial<Record<string, number>> = { ArrowDown: 1, ArrowUp: -1 };

function moveThroughItems(event: KeyboardEvent<HTMLElement>) {
    const step = STEPS[event.key];
    if (!step) return;
    event.preventDefault();
    const items = [...event.currentTarget.querySelectorAll<HTMLElement>('a, button')];
    const index = items.findIndex((item) => item === document.activeElement);
    items.at(index === -1 ? Math.min(step, 0) : (index + step) % items.length)?.focus();
}
