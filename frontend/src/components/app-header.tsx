import { AnimatePresence, motion } from 'motion/react';
import type { ComponentType, SVGProps } from 'react';
import { useState } from 'react';
import { Link as RouterLink, useLocation } from 'wouter';
import { useSearch } from 'wouter/use-browser-location';

import { AccountMenu } from '@/components/account-menu';
import { AddSourceDialog } from '@/components/add-source-dialog';
import {
    ChevronDownIcon,
    DigestIcon,
    LibraryIcon,
    SourcesIcon,
} from '@/components/icons';
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover';
import { useAppProfile } from '@/hooks/use-app-profile';
import { useModeSelect } from '@/hooks/use-mode-select';
import { digestDateRange, parseDigestDate, toDateKey } from '@/lib/digest-dates';
import { digestHref, PATHS } from '@/lib/paths';
import { cn } from '@/lib/utils';

type Mode = {
    label: string;
    href: string;
    color: string;
    iconSize: string;
    Icon: ComponentType<SVGProps<SVGSVGElement>>;
};

const modes: Mode[] = [
    {
        label: 'Digest',
        href: PATHS.digest,
        color: 'text-atmosphere-blue',
        iconSize: 'size-8',
        Icon: DigestIcon,
    },
    {
        label: 'Sources',
        href: PATHS.sources,
        color: 'text-sunrise-orange',
        iconSize: 'h-8 w-[2.3125rem]',
        Icon: SourcesIcon,
    },
    {
        label: 'Library',
        href: PATHS.library,
        color: 'text-aurora-violet',
        iconSize: 'size-7',
        Icon: LibraryIcon,
    },
];

const dayNumber = new Intl.DateTimeFormat(undefined, { day: '2-digit' });
const monthName = new Intl.DateTimeFormat(undefined, { month: 'short' });
const fullDate = new Intl.DateTimeFormat(undefined, {
    weekday: 'long',
    day: 'numeric',
    month: 'long',
    year: 'numeric',
});

const layoutTransition = { type: 'spring', duration: 0.3, bounce: 0 } as const;

export function AppHeader() {
    const [location] = useLocation();
    const [modeOpen, setModeOpen] = useState(false);
    const profile = useAppProfile();

    return (
        <header className="grid grid-cols-[1fr_auto] items-start gap-y-6 px-5 pt-6 sm:px-8 sm:pt-8 md:grid-cols-3 md:px-14 md:pt-10">
            <ModeSelect location={location} onOpenChange={setModeOpen} open={modeOpen} />
            {location === PATHS.digest ? (
                <DigestDateNav />
            ) : (
                <span className="hidden md:block" />
            )}
            <div className="col-start-2 row-start-1 flex items-center justify-end gap-4 md:col-start-3">
                <AddSourceDialog />
                {profile.kind === 'ready' ? (
                    <AccountMenu profile={profile.profile} />
                ) : (
                    <span
                        aria-label={profile.kind === 'loading' ? 'Loading profile' : 'Profile unavailable'}
                        className={cn('size-8 rounded-full bg-muted', {
                            'animate-pulse': profile.kind === 'loading',
                        })}
                    />
                )}
            </div>
        </header>
    );
}

function ModeSelect({
    location,
    onOpenChange,
    open,
}: {
    location: string;
    onOpenChange: (open: boolean) => void;
    open: boolean;
}) {
    const current = currentMode(location);
    const outsideMode = current.href === PATHS.digest && location !== PATHS.digest;
    const state = useModeSelect(current, modes, onOpenChange);
    const {
        finishClose,
        handleOpenChange,
        itemsVisible,
        menuModes,
        reduceMotion,
        selectedMode,
        selectMode,
        settling,
        targetIndex,
    } = state;

    return (
        <Popover onOpenChange={handleOpenChange} open={open}>
            <ModeSelectTrigger
                current={current}
                open={open}
                reduceMotion={reduceMotion}
                selecting={targetIndex !== null || (open && outsideMode)}
            />
            <ModeMenu
                finishClose={finishClose}
                itemsVisible={itemsVisible}
                menuModes={menuModes}
                reduceMotion={reduceMotion}
                selectedMode={menuSelection(outsideMode, targetIndex, selectedMode)}
                selectMode={selectMode}
                settling={settling}
                targetIndex={targetIndex}
            />
        </Popover>
    );
}

function menuSelection(outsideMode: boolean, targetIndex: number | null, selectedMode: Mode): Mode | null {
    return outsideMode && targetIndex === null ? null : selectedMode;
}

