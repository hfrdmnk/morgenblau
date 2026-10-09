import { Link, useLocation } from 'wouter';
import { useSearch } from 'wouter/use-browser-location';

import { AccountMenu } from '@/components/account-menu';
import { AddSourceDialog } from '@/components/add-source-dialog';
import {
    ArrowRightIcon,
    DigestIcon,
    LibraryIcon,
    SettingsIcon,
    SourcesIcon,
} from '@/components/icons';
import { useAppProfile } from '@/hooks/use-app-profile';
import { addCalendarDays, digestDateRange, parseDigestDate, toDateKey } from '@/lib/digest-dates';
import { APP_MODES, appMode, digestHref, isSettingsPath, PATHS } from '@/lib/paths';
import { cn } from '@/lib/utils';

const artwork = {
    [PATHS.digest]: {
        Icon: DigestIcon,
        color: 'text-atmosphere-blue',
        size: 'size-8',
        tabSize: 'size-5.5',
    },
    [PATHS.sources]: {
        Icon: SourcesIcon,
        color: 'text-sunrise-orange',
        size: 'h-7 w-8 -mt-0.5',
        tabSize: 'h-5 w-6 -translate-y-px',
    },
    [PATHS.library]: {
        Icon: LibraryIcon,
        color: 'text-aurora-violet',
        size: 'size-7',
        tabSize: 'size-5',
    },
};

const settingsTabs = [
    { label: 'General', href: PATHS.general },
    { label: 'Import', href: PATHS.import },
    { label: 'Export', href: PATHS.export },
];

const dayNumber = new Intl.DateTimeFormat(undefined, { day: '2-digit' });
const monthName = new Intl.DateTimeFormat(undefined, { month: 'short' });
const shortDate = new Intl.DateTimeFormat(undefined, {
    weekday: 'short',
    day: 'numeric',
    month: 'short',
});
const fullDate = new Intl.DateTimeFormat(undefined, {
    weekday: 'long',
    day: 'numeric',
    month: 'long',
    year: 'numeric',
});

export function AppHeader() {
    const [location] = useLocation();
    const profile = useAppProfile();
    const current = appMode(location);
    const mark = current
        ? artwork[current.href]
        : { Icon: SettingsIcon, color: 'text-muted-foreground', size: 'size-8' };

    return (
        <>
            <header className="app-header grid grid-cols-[1fr_auto] items-start gap-y-6 px-5 sm:px-8 md:grid-cols-3 md:px-14">
                <span className="flex min-h-10 items-center text-base text-muted-foreground md:hidden">
                    Morgenblau
                </span>
                <span
                    aria-label={current?.label ?? 'Settings'}
                    role="img"
                    className="relative hidden size-10 md:block"
                >
                    <mark.Icon className={cn('absolute left-1/2 top-1/2 -translate-x-1/2 -translate-y-1/2', mark.size, mark.color)} />
                </span>
                {location === PATHS.digest ? <DigestDateNav /> : null}
                {isSettingsPath(location) ? <SettingsNav location={location} /> : null}
                <div className="col-start-2 row-start-1 flex min-h-10 items-center justify-end gap-4 md:col-start-3">
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
            <nav aria-label="Modes" className="app-mode-nav text-sm">
                {APP_MODES.map((mode) => {
                    const active = current === mode;
                    const { Icon, color, tabSize } = artwork[mode.href];
                    return (
                        <Link
                            key={mode.href}
                            href={mode.href}
                            aria-current={active ? 'page' : undefined}
                            className={cn(
                                'app-mode-link',
                                active ? 'text-foreground' : 'text-muted-foreground hover:text-foreground',
                            )}
                        >
                            <span className="app-mode-artwork">
                                <Icon className={cn(tabSize, active && color)} />
                            </span>
                            {mode.label}
                        </Link>
                    );
                })}
            </nav>
        </>
    );
}

