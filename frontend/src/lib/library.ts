import { api } from '@/lib/api';
import { entryHref } from '@/lib/paths';
import { hostnameOf, safeHref } from '@/lib/utils';

export type Save = {
    rkey: string;
    uri?: string;
    cid?: string;
    itemUrl: string;
    feedUrl?: string;
    createdAt: string;
    title?: string;
    targetUrl?: string;
    entrySlug?: string;
};

export async function fetchSaves(): Promise<Save[]> {
    return (await api<Save[] | null>('/api/saves')) ?? [];
}

export type SavePresentation = {
    label: string;
    href: string | undefined;
    external: boolean;
};

export function savePresentation(save: Save): SavePresentation {
    const externalHref = safeHref(save.targetUrl) ?? safeHref(save.itemUrl);
    const title = readableTitle(save.title);
    const label =
        title ||
        (externalHref ? hostnameOf(externalHref) : null) ||
        'Saved item';

    if (save.entrySlug) {
        return {
            label,
            href: entryHref(save.entrySlug),
            external: false,
        };
    }
    return {
        label,
        href: externalHref,
        external: Boolean(externalHref),
    };
}

function readableTitle(value: string | undefined): string | undefined {
    const title = value?.trim();
    if (!title || /^(?:https?:\/\/|at:\/\/)/i.test(title)) {
        return undefined;
    }
    return title;
}
