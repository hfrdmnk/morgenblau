import { useEffect, useRef } from 'react';

export type KeyHandler = (event: KeyboardEvent) => void;
export type KeyMap = Record<string, KeyHandler>;

// Enter/Space activate the focused control natively, so we never hijack them when a link or button has focus.
const ACTIVATION_KEYS = new Set(['Enter', ' ']);

function isEditableTarget(target: EventTarget | null): boolean {
    if (!(target instanceof HTMLElement)) return false;
    if (target.isContentEditable) return true;
    return /^(INPUT|TEXTAREA|SELECT)$/.test(target.tagName);
}

function isActivationTarget(target: EventTarget | null): boolean {
    return (
        target instanceof HTMLElement &&
        target.closest('a[href],button,[role="button"],summary') !== null
    );
}

function isOverlayOpen(): boolean {
    return (
        document.querySelector('[role="dialog"],[role="menu"],[role="listbox"]') !==
        null
    );
}

// Read through a ref so callers can pass a fresh map each render without re-subscribing the listener.
export function useKeyboard(map: KeyMap, enabled = true) {
    const mapRef = useRef(map);

    useEffect(() => {
        mapRef.current = map;
    }, [map]);

    useEffect(() => {
        if (!enabled) return;
        const onKeyDown = (event: KeyboardEvent) => {
            if (event.defaultPrevented || event.isComposing) return;
            if (event.metaKey || event.ctrlKey || event.altKey) return;
            if (isEditableTarget(event.target) || isOverlayOpen()) return;
            if (
                ACTIVATION_KEYS.has(event.key) &&
                isActivationTarget(event.target)
            ) {
                return;
            }
            const handler = mapRef.current[event.key];
            if (!handler) return;
            event.preventDefault();
            handler(event);
        };
        document.addEventListener('keydown', onKeyDown);
        return () => document.removeEventListener('keydown', onKeyDown);
    }, [enabled]);
}
