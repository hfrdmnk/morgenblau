import { useEffect, useLayoutEffect, useRef } from 'react';
import type { RefObject } from 'react';

// Vertical breathing room between the pill and the row edges. Horizontal inset is
// handled by `inset-x-2` on the element so it stays a plain Tailwind concern.
const VGAP = 6;

// A single highlight that tracks the active row across a list. It measures the
// `[data-nav-row]` element at `active` and parks itself over it; on `scrollKey`
// change (keyboard travel only) it brings that row into view.
export function ListHighlight({
    containerRef,
    active,
    scrollKey,
}: {
    containerRef: RefObject<HTMLElement | null>;
    active: number | null;
    scrollKey: number;
}) {
    const pillRef = useRef<HTMLDivElement>(null);
    const activeRef = useRef(active);

    useEffect(() => {
        activeRef.current = active;
    }, [active]);

    useLayoutEffect(() => {
        const pill = pillRef.current;
        const container = containerRef.current;
        if (!pill || !container) return;
        if (active === null) {
            pill.style.opacity = '0';
            return;
        }
        const row =
            container.querySelectorAll<HTMLElement>('[data-nav-row]')[active];
        if (!row) {
            pill.style.opacity = '0';
            return;
        }
        const rowRect = row.getBoundingClientRect();
        const containerRect = container.getBoundingClientRect();
        const y = rowRect.top - containerRect.top + VGAP;
        const h = Math.max(0, rowRect.height - VGAP * 2);

        if (pill.style.opacity !== '1') {
            // Was hidden: jump to the target row without a slide, then fade in.
            pill.style.transition = 'none';
            pill.style.transform = `translateY(${y}px)`;
            pill.style.height = `${h}px`;
            void pill.offsetHeight;
            pill.style.transition = '';
        } else {
            pill.style.transform = `translateY(${y}px)`;
            pill.style.height = `${h}px`;
        }
        pill.style.opacity = '1';
    }, [active, containerRef]);

    useEffect(() => {
        const index = activeRef.current;
        const container = containerRef.current;
        if (index === null || !container) return;
        const rows = container.querySelectorAll<HTMLElement>('[data-nav-row]');
        rows[index]?.scrollIntoView({ block: 'nearest' });
    }, [scrollKey, containerRef]);

    return (
        <div
            ref={pillRef}
            aria-hidden
            className="list-highlight pointer-events-none absolute inset-x-2 top-0 z-0 rounded-lg bg-overlay-2 opacity-0 will-change-transform"
        />
    );
}
