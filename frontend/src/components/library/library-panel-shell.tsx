import { useCallback, useRef } from 'react';
import type { ComponentType, ReactNode } from 'react';
import { useLocation } from 'wouter';

import { CardMasthead } from '@/components/card-masthead';
import { ListHighlight } from '@/components/list-highlight';
import { useListNavKeyboard } from '@/hooks/use-list-nav-keyboard';
import {
    useListNavigation,
    type ListNavigation,
} from '@/hooks/use-list-navigation';
import { shareTargetPresentation, type ShareTarget } from '@/lib/share-target';
import { cn } from '@/lib/utils';

export function Divider() {
    return <div aria-hidden className="mx-6 border-t border-border" />;
}

// Card scaffold shared by every list tab: masthead, nav-highlight tracking, and
// open-on-select behavior (external link vs in-app route) driven by ShareTarget fields.
export function ListPanelShell<T extends ShareTarget>({
    eyebrow,
    heading,
    items,
    children,
}: {
    eyebrow: string;
    heading: string;
    items: readonly T[];
    children: (nav: ListNavigation) => ReactNode;
}) {
    const [, navigate] = useLocation();
    const onOpen = useCallback(
        (item: T) => {
            const target = shareTargetPresentation(item);
            if (!target.href) return;
            if (target.external) {
                window.open(target.href, '_blank', 'noopener,noreferrer');
            } else {
                navigate(target.href);
            }
        },
        [navigate],
    );
    const nav = useListNavigation(items, onOpen);
    useListNavKeyboard(nav);

    const listRef = useRef<HTMLDivElement>(null);

    return (
        <article className="overflow-hidden rounded-xl bg-card shadow-card">
            <CardMasthead eyebrow={eyebrow} heading={heading} />
            <Divider />
            <div
                ref={listRef}
                className="relative"
                onMouseLeave={nav.clearPointer}
            >
                <ListHighlight
                    containerRef={listRef}
                    active={nav.active}
                    scrollKey={nav.scrollKey}
                />
                <div className="relative z-10">{children(nav)}</div>
            </div>
        </article>
    );
}

export function SectionState({
    icon: Icon,
    spin,
    lead,
    detail,
}: {
    icon?: ComponentType<{ className?: string }>;
    spin?: boolean;
    lead: string;
    detail?: string;
}) {
    return (
        <div className="flex flex-col items-center gap-2 px-6 py-10 text-center">
            {Icon ? (
                <Icon
                    className={cn(
                        'size-6 text-muted-foreground',
                        spin && 'motion-safe:animate-spin',
                    )}
                />
            ) : null}
            <p>{lead}</p>
            {detail ? (
                <p className="text-sm font-light text-muted-foreground">
                    {detail}
                </p>
            ) : null}
        </div>
    );
}
