import type { Entry } from '@/components/digest-rows';
import { entryHref, type EntryFrom } from '@/lib/paths';
import { safeHref } from '@/lib/utils';

export type EntryActivation = { href: string; external: boolean };

// Single source of truth for what activating a row does, shared by the row and keyboard Enter so they never diverge.
export function entryActivation(
    entry: Entry,
    from?: EntryFrom,
): EntryActivation | null {
    const opensInReader =
        entry.contentType === 'blogpost' || entry.contentType === 'video';
    if (opensInReader) {
        return { href: entryHref(entry.entrySlug, from), external: false };
    }
    // Microblogs render inline with no row-level target, so Enter must not diverge by opening a new tab; the RowHeader link icon is the affordance instead.
    if (entry.contentType === 'microblog') {
        return null;
    }
    const link = safeHref(entry.url);
    return link ? { href: link, external: true } : null;
}
