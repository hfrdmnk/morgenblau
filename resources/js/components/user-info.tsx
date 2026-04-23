import { Avatar, AvatarFallback } from '@/components/ui/avatar';

function initialsFromHandle(handle: string | null, did: string): string {
    const source = handle ?? did.replace(/^did:[a-z]+:/, '');

    return source.slice(0, 2).toUpperCase();
}

function truncateDid(did: string): string {
    const suffix = did.replace(/^did:[a-z]+:/, '');

    if (suffix.length <= 10) {
        return did;
    }

    return `${did.slice(0, 12)}…${suffix.slice(-4)}`;
}

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