function ModeSelectTrigger({
    current,
    open,
    reduceMotion,
    selecting,
}: {
    current: Mode;
    open: boolean;
    reduceMotion: boolean;
    selecting: boolean;
}) {
    return (
        <PopoverTrigger
            render={
                <button
                    aria-label={`${current.label}: choose mode`}
                    className="group inline-flex w-fit items-center gap-3 rounded-md text-muted-foreground"
                    type="button"
                />
            }
        >
            <ModeIcon current={current} reduceMotion={reduceMotion} />
            <ModeLabel
                current={current}
                open={open}
                reduceMotion={reduceMotion}
                selecting={selecting}
            />
        </PopoverTrigger>
    );
}

function ModeIcon({ current, reduceMotion }: { current: Mode; reduceMotion: boolean }) {
    const blur = reducedValue(reduceMotion, 'blur(4px)', 'blur(0px)');
    return (
        <span className="relative size-8 shrink-0">
            <AnimatePresence initial={false} mode="popLayout">
                <motion.span
                    animate={{ opacity: 1, filter: 'blur(0px)' }}
                    className="absolute inset-0 flex items-center justify-center"
                    exit={{ opacity: 0, filter: blur }}
                    initial={{ opacity: 0, filter: blur }}
                    key={current.label}
                    transition={{
                        duration: reducedValue(reduceMotion, 0.16, 0.1),
                        ease: 'easeOut',
                    }}
                >
                    <current.Icon className={cn(current.iconSize, current.color)} />
                </motion.span>
            </AnimatePresence>
        </span>
    );
}

function ModeLabel({
    current,
    open,
    reduceMotion,
    selecting,
}: {
    current: Mode;
    open: boolean;
    reduceMotion: boolean;
    selecting: boolean;
}) {
    const layout = reducedValue<'position' | false>(reduceMotion, 'position', false);
    return (
        <motion.span
            className="inline-flex items-center gap-1.5"
            layout={layout}
            transition={layoutTransition}
        >
            <span className={cn('text-sm leading-none', { invisible: selecting })}>
                {current.label}
            </span>
            <motion.span
                animate={{ rotate: chevronRotation(open, selecting) }}
                className="inline-flex"
                layout={layout}
                transition={{
                    layout: layoutTransition,
                    rotate: {
                        duration: reducedValue(reduceMotion, 0.16, 0),
                        ease: 'easeOut',
                    },
                }}
            >
                <ChevronDownIcon className="size-4 text-subtle-foreground" />
            </motion.span>
        </motion.span>
    );
}

function ModeMenu({
    finishClose,
    itemsVisible,
    menuModes,
    reduceMotion,
    selectedMode,
    selectMode,
    settling,
    targetIndex,
}: {
    finishClose: (delay: number) => void;
    itemsVisible: boolean;
    menuModes: Mode[];
    reduceMotion: boolean;
    selectedMode: Mode | null;
    selectMode: (index: number) => void;
    settling: boolean;
    targetIndex: number | null;
}) {
    return (
        <PopoverContent
            align="start"
            alignOffset={44}
            className="w-auto gap-0 overflow-visible rounded-none p-0 text-sm"
            sideOffset={8}
            style={{
                background: 'transparent',
                boxShadow: 'none',
                transform: 'none',
                transition: 'none',
            }}
        >
            <motion.nav
                animate={{
                    transform: `translateY(${-31 - (targetIndex ?? 0) * 26}px)`,
                }}
                aria-label="Product mode"
                className={cn('flex flex-col items-start gap-3', {
                    'pointer-events-none': targetIndex !== null,
                })}
                initial={false}
                transition={{
                    duration: reduceMotion ? 0 : 0.22,
                    ease: [0.645, 0.045, 0.355, 1],
                }}
            >
                {menuModes.map((mode, index) => (
                    <ModeOptionRow
                        finishClose={finishClose}
                        index={index}
                        itemsVisible={itemsVisible}
                        key={mode.label}
                        mode={mode}
                        selected={mode === selectedMode}
                        selectMode={selectMode}
                        selecting={targetIndex !== null}
                        settling={settling}
                    />
                ))}
            </motion.nav>
        </PopoverContent>
    );
}

