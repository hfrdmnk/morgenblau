// A save is filed by an ordinary tag on its record, so its place in the Library travels with the reader's data and needs no schema of
// its own. A save without it is still to read.
export type SaveState = 'queue' | 'archived';

const ARCHIVED = 'archived';

const isArchived = (tag: string) => tag.toLowerCase() === ARCHIVED;

export function saveState(tags: string[]): SaveState {
    return tags.some(isArchived) ? 'archived' : 'queue';
}

export function visibleTags(tags: string[]): string[] {
    return tags.filter((tag) => !isArchived(tag));
}

// The state tag leads, so a record already at the tag limit loses a tag of the reader's before it loses its place.
export function withState(tags: string[], state: SaveState): string[] {
    return state === 'queue' ? visibleTags(tags) : [ARCHIVED, ...visibleTags(tags)];
}

export function libraryTab(search: string): SaveState {
    return new URLSearchParams(search).get('tab') === ARCHIVED ? 'archived' : 'queue';
}

export type LibrarySource = {
    kind?: string;
    id?: string;
    rkey?: string;
    title: string | null;
    siteUrl?: string | null;
    feedUrl?: string;
    faviconUrl?: string | null;
};

type SaveFields = {
    createdAt: string;
    title?: string;
    entrySlug?: string;
    tags?: string[];
    minutes?: number;
};

export type LibrarySave =
    | (SaveFields & {
          kind?: undefined;
          rkey: string;
          itemUrl: string;
          targetUrl?: string;
          feedUrl?: string;
          contentType?: string;
          source?: LibrarySource;
      })
    | (SaveFields & { kind: 'newsletter'; id: string; messageId: string; source: LibrarySource });

export function saveKey(save: LibrarySave): string {
    return save.kind === 'newsletter' ? `newsletter:${save.id}` : `url:${save.rkey}`;
}

export function saveTagsPath(save: LibrarySave): string {
    return save.kind === 'newsletter'
        ? `/api/newsletter-saves/${encodeURIComponent(save.id)}`
        : `/api/saves/${encodeURIComponent(save.rkey)}`;
}

// The queue is sorted by the time a reader has rather than by when things arrived.
export function queueGroups(saves: LibrarySave[]): Array<{ title: string; saves: LibrarySave[] }> {
    const video = (save: LibrarySave) => save.kind !== 'newsletter' && save.contentType === 'video';
    const read = (save: LibrarySave, from: number, to = Infinity) =>
        !video(save) && !!save.minutes && save.minutes >= from && save.minutes <= to;
    return [
        { title: 'Quick reads', saves: saves.filter((s) => read(s, 1, 5)) },
        { title: 'Over coffee', saves: saves.filter((s) => read(s, 6, 15)) },
        { title: 'Deep dives', saves: saves.filter((s) => read(s, 16)) },
        // Videos carry no length yet, and a save whose post is no longer cached has nothing to measure.
        { title: 'To watch', saves: saves.filter(video) },
        { title: 'Links', saves: saves.filter((s) => !video(s) && !s.minutes) },
    ].filter((group) => group.saves.length);
}

const DAY = 24 * 60 * 60 * 1000;
const sameYear = new Intl.DateTimeFormat(undefined, { day: 'numeric', month: 'short' });
const otherYear = new Intl.DateTimeFormat(undefined, { day: 'numeric', month: 'short', year: 'numeric' });

export function savedLabel(createdAt: string, now = new Date()): string {
    const saved = new Date(createdAt);
    const midnight = new Date(now.getFullYear(), now.getMonth(), now.getDate()).getTime();
    const days = Math.ceil((midnight - saved.getTime()) / DAY);
    if (days <= 0) return 'Today';
    if (days === 1) return 'Yesterday';
    if (days < 7) return `${days} days ago`;
    return (saved.getFullYear() === now.getFullYear() ? sameYear : otherYear).format(saved);
}
