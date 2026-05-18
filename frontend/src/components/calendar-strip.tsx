import { ArrowRight02Icon } from '@hugeicons/core-free-icons';
import { HugeiconsIcon } from '@hugeicons/react';
import type { CSSProperties } from 'react';
import { useLayoutEffect, useRef, useState } from 'react';

import {
    addDays,
    formatISODate,
    isSameDay,
    startOfLocalDay,
} from '@/lib/date';
import { cn } from '@/lib/utils';

const SLOT_WIDTH = 36;
const MIN_SLOTS = 5;
const FADE_PER_STEP = 0.13;
const FADE_MIN = 0.15;

type Props = {
    selected: Date;
    today: Date;
    onSelect: (date: Date) => void;
};

type StartViewTransition = (cb: () => void) => { finished: Promise<void> };

function startStripTransition(apply: () => void): void {
    if (typeof document === 'undefined' || !('startViewTransition' in document)) {
        apply();
        return;
    }
    const html = document.documentElement;
    html.dataset.stripTransition = 'true';
    const start = (
        document as Document & { startViewTransition: StartViewTransition }
    ).startViewTransition;
    const tx = start.call(document, apply);
    tx.finished.finally(() => {
        delete html.dataset.stripTransition;
    });
}

export function CalendarStrip({ selected, today, onSelect }: Props) {
    const todayStart = startOfLocalDay(today);
    const daysAreaRef = useRef<HTMLDivElement | null>(null);
    const [slotCount, setSlotCount] = useState(11);

    useLayoutEffect(() => {
        const el = daysAreaRef.current;
        if (!el) return;
        const update = () => {
            const w = el.clientWidth;
            if (w === 0) return;
            let count = Math.floor(w / SLOT_WIDTH);
            if (count % 2 === 0) count -= 1;
            if (count < MIN_SLOTS) count = MIN_SLOTS;
            setSlotCount(count);
        };
        update();
        const observer = new ResizeObserver(update);
        observer.observe(el);
        return () => observer.disconnect();
    }, []);

    const centerIndex = Math.floor(slotCount / 2);
    const selectedIsToday = isSameDay(selected, today);

    const transitionTo = (target: Date) => {
        if (isSameDay(target, selected)) return;
        if (startOfLocalDay(target) > todayStart) return;
        startStripTransition(() => onSelect(target));
    };

    return (
        <div className="mx-auto w-full max-w-2xl px-4 pt-10 pb-12 sm:px-6">
            <div className="group/strip flex items-baseline gap-3">
                <div
                    ref={daysAreaRef}
                    className="flex flex-1 items-baseline justify-center"
                >
                    {Array.from({ length: slotCount }, (_, i) => {
                        const offset = i - centerIndex;
                        const date = addDays(selected, offset);
                        const isFuture =
                            startOfLocalDay(date).getTime() >
                            todayStart.getTime();

                        if (isFuture) {
                            return <EmptySlot key={`empty-${i}`} />;
                        }

                        const iso = formatISODate(date);
                        const cellStyle: CSSProperties = {
                            viewTransitionName: `day-${iso}`,
                        };

                        if (i === centerIndex) {
                            return (
                                <SelectedSlot
                                    key={iso}
                                    date={date}
                                    style={cellStyle}
                                />
                            );
                        }

                        const fadeOpacity = Math.max(
                            FADE_MIN,
                            1 - FADE_PER_STEP * Math.abs(offset),
                        );

                        return (
                            <FadedSlot
                                key={iso}
                                date={date}
                                fadeOpacity={fadeOpacity}
                                onClick={() => transitionTo(date)}
                                style={cellStyle}
                            />
                        );
                    })}
                </div>
                <TodayAnchor
                    hidden={selectedIsToday}
                    onClick={() => transitionTo(today)}
                />
            </div>
        </div>
    );
}

function EmptySlot() {
    return <div aria-hidden className="w-9 shrink-0" />;
}

function SelectedSlot({
    date,
    style,
}: {
    date: Date;
    style: CSSProperties;
}) {
    const month = date.toLocaleDateString(undefined, { month: 'short' });
    const dayLabel = date.toLocaleDateString(undefined, {
        weekday: 'long',
        month: 'long',
        day: 'numeric',
    });

    return (
        <div className="flex w-9 shrink-0 flex-col items-center" style={style}>
            <span
                aria-label={dayLabel}
                aria-current="date"
                className={cn(
                    'text-lg leading-none font-medium tracking-tight text-foreground'
                )}
            >
                {pad(date.getDate())}
            </span>
            <span className="mt-1 text-xs font-light text-muted-foreground">
                {month}
            </span>
        </div>
    );
}

function FadedSlot({
    date,
    fadeOpacity,
    onClick,
    style,
}: {
    date: Date;
    fadeOpacity: number;
    onClick: () => void;
    style: CSSProperties;
}) {
    const dayLabel = date.toLocaleDateString(undefined, {
        weekday: 'long',
        month: 'long',
        day: 'numeric',
    });

    return (
        <button
            type="button"
            aria-label={dayLabel}
            onClick={onClick}
            className={cn(
                'flex w-9 shrink-0 cursor-pointer flex-col items-center rounded-sm',
                'text-sm font-normal text-muted-foreground',
                'opacity-(--day-opacity) transition-opacity duration-200 ease-out',
                'group-hover/strip:opacity-100',
                'focus-visible:outline-1 focus-visible:outline-offset-2 focus-visible:outline-ring',
            )}
            style={
                {
                    ...style,
                    '--day-opacity': fadeOpacity,
                } as CSSProperties
            }
        >
            <span className="leading-none">{pad(date.getDate())}</span>
            <span aria-hidden className="invisible mt-1 text-xs">
                ·
            </span>
        </button>
    );
}

function TodayAnchor({
    hidden,
    onClick,
}: {
    hidden: boolean;
    onClick: () => void;
}) {
    return (
        <button
            type="button"
            aria-label="Go to today"
            onClick={onClick}
            aria-hidden={hidden}
            tabIndex={hidden ? -1 : 0}
            className={cn(
                'inline-flex shrink-0 cursor-pointer items-center gap-1 rounded-sm text-sm font-normal text-muted-foreground opacity-50',
                'transition duration-200 ease-out',
                'hover:text-atmosphere-blue group-hover/strip:opacity-100',
                'focus-visible:outline-1 focus-visible:outline-offset-2 focus-visible:outline-ring',
                hidden && 'pointer-events-none opacity-0!',
            )}
        >
            <HugeiconsIcon icon={ArrowRight02Icon} className="size-3.5" />
            today
        </button>
    );
}

function pad(n: number): string {
    return String(n).padStart(2, '0');
}
