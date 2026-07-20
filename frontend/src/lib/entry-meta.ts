// Entry metadata is an untrusted JSON blob persisted by the backend.
export function readAuthor(metadata: string | null | undefined): string | null {
    if (!metadata) return null;
    try {
        const parsed = JSON.parse(metadata) as { author?: unknown };
        return typeof parsed.author === 'string' ? parsed.author : null;
    } catch {
        return null;
    }
}

export function metaLine(
    parts: Array<string | null | undefined>,
): string | null {
    const bits = parts.filter((p): p is string => Boolean(p));
    return bits.length > 0 ? bits.join(' · ') : null;
}
