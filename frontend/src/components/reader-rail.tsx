import {
    BookmarkIcon,
    OpenIcon,
    SendIcon,
    SparkleIcon,
    SpinnerIcon,
} from '@proicons/react';
import { useEffect, useState } from 'react';

import type { SaveControl } from '@/hooks/use-save-toggle';
import type { ShareControl } from '@/hooks/use-share-toggle';
import { cn, safeHref } from '@/lib/utils';

export type ExtractedToggleState = 'inactive' | 'active' | 'loading';

type ExtractedToggle = {
    state: ExtractedToggleState;
    onClick: () => void;
};

type ReaderRailProps = {
    sourceUrl: string | null;
    extractedToggle?: ExtractedToggle;
    save?: SaveControl;
    share?: ShareControl;
    showProgress?: boolean;
};

const RAIL_BUTTON_BASE =
    'rail-icon-btn inline-flex size-9 items-center justify-center rounded-xl outline-none focus-visible:outline-1 focus-visible:outline-offset-2 focus-visible:outline-ring focus-visible:outline-solid';

export function ReaderRail({
    sourceUrl,
    extractedToggle,
    save,
    share,
    showProgress = true,
}: ReaderRailProps) {
    const progress = useScrollProgress(showProgress);

    return (
        <>
            <aside
                aria-label="Reader actions (sidebar)"
                className="pointer-events-none fixed top-1/2 right-4 z-10 hidden -translate-y-1/2 sm:right-6 sm:block"
            >
                <div className="pointer-events-auto flex flex-col items-center gap-3">
                    <RailIcons
                        sourceUrl={sourceUrl}
                        extractedToggle={extractedToggle}
                        save={save}
                        share={share}
                    />
                    {showProgress ? (
                        <ScrollProgressTrack
                            progress={progress}
                            orientation="vertical"
                        />
                    ) : null}
                </div>
            </aside>

            <aside
                aria-label="Reader actions (bottom bar)"
                className="fixed inset-x-0 bottom-0 z-10 border-t border-border bg-card/95 backdrop-blur sm:hidden"
            >
                {showProgress ? (
                    <ScrollProgressTrack
                        progress={progress}
                        orientation="horizontal"
                    />
                ) : null}
                <div className="flex items-center justify-around px-4 py-2">
                    <RailIcons
                        sourceUrl={sourceUrl}
                        extractedToggle={extractedToggle}
                        save={save}
                        share={share}
                    />
                </div>
            </aside>
        </>
    );
}

function RailIcons({
    sourceUrl,
    extractedToggle,
    save,
    share,
}: {
    sourceUrl: string | null;
    extractedToggle?: ExtractedToggle;
    save?: SaveControl;
    share?: ShareControl;
}) {
    const safeSource = safeHref(sourceUrl);

    return (
        <>
            {save ? <SaveRailButton {...save} /> : null}
            {share ? <ShareRailButton {...share} /> : null}
            {extractedToggle ? (
                <ExtractedToggleIcon
                    state={extractedToggle.state}
                    onClick={extractedToggle.onClick}
                />
            ) : null}
            {safeSource ? (
                <a
                    href={safeSource}
                    target="_blank"
                    rel="noopener noreferrer"
                    aria-label="Open original article"
                    className="inline-flex size-9 items-center justify-center rounded-xl text-muted-foreground transition-colors duration-200 ease-out outline-none hover:text-foreground focus-visible:outline-1 focus-visible:outline-offset-2 focus-visible:outline-ring focus-visible:outline-solid"
                >
                    <OpenIcon className="size-[1.125rem]" />
                </a>
            ) : null}
        </>
    );
}

function ExtractedToggleIcon({
    state,
    onClick,
}: {
    state: ExtractedToggleState;
    onClick: () => void;
}) {
    const { displayed, swapping } = useDeferredState(state);
    const isLoading = displayed === 'loading';
    const isActive = displayed === 'active';
    const Icon = isLoading ? SpinnerIcon : SparkleIcon;

    return (
        <button
            type="button"
            onClick={onClick}
            disabled={state === 'loading'}
            aria-pressed={isActive}
            aria-label={
                isActive ? 'Show feed version' : 'Show extracted version'
            }
            aria-busy={isLoading || undefined}
            data-swapping={swapping || undefined}
            className={cn(
                RAIL_BUTTON_BASE,
                'disabled:cursor-wait',
                isActive
                    ? 'text-primary hover:text-primary'
                    : 'text-muted-foreground hover:text-foreground',
            )}
        >
            <Icon
                className={cn(
                    'size-[1.125rem]',
                    isLoading && 'motion-safe:animate-spin',
                )}
            />
        </button>
    );
}

function SaveRailButton({ saved, busy, onToggle }: SaveControl) {
    const { displayed, swapping } = useDeferredState(saved);
    const label = saved ? 'Remove from saves' : 'Save for later';

    return (
        <button
            type="button"
            onClick={onToggle}
            disabled={busy}
            aria-pressed={displayed}
            aria-label={label}
            aria-busy={busy || undefined}
            data-swapping={swapping || undefined}
            data-saved={displayed || undefined}
            className={cn(
                RAIL_BUTTON_BASE,
                'disabled:cursor-wait',
                displayed
                    ? 'text-primary hover:text-primary'
                    : 'text-muted-foreground hover:text-foreground',
            )}
        >
            <BookmarkIcon className="size-[1.125rem]" />
        </button>
    );
}

