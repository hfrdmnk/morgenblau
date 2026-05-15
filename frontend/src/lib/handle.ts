export function initialsFromHandle(handle: string | null, did: string): string {
    const source = handle ?? did.replace(/^did:[a-z]+:/, '');

    return source.slice(0, 2).toUpperCase();
}

export function truncateDid(did: string): string {
    const suffix = did.replace(/^did:[a-z]+:/, '');

    if (suffix.length <= 10) {
        return did;
    }

    return `${did.slice(0, 12)}…${suffix.slice(-4)}`;
}
