import type { FeedSource, NewsletterSource } from '@/hooks/use-sources';

import { api, ApiError } from './api';

export type EditableSource =
    | { kind: 'feed'; rkey: string; title: string; tags: string[] }
    | { kind: 'newsletter'; id: string; title: string; tags: string[]; stopped: boolean; canUnsubscribe: boolean; latestEntrySlug?: string };

// The subscription lexicon caps a record at ten tags of 64 graphemes and titles at 128.
export const MAX_TAGS = 10;
export const MAX_TAG_LENGTH = 64;
export const MAX_TITLE_LENGTH = 128;

export const GONE = 'This source is no longer in your sources.';

export const REMOVAL = {
    feed: {
        title: (name: string) => `Delete “${name}”?`,
        description: 'Its posts leave your digest. Posts you saved stay in your Library.',
        action: 'Delete source',
        done: 'Source deleted',
        failed: 'Couldn’t delete this source.',
    },
    newsletter: {
        title: (name: string) => `Stop “${name}”?`,
        description: 'Unsaved issues are deleted and new mail from this sender is discarded. Saved issues stay in your Library, and you can resume it later.',
        action: 'Stop newsletter',
        done: 'Newsletter stopped',
        failed: 'Couldn’t stop this newsletter.',
    },
};

export const DELETION = {
    automatic: {
        title: (name: string) => `Unsubscribe from “${name}”?`,
        description: 'Morgenblau asks the sender to remove your address, then deletes this newsletter and its unsaved issues. Saved issues stay in your Library.',
        action: 'Unsubscribe and delete',
        done: 'Unsubscribed and deleted',
    },
    manual: {
        title: (name: string) => `Delete “${name}”?`,
        description: 'Unsubscribe manually, usually through the link at the bottom of an issue, then delete it here. Saved issues stay in your Library. If you don’t unsubscribe, its next issue shows up again as a new newsletter.',
        action: 'Delete newsletter',
        done: 'Newsletter deleted',
    },
};

export function isGone(error: unknown): boolean {
    return error instanceof ApiError && error.status === 404;
}

export function feedEditable(source: FeedSource, title: string): EditableSource {
    return { kind: 'feed', rkey: source.rkey, title, tags: source.tags ?? [] };
}

export function newsletterEditable(source: NewsletterSource, stopped: boolean): Extract<EditableSource, { kind: 'newsletter' }> {
    return { kind: 'newsletter', id: source.id, title: source.title, tags: source.tags, stopped, canUnsubscribe: source.canUnsubscribe, latestEntrySlug: source.latestEntrySlug };
}

function sourcePath(source: EditableSource): string {
    return source.kind === 'feed'
        ? `/api/subscriptions/${encodeURIComponent(source.rkey)}`
        : `/api/newsletters/${encodeURIComponent(source.id)}`;
}

// The PATCH route changes only the fields it is sent, so an edit never rewrites a field from a stale copy.
export function editNewsletter(source: Extract<EditableSource, { kind: 'newsletter' }>, change: { title?: string; tags?: string[]; primary?: boolean }) {
    return api(sourcePath(source), { method: 'PATCH', body: change });
}

// Stopping is a newsletter's reversible removal; deleting one is unsubscribeNewsletter or deleteNewsletter.
export function removeSource(source: EditableSource) {
    if (source.kind === 'feed') return api(sourcePath(source), { method: 'DELETE' });
    return api(`${sourcePath(source)}/stop`, { method: 'POST' });
}

// The server deletes the newsletter only after the sender confirms the one-click unsubscribe.
export function unsubscribeNewsletter(source: Extract<EditableSource, { kind: 'newsletter' }>) {
    return api(`${sourcePath(source)}/unsubscribe`, { method: 'POST' });
}

export function deleteNewsletter(source: Extract<EditableSource, { kind: 'newsletter' }>) {
    return api(sourcePath(source), { method: 'DELETE' });
}

export function resumeNewsletter(source: Extract<EditableSource, { kind: 'newsletter' }>) {
    return api(`${sourcePath(source)}/enable`, { method: 'POST' });
}

// Code points, as the server counts; the lexicon's grapheme limit is never stricter.
function length(text: string): number {
    return [...text].length;
}

export function titleTooLong(title: string): boolean {
    return length(title.trim()) > MAX_TITLE_LENGTH;
}

export function hasOverlongTag(input: string): boolean {
    return input.split(',').some((tag) => length(tag.trim()) > MAX_TAG_LENGTH);
}

export function canAddTags(tags: string[], input: string): boolean {
    return tags.length < MAX_TAGS && input.trim() !== '' && !hasOverlongTag(input);
}

export function addTags(tags: string[], input: string): string[] {
    const next = [...tags];
    const seen = new Set(tags.map((tag) => tag.toLowerCase()));
    for (const raw of input.split(',')) {
        const tag = raw.trim();
        if (!tag || seen.has(tag.toLowerCase()) || next.length >= MAX_TAGS) continue;
        seen.add(tag.toLowerCase());
        next.push(tag);
    }
    return next;
}
