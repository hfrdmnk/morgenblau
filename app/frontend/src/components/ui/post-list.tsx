import { cva, type VariantProps } from 'class-variance-authority';
import { useEffect, useRef, type ReactNode } from 'react';
import { Link as RouterLink } from 'wouter';

import { shortcutKey } from '@/lib/shortcuts';
import { cn } from 'cn';

function PostList({ children, arrowsOnly = false }: { children: ReactNode; arrowsOnly?: boolean }) {
    const root = useRef<HTMLDivElement>(null);
    useEffect(() => {
        const list = root.current!;
        let source: 'pointer' | 'keys' | null = null;
        let pointer = { x: -1, y: -1 };
        const rows = () =>
            [...list.querySelectorAll<HTMLElement>('[data-post]')].filter((row) => !row.closest('[inert]'));
        const selected = () => list.querySelector<HTMLElement>('[data-post][data-selected]');
        const link = (row: HTMLElement) => row.querySelector<HTMLElement>('[data-post-link]')!;
        const rowOf = (target: EventTarget | null) =>
            target instanceof Element ? target.closest<HTMLElement>('[data-post]') : null;
        function select(row: HTMLElement | null, by: typeof source) {
            const current = selected();
            if (current !== row) current?.removeAttribute('data-selected');
            row?.setAttribute('data-selected', '');
            source = row ? by : null;
        }

        function onKeyDown(event: KeyboardEvent) {
            const key = shortcutKey(event);
            if (key === 'ArrowDown' || (key === 'j' && !arrowsOnly)) move(event, 1);
            else if (key === 'ArrowUp' || (key === 'k' && !arrowsOnly)) move(event, -1);
            else if (key === 'Enter') open(event);
        }
        function move(event: KeyboardEvent, step: number) {
            const all = rows();
            if (!all.length) return;
            event.preventDefault();
            const current = selected() ?? list.querySelector<HTMLElement>('[data-post][aria-current]');
            const next = all[current ? Math.min(Math.max(all.indexOf(current) + step, 0), all.length - 1) : 0];
            select(next, 'keys');
            link(next).focus({ preventScroll: true });
            if (next === all[0]) window.scrollTo(0, 0);
            else next.scrollIntoView({ block: 'nearest' });
        }
        function open(event: KeyboardEvent) {
            const current = selected();
            const onPostOrPage =
                event.target === document.body ||
                (event.target instanceof Element && event.target.matches('[data-post-link]'));
            if (!current || !onPostOrPage) return;
            event.preventDefault();
            link(current).click();
        }
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
            if (source === 'keys' && !(event.relatedTarget instanceof Node && list.contains(event.relatedTarget)))
                select(null, null);
        }
        function onClick(event: MouseEvent) {
            const row = rowOf(event.target);
            if (
                !row ||
                (event.target instanceof Element && event.target.closest('a, button')) ||
                !document.getSelection()?.isCollapsed
            )
                return;
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
    }, [arrowsOnly]);
    return (
        <div
            data-slot="post-list"
            className="[&_[data-post]]:cursor-pointer [&_[data-post]]:scroll-mb-6 [&_[data-post]]:rounded-lg [&_[data-post][data-selected]]:bg-secondary [&_[data-selected]_[data-post-link]]:outline-none"
            ref={root}
        >
            {children}
        </div>
    );
}

const postRowVariants = cva('relative', {
    variants: {
        size: {
            default: '-mx-3 -my-2 px-3 py-2',
            compact: '-mx-2 -my-1.5 min-w-0 px-2 py-1.5',
        },
    },
    defaultVariants: { size: 'default' },
});

function PostRow({
    as: Row = 'div',
    size,
    children,
}: { as?: 'div' | 'li'; children: ReactNode } & VariantProps<typeof postRowVariants>) {
    return (
        <Row data-post="" data-slot="post-row" className={postRowVariants({ size })}>
            {children}
        </Row>
    );
}

function PostRowActions({ children }: { children: ReactNode }) {
    return (
        <div
            data-slot="post-row-actions"
            className="pointer-events-none absolute inset-y-0 end-0 flex items-center gap-0.5 rounded-e-lg pe-1.5 ps-12 opacity-0 post-row-actions-fade transition-opacity duration-(--motion-duration-fast) pointer-coarse:hidden focus-within:pointer-events-auto focus-within:opacity-100 in-data-selected:pointer-events-auto in-data-selected:opacity-100"
        >
            {children}
        </div>
    );
}

function FoldedPosts({ children }: { children: ReactNode }) {
    return (
        <ol data-slot="folded-posts" className="ml-1.75 flex flex-col gap-5 border-l border-border pt-5 pl-4.25">
            {children}
        </ol>
    );
}

type FeatureImage = { href: string; src: string; onError: () => void };

function FeaturePost({
    source,
    headline,
    excerpt,
    image,
}: {
    source: ReactNode;
    headline: ReactNode;
    excerpt?: ReactNode;
    image?: FeatureImage | null;
}) {
    return (
        <div
            data-post=""
            data-slot="feature-post"
            className={cn(
                '-m-3 flex flex-col gap-3 p-3',
                image && 'sm:grid sm:grid-cols-(--grid-trailing) sm:grid-rows-(--grid-feature-rows) sm:gap-x-8',
            )}
        >
            <div className="min-w-0 self-start sm:col-start-1 sm:row-start-1">{source}</div>
            <div className="*:tracking-tight sm:col-start-1 sm:row-start-2">{headline}</div>
            {excerpt ? (
                <div className="max-w-(--excerpt-measure) self-start sm:col-start-1 sm:row-start-3">{excerpt}</div>
            ) : null}
            {image ? (
                <RouterLink
                    aria-hidden
                    className="order-first mb-1 block aspect-2/1 w-full overflow-hidden rounded-lg bg-muted ring-1 ring-foreground-faint sm:order-none sm:col-start-2 sm:row-span-2 sm:row-start-2 sm:mb-0 sm:aspect-4/3 sm:w-44 sm:self-start"
                    href={image.href}
                    tabIndex={-1}
                >
                    <img
                        alt=""
                        className="size-full object-cover"
                        decoding="async"
                        loading="lazy"
                        onError={image.onError}
                        src={image.src}
                    />
                </RouterLink>
            ) : null}
        </div>
    );
}

export { FeaturePost, FoldedPosts, PostList, PostRow, PostRowActions };
