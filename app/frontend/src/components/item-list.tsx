import { useEffect, useRef, type ReactNode } from 'react';
import { Link } from 'wouter';

import { Button } from '@/components/ui/button';
import { shortcutKey } from '@/lib/shortcuts';
import { cn } from '@/lib/utils';

// The arrows, or j and k, walk the posts and Enter opens the selected one; the pointer selects too, as in the command menu. Selection is a
// data attribute the list sets on its rows, so a page marks each post's row with data-post and the link that opens it with
// data-post-link. A selected row's highlight is its focus indicator, so its link drops the ring.
export function PostList({ children, className }: { children: ReactNode; className?: string }) {
    const root = useRef<HTMLDivElement>(null);
    useEffect(() => {
        const list = root.current!;
        let source: 'pointer' | 'keys' | null = null;
        let pointer = { x: -1, y: -1 };
        // Sample rows mount inert while a skeleton shows.
        const rows = () => [...list.querySelectorAll<HTMLElement>('[data-post]')].filter((row) => !row.closest('[inert]'));
        const selected = () => list.querySelector<HTMLElement>('[data-post][data-selected]');
        const link = (row: HTMLElement) => row.querySelector<HTMLAnchorElement>('[data-post-link]')!;
        const rowOf = (target: EventTarget | null) => (target instanceof Element ? target.closest<HTMLElement>('[data-post]') : null);
        function select(row: HTMLElement | null, by: typeof source) {
            const current = selected();
            if (current !== row) current?.removeAttribute('data-selected');
            row?.setAttribute('data-selected', '');
            source = row ? by : null;
        }

        function onKeyDown(event: KeyboardEvent) {
            const key = shortcutKey(event);
            if (key === 'ArrowDown' || key === 'j') move(event, 1);
            else if (key === 'ArrowUp' || key === 'k') move(event, -1);
            else if (key === 'Enter') open(event);
        }
        function move(event: KeyboardEvent, step: number) {
            const all = rows();
            if (!all.length) return;
            event.preventDefault();
            const current = selected();
            const next = all[current ? Math.min(Math.max(all.indexOf(current) + step, 0), all.length - 1) : 0];
            select(next, 'keys');
            link(next).focus({ preventScroll: true });
            // Back at the first post, the page's heading above it comes back into view too.
            if (next === all[0]) window.scrollTo(0, 0);
            else next.scrollIntoView({ block: 'nearest' });
        }
        // A focused post link would open itself, but the pointer may have selected another row since.
        function open(event: KeyboardEvent) {
            const current = selected();
            const onPostOrPage = event.target === document.body || (event.target instanceof Element && event.target.matches('[data-post-link]'));
            if (!current || !onPostOrPage) return;
            event.preventDefault();
            link(current).click();
        }
        // Scrolling moves rows under a still pointer, and some browsers report that as a move; only a real move selects.
        function onPointerMove(event: PointerEvent) {
            if (event.pointerType !== 'mouse' || (event.clientX === pointer.x && event.clientY === pointer.y)) return;
            pointer = { x: event.clientX, y: event.clientY };
            const row = rowOf(event.target);
            if (row) select(row, 'pointer');
            else if (source === 'pointer') select(null, null);
        }
        function onPointerLeave() {
            if (source === 'pointer') select(null, null);
        }
        function onFocusIn(event: FocusEvent) {
            const row = rowOf(event.target);
            if (row) select(row, 'keys');
        }
        function onFocusOut(event: FocusEvent) {
            if (source === 'keys' && !(event.relatedTarget instanceof Node && list.contains(event.relatedTarget))) select(null, null);
        }
        // The whole row reads as the post, so a click anywhere on it opens the post, except on the row's own controls.
        function onClick(event: MouseEvent) {
            const row = rowOf(event.target);
            if (!row || (event.target instanceof Element && event.target.closest('a, button')) || !document.getSelection()?.isCollapsed) return;
            link(row).click();
        }

        window.addEventListener('keydown', onKeyDown);
        list.addEventListener('pointermove', onPointerMove);
        list.addEventListener('pointerleave', onPointerLeave);
        list.addEventListener('focusin', onFocusIn);
        list.addEventListener('focusout', onFocusOut);
        list.addEventListener('click', onClick);
        return () => {
            window.removeEventListener('keydown', onKeyDown);
            list.removeEventListener('pointermove', onPointerMove);
            list.removeEventListener('pointerleave', onPointerLeave);
            list.removeEventListener('focusin', onFocusIn);
            list.removeEventListener('focusout', onFocusOut);
            list.removeEventListener('click', onClick);
        };
    }, []);
    return (
        <div
            className={cn(
                '[&_[data-post]]:cursor-pointer [&_[data-post]]:scroll-mb-6 [&_[data-post]]:rounded-lg [&_[data-post][data-selected]]:bg-secondary [&_[data-selected]_[data-post-link]]:outline-none',
                className,
            )}
            ref={root}
        >
            {children}
        </div>
    );
}

export function ItemList({ children, label, ordered = false }: {
    children: ReactNode;
    label?: string;
    ordered?: boolean;
}) {
    const List = ordered ? 'ol' : 'ul';
    return <List aria-label={label} className="space-y-8">{children}</List>;
}

export function ListItem({ title, href, aside, icon, meta, status, note, actions }: {
    title: string;
    href: string;
    aside?: ReactNode;
    icon?: string | null;
    meta?: ReactNode;
    status?: ReactNode;
    note?: ReactNode;
    actions?: ReactNode;
}) {
    return (
        <li className="flex min-w-0 items-center gap-3">
            <div className="min-w-0 flex-1">
                <div className="-mx-3 -my-2 px-3 py-2" data-post>
                    <div className="flex min-w-0 items-baseline gap-3">
                        <Link
                            className="block min-w-0 truncate rounded-sm text-lg font-medium text-foreground hover:text-foreground/75"
                            data-post-link
                            href={href}
                            title={title}
                        >
                            {title}
                        </Link>
                        {aside ? <span className="shrink-0 text-xs text-muted-foreground">{aside}</span> : null}
                    </div>
                    {icon || meta ? (
                        <div className="mt-2 flex min-w-0 items-center gap-2.5 text-sm text-subtle-foreground">
                            <SourceIcon src={icon} />
                            <span className="min-w-0 truncate">{meta}</span>
                            {status}
                        </div>
                    ) : null}
                </div>
                {note}
            </div>
            {actions}
        </li>
    );
}

export function SourceIcon({ src, className }: { src?: string | null; className?: string }) {
    if (!src) return null;
    return (
        <img
            alt=""
            className={cn('size-4 shrink-0 rounded-sm object-cover', className)}
            loading="lazy"
            onError={(event) => {
                event.currentTarget.hidden = true;
            }}
            referrerPolicy="no-referrer"
            src={src}
        />
    );
}

export function ListError({ label, retry }: { label: string; retry: () => void }) {
    return (
        <div className="flex flex-wrap items-center gap-4 text-sm text-muted-foreground" role="alert">
            <p>Couldn’t load {label}.</p>
            <Button onClick={retry} size="sm" variant="secondary">Try again</Button>
        </div>
    );
}