function ShareRailButton({ shared, busy, onToggle }: ShareControl) {
    const { displayed, swapping } = useDeferredState(shared);
    const label = shared ? 'Unshare' : 'Share';

    return (
        <button
            type="button"
            onClick={onToggle}
            disabled={busy}
            aria-pressed={displayed}
            aria-label={label}
            aria-busy={busy || undefined}
            data-swapping={swapping || undefined}
            className={cn(
                RAIL_BUTTON_BASE,
                'disabled:cursor-wait',
                displayed
                    ? 'text-primary hover:text-primary'
                    : 'text-muted-foreground hover:text-foreground',
            )}
        >
            <SendIcon className="size-[1.125rem]" />
        </button>
    );
}

// Mirrors `target` with a swap-blur: on change the button blurs, `displayed` catches up ~150ms later under
// the blur, which lingers ~30ms past the swap to mask the icon change. Reduced-motion users update instantly.
function useDeferredState<T>(target: T): { displayed: T; swapping: boolean } {
    const [displayed, setDisplayed] = useState(target);
    const [settling, setSettling] = useState(false);

    const reducedMotion =
        typeof window === 'undefined' ||
        window.matchMedia('(prefers-reduced-motion: reduce)').matches;

    // Reduced motion: mirror instantly during render, no blur stage.
    if (reducedMotion && !Object.is(displayed, target)) {
        setDisplayed(target);
    }

    // Hold the swap so the blur can build, then open a brief settling window.
    useEffect(() => {
        if (reducedMotion || Object.is(displayed, target)) return;
        const id = window.setTimeout(() => {
            setDisplayed(target);
            setSettling(true);
        }, 150);
        return () => window.clearTimeout(id);
    }, [target, displayed, reducedMotion]);

    // Trail the un-blur ~30ms past the icon swap.
    useEffect(() => {
        if (!settling) return;
        const id = window.setTimeout(() => setSettling(false), 30);
        return () => window.clearTimeout(id);
    }, [settling]);

    const swapping =
        !reducedMotion && (settling || !Object.is(displayed, target));
    return { displayed, swapping };
}

const RING_RADIUS = 8.25;
const RING_CIRCUMFERENCE = 2 * Math.PI * RING_RADIUS;

function ScrollProgressTrack({
    progress,
    orientation,
}: {
    progress: number;
    orientation: 'vertical' | 'horizontal';
}) {
    const percent = Math.round(progress * 100);

    if (orientation === 'vertical') {
        return (
            <div
                role="progressbar"
                aria-label="Reading progress"
                aria-valuemin={0}
                aria-valuemax={100}
                aria-valuenow={percent}
                className="mt-2 size-[1.125rem]"
            >
                <svg
                    viewBox="0 0 18 18"
                    className="size-full -rotate-90"
                    aria-hidden="true"
                >
                    <circle
                        cx="9"
                        cy="9"
                        r={RING_RADIUS}
                        fill="none"
                        strokeWidth="1.5"
                        className="stroke-gray-200 dark:stroke-gray-700"
                    />
                    <circle
                        cx="9"
                        cy="9"
                        r={RING_RADIUS}
                        fill="none"
                        strokeWidth="1.5"
                        strokeLinecap="round"
                        strokeDasharray={RING_CIRCUMFERENCE}
                        strokeDashoffset={RING_CIRCUMFERENCE * (1 - progress)}
                        className="stroke-primary motion-safe:[transition:stroke-dashoffset_120ms_linear]"
                    />
                </svg>
            </div>
        );
    }

    return (
        <div
            role="progressbar"
            aria-label="Reading progress"
            aria-valuemin={0}
            aria-valuemax={100}
            aria-valuenow={percent}
            className="h-px w-full overflow-hidden bg-overlay-2"
        >
            <div
                className="h-full w-full origin-left bg-foreground motion-safe:transition-transform motion-safe:duration-[120ms] motion-safe:ease-linear"
                style={{ transform: `scaleX(${progress})` }}
            />
        </div>
    );
}

function useScrollProgress(enabled: boolean): number {
    const [progress, setProgress] = useState(0);

    useEffect(() => {
        if (!enabled) {
            return;
        }

        const compute = () => {
            const max =
                document.documentElement.scrollHeight - window.innerHeight;

            if (max <= 0) {
                setProgress(1);

                return;
            }

            setProgress(Math.max(0, Math.min(1, window.scrollY / max)));
        };

        compute();
        window.addEventListener('scroll', compute, { passive: true });
        window.addEventListener('resize', compute);

        const observer = new ResizeObserver(compute);
        observer.observe(document.documentElement);

        return () => {
            window.removeEventListener('scroll', compute);
            window.removeEventListener('resize', compute);
            observer.disconnect();
        };
    }, [enabled]);

    return progress;
}
