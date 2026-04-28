import { Logout03Icon } from '@hugeicons/core-free-icons';
import { HugeiconsIcon } from '@hugeicons/react';
import { router } from '@inertiajs/react';
import {
    DropdownMenuGroup,
    DropdownMenuItem,
    DropdownMenuLabel,
    DropdownMenuSeparator,
} from '@/components/ui/dropdown-menu';
import { UserInfo } from '@/components/user-info';
import { useMobileNavigation } from '@/hooks/use-mobile-navigation';
import { logout } from '@/routes';

type Props = {
    handle: string | null;
    did: string;
};

export function UserMenuContent({ handle, did }: Props) {
    const cleanup = useMobileNavigation();

    const handleLogout = () => {
        cleanup();
        router.flushAll();
        router.post(logout().url);
    };

    return (
        <DropdownMenuGroup>
            <DropdownMenuLabel className="p-0 font-normal">
                <div className="flex items-center gap-2 px-1 py-1.5 text-left text-sm">
                    <UserInfo handle={handle} did={did} showDid />
                </div>
            </DropdownMenuLabel>
            <DropdownMenuSeparator />
            <DropdownMenuItem
                className="cursor-pointer"
                data-test="logout-button"
                onClick={handleLogout}
            >
                <HugeiconsIcon icon={Logout03Icon} />
                Log out
            </DropdownMenuItem>
        </DropdownMenuGroup>
    );
}
