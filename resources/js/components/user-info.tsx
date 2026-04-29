import { Avatar, AvatarFallback } from '@/components/ui/avatar';
import { initialsFromHandle, truncateDid } from '@/lib/handle';

export function UserInfo({
    handle,
    did,
    showDid = false,
}: {
    handle: string | null;
    did: string;
    showDid?: boolean;
}) {
    const label = handle ? `@${handle}` : truncateDid(did);

    return (
        <>
            <Avatar className="h-8 w-8 overflow-hidden rounded-full">
                <AvatarFallback className="rounded-lg bg-neutral-200 text-black dark:bg-neutral-700 dark:text-white">
                    {initialsFromHandle(handle, did)}
                </AvatarFallback>
            </Avatar>
            <div className="grid flex-1 text-left text-sm leading-tight">
                <span className="truncate font-medium">{label}</span>
                {showDid && handle && (
                    <span className="truncate text-xs text-muted-foreground">
                        {truncateDid(did)}
                    </span>
                )}
            </div>
        </>
    );
}
