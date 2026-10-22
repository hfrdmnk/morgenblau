import { useEffect, useRef } from 'react';
import { toast } from 'sonner';
import { useLocation } from 'wouter';

import { useShortcuts } from '@/hooks/use-shortcuts';
import { api } from '@/lib/api';
import { parseDigestDate } from '@/lib/digest-dates';
import { entryHref, entryListPath, sourceOrigin } from '@/lib/paths';
import { showShortcutNotice } from '@/lib/shortcuts';

type ListedEntry = { entrySlug: string };

async function loadSlugs(path: string, signal: AbortSignal): Promise<string[]> {
    const data = await api<ListedEntry[] | { entries: ListedEntry[] }>(path, { signal });
    return (Array.isArray(data) ? data : data.entries).map((entry) => entry.entrySlug);
}

function neighbour(slugs: string[], slug: string, offset: number): string | undefined {
    const index = slugs.indexOf(slug);
    return index < 0 ? undefined : slugs[index + offset];
}

function edgeNotice(from: string | null, offset: number): string {
    const place = sourceOrigin(from) ? ' from this source' : parseDigestDate(from) ? ' in this digest' : '';
    return `No ${offset > 0 ? 'next' : 'previous'} article${place}`;
}

// Steps through the list the entry was opened from. Steps queue, so presses made while the list loads each move one entry.
export function useEntrySteps(slug: string, from: string | null) {
    const [, navigate] = useLocation();
    const list = useRef<{ path: string; slugs: string[] } | null>(null);
    const lifetime = useRef(new AbortController());
    const queue = useRef(Promise.resolve());
    const at = useRef(slug);

    useEffect(() => {
        at.current = slug;
    }, [slug]);

    useEffect(() => {
        const controller = new AbortController();
        lifetime.current = controller;
        return () => controller.abort();
    }, []);

    async function load(path: string): Promise<string[] | null> {
        const signal = lifetime.current.signal;
        try {
            const slugs = await loadSlugs(path, signal);
            list.current = { path, slugs };
            return slugs;
        } catch {
            if (!signal.aborted) toast.error('Couldn’t load the article list', { id: 'entry-step' });
            return null;
        }
    }

    // The kept list may predate entries collected since, so only a fresh one confirms an edge. Null means loading failed.
    async function targetIn(path: string, offset: number): Promise<string | undefined | null> {
        const kept = list.current?.path === path ? neighbour(list.current.slugs, at.current, offset) : undefined;
        if (kept) return kept;
        const slugs = await load(path);
        return slugs && neighbour(slugs, at.current, offset);
    }

    async function move(offset: 1 | -1) {
        if (lifetime.current.signal.aborted) return;
        const path = entryListPath(from, Intl.DateTimeFormat().resolvedOptions().timeZone);
        const target = path ? await targetIn(path, offset) : undefined;
        if (target === null) return;
        if (!target) return showShortcutNotice(edgeNotice(from, offset));
        at.current = target;
        navigate(entryHref(target, from ?? ''));
    }

    const step = (offset: 1 | -1) => {
        queue.current = queue.current.then(() => move(offset));
    };
    const previous = () => step(-1);
    const next = () => step(1);
    useShortcuts({ 'Shift+ArrowUp': previous, k: previous, 'Shift+ArrowDown': next, j: next });
}