function SettingsNav({ location }: { location: string }) {
    const selected = location === PATHS.settings ? PATHS.general : location;
    return (
        <nav
            aria-label="Settings"
            className="col-span-2 row-start-2 flex items-center gap-2 text-sm md:col-span-1 md:col-start-2 md:row-start-1 md:justify-self-center md:gap-6"
        >
            {settingsTabs.map(({ label, href }) => (
                <Link
                    key={href}
                    href={href}
                    aria-current={selected === href ? 'page' : undefined}
                    className={cn(
                        'inline-flex min-h-11 items-center px-3.5 md:px-0',
                        selected === href ? 'text-foreground' : 'text-muted-foreground hover:text-foreground',
                    )}
                >
                    {label}
                </Link>
            ))}
        </nav>
    );
}

function DigestDateNav() {
    const search = useSearch();
    const now = new Date();
    const today = new Date(now.getFullYear(), now.getMonth(), now.getDate());
    const requestedDate = parseDigestDate(new URLSearchParams(search).get('date'));
    const selectedDate = requestedDate && requestedDate <= today ? requestedDate : today;
    const todayKey = toDateKey(today);
    const selectedKey = toDateKey(selectedDate);
    const dates = digestDateRange(selectedDate, today);
    const emptyDateSlots = 7 - dates.length;
    const isToday = selectedKey === todayKey;
    const nextDate = toDateKey(addCalendarDays(selectedDate, 1));

    return (
        <div className="col-span-2 row-start-2 md:col-span-1 md:col-start-2 md:row-start-1 md:justify-self-center">
            <nav
                aria-label="Digest day"
                className="grid min-h-11 grid-cols-[2.75rem_1fr_2.75rem] items-center gap-1 text-sm md:hidden"
            >
                <Link
                    aria-label="Previous day"
                    href={digestHref(toDateKey(addCalendarDays(selectedDate, -1)))}
                    className="flex size-11 items-center justify-center text-muted-foreground hover:text-foreground"
                >
                    <ArrowRightIcon className="size-4 rotate-180" />
                </Link>
                <span aria-label={fullDate.format(selectedDate)} className="text-center">
                    {shortDate.format(selectedDate)}
                </span>
                {isToday ? (
                    <button
                        aria-label="Next day"
                        disabled
                        type="button"
                        className="flex size-11 items-center justify-center text-subtle-foreground opacity-40"
                    >
                        <ArrowRightIcon className="size-4" />
                    </button>
                ) : (
                    <Link
                        aria-label="Next day"
                        href={digestHref(nextDate === todayKey ? undefined : nextDate)}
                        className="flex size-11 items-center justify-center text-muted-foreground hover:text-foreground"
                    >
                        <ArrowRightIcon className="size-4" />
                    </Link>
                )}
            </nav>
            <nav aria-label="Digest date" className="hidden grid-cols-7 items-start gap-3.5 md:inline-grid md:translate-y-1.5">
                {dates.map((date) => (
                    <DateOption date={date} key={toDateKey(date)} selectedKey={selectedKey} todayKey={todayKey} />
                ))}
                {Array.from({ length: emptyDateSlots }, (_, index) => (
                    <span aria-hidden="true" className="invisible w-5 text-center text-sm" key={index}>
                        <span className="block">00</span>
                        <span className="block">Mon</span>
                    </span>
                ))}
            </nav>
        </div>
    );
}

function DateOption({ date, selectedKey, todayKey }: {
    date: Date;
    selectedKey: string;
    todayKey: string;
}) {
    const key = toDateKey(date);
    const selected = key === selectedKey;
    return (
        <Link
            aria-current={selected ? 'date' : undefined}
            aria-label={fullDate.format(date)}
            className={cn('w-5 text-center text-sm hover:text-foreground', selected ? 'text-primary' : 'text-muted-foreground')}
            href={digestHref(key === todayKey ? undefined : key)}
        >
            <span className="block">{dayNumber.format(date)}</span>
            <span className={cn('block text-sm', { invisible: !selected })}>{monthName.format(date)}</span>
        </Link>
    );
}
