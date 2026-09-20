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
