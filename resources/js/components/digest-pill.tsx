import { Loading03Icon } from '@hugeicons/core-free-icons';
import { HugeiconsIcon } from '@hugeicons/react';
import { useEffect, useState } from 'react';

import type { DigestStatusState } from '@/hooks/use-digest-status';
import { cn } from '@/lib/utils';

type DigestPillProps = {
    state: DigestStatusState;
    onReady: () => void;
    onCaughtUpFade: () => void;
};

const CAUGHT_UP_HOLD_MS = 3000;

export function DigestPill({
    state,
    onReady,
    onCaughtUpFade,
}: DigestPillProps) {
    const [mounted, setMounted] = useState(false);
    const [leaving, setLeaving] = useState(false);

    useEffect(() => {
        const raf = requestAnimationFrame(() => setMounted(true));

        return () => cancelAnimationFrame(raf);
    }, []);

    useEffect(() => {
        if (state.phase !== 'caught_up') {
            return;
        }

        const hold = setTimeout(() => setLeaving(true), CAUGHT_UP_HOLD_MS);
        const finish = setTimeout(
            () => onCaughtUpFade(),
            CAUGHT_UP_HOLD_MS + 160,
        );

        return () => {
            clearTimeout(hold);
            clearTimeout(finish);
        };
    }, [state.phase, onCaughtUpFade]);

    if (state.phase === 'idle') {
        return null;
    }

    const isReady = state.phase === 'ready';

    const content = (
        <>
            {state.phase === 'fetching' ? (
                <HugeiconsIcon
                    icon={Loading03Icon}
                    className="size-4 shrink-0 animate-[spin_1.2s_linear_infinite] text-muted-foreground"
                    aria-hidden
                />
            ) : null}
            <span
                className={cn(
                    'text-sm',
                    state.phase === 'caught_up'
                        ? 'font-light text-muted-foreground'
                        : 'font-normal text-foreground',
                )}
            >
                {state.phase === 'fetching'
                    ? 'Fetching new content…'
                    : state.phase === 'ready'
                      ? 'New posts are ready'
                      : 'All caught up'}
            </span>
        </>
    );

    const baseClasses = cn(
        'pointer-events-auto inline-flex h-9 items-center gap-2 rounded-2xl border bg-card px-4',
        'border-gray-100 dark:border-gray-700',
        'transition-[opacity,transform] duration-[180ms] ease-[cubic-bezier(0.23,1,0.32,1)]',
        'motion-reduce:transition-opacity motion-reduce:duration-[120ms]',
        mounted && !leaving
            ? 'translate-y-0 scale-100 opacity-100'
            : '-translate-y-1 scale-[0.98] opacity-0 motion-reduce:translate-y-0 motion-reduce:scale-100',
    );

    const wrapper = (
        <div
            className="pointer-events-none fixed inset-x-0 top-6 z-30 flex justify-center"
            aria-live="polite"
        >
            {isReady ? (
                <button
                    type="button"
                    onClick={onReady}
                    className={cn(
                        baseClasses,
                        'cursor-pointer outline-none',
                        'hover:bg-gray-50 dark:hover:bg-gray-700',
                        'active:scale-[0.97]',
                        'focus-visible:outline-1 focus-visible:outline-offset-2 focus-visible:outline-ring focus-visible:outline-solid',
                    )}
                >
                    {content}
                </button>
            ) : (
                <div className={baseClasses} role="status">
                    {content}
                </div>
            )}
        </div>
    );

    return wrapper;
}
