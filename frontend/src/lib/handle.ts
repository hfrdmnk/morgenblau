export function initialsFromHandle(
    handle: string | null | undefined,
    did: string,
): string {
    const source = handle ?? did.replace(/^did:[a-z]+:/, '');

    return source.slice(0, 2).toUpperCase();
}

export type PersonRowLines = {
    primary: string;
    secondary: string | undefined;
};

// handleAsSecondary shows the @handle under the name only when a display name exists;
// an explicit secondary always wins.
export function personRowLines({
    handle,
    displayName,
    did,
    secondary,
    handleAsSecondary,
}: {
    handle: string | undefined;
    displayName: string | null | undefined;
    did: string;
    secondary?: string;
    handleAsSecondary?: boolean;
}): PersonRowLines {
    const label = handle ? `@${handle}` : truncateDid(did);
    const name = displayName?.trim() || undefined;

    return {
        primary: name ?? label,
        secondary:
            secondary ?? (handleAsSecondary && name ? label : undefined),
    };
}

export function truncateDid(did: string): string {
    const suffix = did.replace(/^did:[a-z]+:/, '');

    if (suffix.length <= 10) {
        return did;
    }

    return `${did.slice(0, 12)}…${suffix.slice(-4)}`;
}
