import {
    Bookmark01Icon,
    LinkSquare01Icon,
    Loading03Icon,
    MagicWand01Icon,
    Share01Icon,
} from '@hugeicons/core-free-icons';
import { HugeiconsIcon } from '@hugeicons/react';
import type { IconSvgElement } from '@hugeicons/react';
import { useEffect, useState } from 'react';

import { cn, safeHref } from '@/lib/utils';

export type ExtractedToggleState = 'inactive' | 'active' | 'loading';

export type ExtractedToggle = {
    state: ExtractedToggleState;
    onClick: () => void;
};

type ReaderRailProps = {
    sourceUrl: string | null;
    extractedToggle?: ExtractedToggle;
    showProgress?: boolean;
};

const RAIL_BUTTON_BASE =
    'inline-flex size-9 items-center justify-center rounded-xl transition-colors duration-200 ease-out outline-none focus-visible:outline-1 focus-visible:outline-offset-2 focus-visible:outline-ring focus-visible:outline-solid';

export function ReaderRail({
    sourceUrl,
    extractedToggle,
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
                    />
                </div>
            </aside>
        </>
    );
}

function RailIcons({
    sourceUrl,
    extractedToggle,
}: {
    sourceUrl: string | null;
    extractedToggle?: ExtractedToggle;
}) {
    const safeSource = safeHref(sourceUrl);

    return (
        <>
            <DisabledRailIcon icon={Bookmark01Icon} label="Save" />
            <DisabledRailIcon icon={Share01Icon} label="Share" />
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
                    className={cn(
                        RAIL_BUTTON_BASE,
                        'text-muted-foreground hover:text-foreground',
                    )}
                >
                    <HugeiconsIcon
                        icon={LinkSquare01Icon}
                        className="size-[1.125rem]"
                    />
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
    const isLoading = state === 'loading';
    const isActive = state === 'active';

    return (
        <button
            type="button"
            onClick={onClick}
            disabled={isLoading}
            aria-pressed={isActive}
            aria-label={
                isActive ? 'Show feed version' : 'Show extracted version'
            }
            aria-busy={isLoading || undefined}
            className={cn(
                RAIL_BUTTON_BASE,
                'disabled:cursor-wait',
                isActive
                    ? 'text-primary hover:text-primary'
                    : 'text-muted-foreground hover:text-foreground',
            )}
        >
            <HugeiconsIcon
                icon={isLoading ? Loading03Icon : MagicWand01Icon}
                className={cn(
                    'size-[1.125rem]',
                    isLoading && 'motion-safe:animate-spin',
                )}
            />
        </button>
    );
}

function DisabledRailIcon({
    icon,
    label,
}: {
    icon: IconSvgElement;
    label: string;
}) {
    return (
        <button
            type="button"
            disabled
            aria-label={label}
            aria-disabled="true"
            className="inline-flex size-9 cursor-not-allowed items-center justify-center rounded-xl text-muted-foreground/40"
        >
            <HugeiconsIcon icon={icon} className="size-[1.125rem]" />
        </button>
    );
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
                        className="stroke-border"
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
            className="h-px w-full overflow-hidden bg-border"
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
