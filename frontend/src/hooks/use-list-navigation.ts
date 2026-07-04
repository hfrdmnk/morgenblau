import { useCallback, useRef, useState } from 'react';

export type ListNavigation = {
    active: number | null;
    setActive: (index: number) => void;
    clearPointer: () => void;
    move: (delta: number) => void;
    open: () => void;
    clear: () => void;
    scrollKey: number;
};

// One active index shared by pointer hover and keyboard selection, so the two can
// never paint competing highlights. Pointer enters set `active` and mark the input
// mode; keyboard `move` does the same and bumps `scrollKey` so the list scrolls on
// keyboard travel but stays put under the mouse.
//
// The highlight is intentionally a sighted-only power-user layer: it moves a
// purely visual index, not DOM focus or aria-activedescendant. Rows are real
// links, so assistive tech navigates the list fully by Tab / reading order.
export function useListNavigation<T>(
    items: readonly T[],
    onOpen: (item: T, index: number) => void,
): ListNavigation {
    const [active, setActiveState] = useState<number | null>(null);
    const [scrollKey, setScrollKey] = useState(0);
    const mode = useRef<'pointer' | 'keyboard'>('pointer');

    // Drop the selection when the list changes (day switch, refetch). Adjusting
    // during render avoids a setState-in-effect cascade.
    const [prevItems, setPrevItems] = useState(items);
    if (items !== prevItems) {
        setPrevItems(items);
        setActiveState(null);
    }

    const move = useCallback(
        (delta: number) => {
            mode.current = 'keyboard';
            setScrollKey((key) => key + 1);
            setActiveState((current) => {
                if (items.length === 0) return null;
                if (current === null) return delta > 0 ? 0 : items.length - 1;
                const next = current + delta;
                if (next < 0) return 0;
                if (next > items.length - 1) return items.length - 1;
                return next;
            });
        },
        [items.length],
    );

    const setActive = useCallback((index: number) => {
        mode.current = 'pointer';
        setActiveState(index);
    }, []);

    // Mouse left the list: drop the highlight only if the mouse owns it, so a
    // keyboard selection survives the pointer wandering away.
    const clearPointer = useCallback(() => {
        if (mode.current === 'pointer') setActiveState(null);
    }, []);

    const open = useCallback(() => {
        if (active === null) return;
        const item = items[active];
        if (item !== undefined) onOpen(item, active);
    }, [active, items, onOpen]);

    const clear = useCallback(() => setActiveState(null), []);

    return { active, setActive, clearPointer, move, open, clear, scrollKey };
}
