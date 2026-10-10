import { type RefObject, useEffect } from 'react';

export function useHeightProperty(ref: RefObject<HTMLElement | null>, property: string) {
    useEffect(() => {
        const element = ref.current;
        if (!element) return;
        const root = document.documentElement.style;
        const observer = new ResizeObserver(() => {
            root.setProperty(property, `${element.getBoundingClientRect().height}px`);
        });
        observer.observe(element);
        return () => {
            observer.disconnect();
            root.removeProperty(property);
        };
    }, [ref, property]);
}
