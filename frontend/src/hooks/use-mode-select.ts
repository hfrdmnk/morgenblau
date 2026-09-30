import { useReducedMotion } from 'motion/react';
import { useEffect, useRef, useState } from 'react';

export function useModeSelect<T>(
    current: T,
    modes: readonly T[],
    onOpenChange: (open: boolean) => void,
) {
    const [itemsVisible, setItemsVisible] = useState(false);
    const [settling, setSettling] = useState(false);
    const [targetIndex, setTargetIndex] = useState<number | null>(null);
    const [menuModes, setMenuModes] = useState(() => [
        current,
        ...modes.filter((mode) => mode !== current),
    ]);
    const reduceMotion = useReducedMotion() ?? false;
    const timers = useRef<number[]>([]);
    const selectedMode = targetIndex === null ? current : menuModes[targetIndex] ?? current;

    useEffect(() => {
        const scheduled = timers.current;
        return () => scheduled.forEach(window.clearTimeout);
    }, []);

    function clearTimers() {
        timers.current.forEach(window.clearTimeout);
        timers.current.length = 0;
    }

    function finishClose(delay: number) {
        clearTimers();
        const fadeDuration = reduceMotion ? 0 : 140;
        timers.current.push(
            window.setTimeout(() => {
                setItemsVisible(false);
                setSettling(false);
            }, delay),
            window.setTimeout(() => {
                onOpenChange(false);
                setTargetIndex(null);
            }, delay + fadeDuration),
        );
    }

    function selectMode(index: number) {
        clearTimers();
        setTargetIndex(index);
        setSettling(true);
        if (reduceMotion) {
            setItemsVisible(false);
            setSettling(false);
            onOpenChange(false);
            setTargetIndex(null);
            return;
        }
        timers.current.push(
            window.setTimeout(() => setSettling(false), 16),
            window.setTimeout(() => {
                onOpenChange(false);
                setTargetIndex(null);
            }, 220),
        );
    }

    function handleOpenChange(nextOpen: boolean) {
        if (!nextOpen) {
            finishClose(0);
            return;
        }
        clearTimers();
        setSettling(false);
        setTargetIndex(null);
        setMenuModes([current, ...modes.filter((mode) => mode !== current)]);
        setItemsVisible(true);
        onOpenChange(true);
    }

    return {
        finishClose,
        handleOpenChange,
        itemsVisible,
        menuModes,
        reduceMotion,
        selectedMode,
        selectMode,
        settling,
        targetIndex,
    };
}
