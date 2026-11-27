export type LibraryView = 'recent' | 'everything';

export const RECENT_SIZE = 10;

export function libraryView(search: string): LibraryView {
    return new URLSearchParams(search).get('view') === 'everything' ? 'everything' : 'recent';
}

export type LibrarySort = 'newest' | 'title' | 'site';

export function librarySort(search: string): LibrarySort {
    const sort = new URLSearchParams(search).get('sort');
    return sort === 'title' || sort === 'site' ? sort : 'newest';
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

export function saveTitle(save: LibrarySave): string {
    return save.title || 'Untitled';
}

export function saveHost(save: LibrarySave): string | null {
    if (save.kind === 'newsletter') return null;
    try {
        return new URL(save.itemUrl).hostname.replace(/^www\./, '');
    } catch {
        return save.itemUrl;
    }
}

export function saveSourceLabel(save: LibrarySave): string {
    return save.source?.title || saveHost(save) || 'Newsletter';
}

export const fold = (text: string) => text.normalize('NFD').replace(/\p{M}/gu, '').toLocaleLowerCase();

export function matchesQuery(save: LibrarySave, query: string): boolean {
    const terms = fold(query)
        .split(/\s+/)
        .filter((term) => term && term !== '#');
    const tags = (save.tags ?? []).map(fold);
    const text = fold([saveTitle(save), saveSourceLabel(save), saveHost(save) ?? '', ...(save.tags ?? [])].join(' '));
    return terms.every((term) => (term.startsWith('#') ? tags.includes(term.slice(1)) : text.includes(term)));
}

const collator = new Intl.Collator(undefined, { sensitivity: 'base', numeric: true });

export function groupSaves(
    saves: LibrarySave[],
    sort: LibrarySort,
    now = new Date(),
): Array<{ title: string; saves: LibrarySave[] }> {
    const sorted =
        sort === 'title'
            ? [...saves].sort((a, b) => collator.compare(saveTitle(a), saveTitle(b)))
            : sort === 'site'
              ? [...saves].sort((a, b) => collator.compare(saveSourceLabel(a), saveSourceLabel(b)))
              : saves;
    const key = (save: LibrarySave) => {
        if (sort === 'site') return saveSourceLabel(save);
        if (sort === 'newest') return monthLabel(save.createdAt, now);
        const first = saveTitle(save).normalize('NFD').charAt(0).toLocaleUpperCase();
        return /\p{L}/u.test(first) ? first : '#';
    };
    const groups = new Map<string, LibrarySave[]>();
    for (const save of sorted) groups.set(key(save), [...(groups.get(key(save)) ?? []), save]);
    return [...groups].map(([title, saves]) => ({ title, saves }));
}

const monthOnly = new Intl.DateTimeFormat(undefined, { month: 'long' });
const monthAndYear = new Intl.DateTimeFormat(undefined, { month: 'long', year: 'numeric' });

function monthLabel(createdAt: string, now: Date): string {
    const saved = new Date(createdAt);
    return (saved.getFullYear() === now.getFullYear() ? monthOnly : monthAndYear).format(saved);
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

export function savedPhrase(savedAt: string, now = new Date()): string {
    const label = savedLabel(savedAt, now);
    if (label === 'Today' || label === 'Yesterday') return `Saved to your Library ${label.toLowerCase()}`;
    if (label.endsWith(' ago')) return `Saved to your Library ${label}`;
    return `Saved to your Library on ${label}`;
}
