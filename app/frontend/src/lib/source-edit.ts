import type { FeedSource, NewsletterSource } from '@/hooks/use-sources';

import { api } from './api';

export type EditableSource =
    | { kind: 'feed'; rkey: string; title: string; tags: string[] }
    | { kind: 'newsletter'; id: string; title: string; tags: string[]; stopped: boolean };

// The subscription lexicon caps a record at ten tags of 64 graphemes and titles at 128.
export const MAX_TAGS = 10;
export const MAX_TAG_LENGTH = 64;
export const MAX_TITLE_LENGTH = 128;

export function feedEditable(source: FeedSource, title: string): EditableSource {
    return { kind: 'feed', rkey: source.rkey, title, tags: source.tags ?? [] };
}

export function newsletterEditable(source: NewsletterSource, stopped: boolean): EditableSource {
    return { kind: 'newsletter', id: source.id, title: source.title, tags: source.tags, stopped };
}

function sourcePath(source: EditableSource): string {
    return source.kind === 'feed'
        ? `/api/subscriptions/${encodeURIComponent(source.rkey)}`
        : `/api/newsletters/${encodeURIComponent(source.id)}`;
}

// Both PATCH routes change only the fields they are sent, so an edit never rewrites a field from a stale copy.
function patchSource(source: EditableSource, change: { title: string } | { tags: string[] }) {
    return api(sourcePath(source), { method: 'PATCH', body: change });
}

export function renameSource(source: EditableSource, title: string) {
    return patchSource(source, { title: title.trim() });
}

export function retagSource(source: EditableSource, tags: string[]) {
    return patchSource(source, { tags });
}

// V1 never deletes a newsletter: stopping it is its removal, and a stopped one can resume.
export function removeSource(source: EditableSource) {
    if (source.kind === 'feed') return api(sourcePath(source), { method: 'DELETE' });
    return api(`${sourcePath(source)}/stop`, { method: 'POST' });
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
