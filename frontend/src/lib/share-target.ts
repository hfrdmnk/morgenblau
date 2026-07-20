import { entryHref } from '@/lib/paths';
import { hostnameOf, safeHref } from '@/lib/utils';

export type ShareTarget = {
    title?: string;
    targetUrl?: string;
    entrySlug?: string;
    itemUrl?: string;
    document?: string;
};

export type ShareTargetPresentation = {
    label: string;
    href: string | undefined;
    external: boolean;
};

export function shareTargetPresentation(
    target: ShareTarget,
): ShareTargetPresentation {
    const externalHref = safeHref(target.targetUrl) ?? safeHref(target.itemUrl);
    const title = readableTitle(target.title);
    const label =
        title ||
        (externalHref ? hostnameOf(externalHref) : null) ||
        'Shared item';

    if (target.entrySlug) {
        return {
            label,
            href: entryHref(target.entrySlug),
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