function ModeOptionRow({
    finishClose,
    index,
    itemsVisible,
    mode,
    selected,
    selectMode,
    selecting,
    settling,
}: {
    finishClose: (delay: number) => void;
    index: number;
    itemsVisible: boolean;
    mode: Mode;
    selected: boolean;
    selectMode: (index: number) => void;
    selecting: boolean;
    settling: boolean;
}) {
    const visible = modeOptionVisible(selecting, settling, selected, itemsVisible);
    return (
        <motion.span
            animate={{ opacity: opacityFor(visible) }}
            className="h-3.5 leading-none"
            initial={{ opacity: 0 }}
            transition={{
                opacity: {
                    duration: reducedValue(settling, 0.14, 0),
                    delay: modeOptionDelay(visible, settling, index),
                    ease: 'easeOut',
                },
            }}
        >
            <ModeOption
                finishClose={finishClose}
                index={index}
                mode={mode}
                selected={selected}
                selectMode={selectMode}
            />
        </motion.span>
    );
}

function ModeOption({
    finishClose,
    index,
    mode,
    selected,
    selectMode,
}: {
    finishClose: (delay: number) => void;
    index: number;
    mode: Mode;
    selected: boolean;
    selectMode: (index: number) => void;
}) {
    if (selected) {
        return (
            <button
                aria-label="Close mode menu"
                className="text-muted-foreground"
                onClick={() => finishClose(0)}
                tabIndex={-1}
                type="button"
            >
                {mode.label}
            </button>
        );
    }

    return (
        <RouterLink
            className="rounded-sm text-subtle-foreground transition-colors duration-(--motion-duration-fast) hover:text-muted-foreground"
            href={mode.href}
            onClick={() => selectMode(index)}
        >
            {mode.label}
        </RouterLink>
    );
}

function reducedValue<T>(reduced: boolean, standard: T, fallback: T): T {
    return reduced ? fallback : standard;
}

function chevronRotation(open: boolean, selecting: boolean): number {
    return open && !selecting ? 180 : 0;
}

function modeOptionVisible(
    selecting: boolean,
    settling: boolean,
    selected: boolean,
    itemsVisible: boolean,
): boolean {
    return selecting ? settling || selected : !selected && itemsVisible;
}

function opacityFor(visible: boolean): number {
    return visible ? 1 : 0;
}

function modeOptionDelay(visible: boolean, settling: boolean, index: number): number {
    return visible && !settling ? Math.max(0, index - 1) * 0.045 : 0;
}

function currentMode(location: string): Mode {
    if (location.startsWith(PATHS.sources)) return modes[1];
    if (location.startsWith(PATHS.library)) return modes[2];
    return modes[0];
}

function DigestDateNav() {
    const search = useSearch();
    const today = startOfToday();
    const requestedDate = parseDigestDate(new URLSearchParams(search).get('date'));
    const selectedDate = requestedDate && requestedDate <= today ? requestedDate : today;
    const todayKey = toDateKey(today);
    const selectedKey = toDateKey(selectedDate);
    const dates = digestDateRange(selectedDate, today);
    const emptyDateSlots = 7 - dates.length;

    return (
        <nav
            aria-label="Digest date"
            className="col-span-2 row-start-2 inline-grid translate-y-0 grid-cols-7 items-start justify-self-center gap-2.5 sm:gap-3.5 md:col-span-1 md:col-start-2 md:row-start-1 md:translate-y-1.5"
        >
            {dates.map((date) => (
                <DateOption
                    date={date}
                    key={toDateKey(date)}
                    selectedKey={selectedKey}
                    todayKey={todayKey}
                />
            ))}
            {Array.from({ length: emptyDateSlots }, (_, index) => (
                <span
                    aria-hidden="true"
                    className="invisible w-5 text-center text-sm"
                    key={index}
                >
                    <span className="block">00</span>
                    <span className="block">Mon</span>
                </span>
            ))}
        </nav>
    );
}

function DateOption({
    date,
    selectedKey,
    todayKey,
}: {
    date: Date;
    selectedKey: string;
    todayKey: string;
}) {
    const key = toDateKey(date);
    const selected = key === selectedKey;

    return (
        <RouterLink
            aria-current={selected ? 'date' : undefined}
            aria-label={fullDate.format(date)}
            className={cn(
                'w-5 rounded-sm text-center text-sm transition-colors duration-(--motion-duration-fast) hover:text-foreground',
                {
                    'text-primary': selected,
                    'text-subtle-foreground': !selected,
                },
            )}
            href={digestHref(key === todayKey ? undefined : key)}
        >
            <span className="block">{dayNumber.format(date)}</span>
            <span className={cn('block text-sm', { invisible: !selected })}>
                {monthName.format(date)}
            </span>
        </RouterLink>
    );
}

function startOfToday(): Date {
    const now = new Date();
    return new Date(now.getFullYear(), now.getMonth(), now.getDate());
}
