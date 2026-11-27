import { useRef, type ReactNode } from 'react';
import { useLocation } from 'wouter';
import { useSearch } from 'wouter/use-browser-location';

import { AccountMenu } from '@/components/account-menu';
import { AddSourceDialog } from '@/components/add-source-dialog';
import { FeedbackDialog } from '@/components/feedback-dialog';
import {
    ArrowRightIcon,
    DigestActiveIcon,
    DigestIcon,
    LibraryActiveIcon,
    LibraryIcon,
    MorgenblauMark,
    SourcesActiveIcon,
    SourcesIcon,
} from '@/components/icons';
import { Skeleton } from '@/components/ui/skeleton';
import {
    AppHeaderBar,
    DateStrip,
    DateStripDay,
    DateStripSpacer,
    DayStepper,
    ModeLink,
    ModeNav,
    PageTab,
} from '@/components/ui/app-chrome';
import { Avatar } from '@/components/ui/avatar';
import { Button } from '@/components/ui/button';
import { Link } from '@/components/ui/link';
import { Frame } from '@/design/frame';
import { Text } from '@/design/text';
import type { AppNavigationState } from '@/hooks/use-app-location';
import { useAppProfile } from '@/hooks/use-app-profile';
import { useHeightProperty } from '@/hooks/use-height-property';
import { addCalendarDays, digestDateRange, selectedDigestDate, startOfToday, toDateKey } from '@/lib/digest-dates';
import { libraryView } from '@/lib/library';
import { APP_MODES, appMode, digestHref, isSettingsPath, libraryHref, PATHS } from '@/lib/paths';

const artwork = {
    [PATHS.digest]: { Icon: DigestIcon, ActiveIcon: DigestActiveIcon },
    [PATHS.sources]: { Icon: SourcesIcon, ActiveIcon: SourcesActiveIcon },
    [PATHS.library]: { Icon: LibraryIcon, ActiveIcon: LibraryActiveIcon },
} as const;

const libraryViews = [
    { label: 'Recent', view: 'recent' },
    { label: 'Everything', view: 'everything' },
] as const;

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

const SAMPLE_PROFILE = { did: 'did:plc:sample', handle: 'reader.example', displayName: null, avatar: null };

export function AppHeader() {
    const [location] = useLocation();
    const profile = useAppProfile();
    const headerRef = useRef<HTMLElement>(null);
    const modeNavRef = useRef<HTMLElement>(null);
    useHeightProperty(headerRef, '--app-header-height');
    useHeightProperty(modeNavRef, '--app-mode-nav-height');
    const current = appMode(location);

    return (
        <>
            <AppHeaderBar ref={headerRef}>
                {current ? (
                    <Link aria-label="Morgenblau" href={PATHS.digest} variant="brand">
                        <MorgenblauMark size="2xl" />
                    </Link>
                ) : (
                    <SettingsReturnLink />
                )}
                {location === PATHS.digest ? <DigestDateNav /> : null}
                {isSettingsPath(location) ? <SettingsNav location={location} /> : null}
                {location === PATHS.library ? <LibraryNav /> : null}
                <Frame
                    align="center"
                    colStart={{ base: 2, md: 3 }}
                    direction="row"
                    gap="l"
                    justify="end"
                    minHeight="control"
                    rowStart={1}
                >
                    {profile.kind === 'ready' && profile.profile.feedback ? <FeedbackDialog /> : null}
                    <AddSourceDialog />
                    <Skeleton
                        data={profile.kind === 'ready' ? profile.profile : null}
                        error={
                            profile.kind === 'error' ? (
                                <Avatar aria-label="Profile unavailable" blank role="img" />
                            ) : undefined
                        }
                        label="Loading profile"
                        sample={SAMPLE_PROFILE}
                    >
                        {(ready) => <AccountMenu profile={ready} />}
                    </Skeleton>
                </Frame>
            </AppHeaderBar>
            <ModeNav aria-label="Modes" ref={modeNavRef}>
                {APP_MODES.map((mode) => {
                    const active = current === mode;
                    const { Icon, ActiveIcon } = artwork[mode.href];
                    return (
                        <ModeLink
                            aria-keyshortcuts={mode.shortcut}
                            artwork={active ? <ActiveIcon size="2xl" /> : <Icon size="2xl" />}
                            current={active}
                            href={mode.href}
                            key={mode.href}
                        >
                            {mode.label}
                        </ModeLink>
                    );
                })}
            </ModeNav>
        </>
    );
}

function SettingsReturnLink() {
    const state: AppNavigationState | null = window.history.state;
    const returnTo = state?.settingsReturn ?? { href: PATHS.digest, scrollY: 0 };
    const label = appMode(returnTo.href.split('?')[0])?.label ?? 'Digest';
    return (
        <Link href={returnTo.href} state={{ scrollY: returnTo.scrollY }} variant="back">
            <ArrowRightIcon rotate={180} size="md" />
            <Text as="span">Back to {label}</Text>
        </Link>
    );
}

function PageNav({ label, children }: { label: string; children: ReactNode }) {
    return (
        <Frame
            align="center"
            aria-label={label}
            as="nav"
            colSpan={{ base: 2, md: 1 }}
            colStart={{ md: 2 }}
            direction="row"
            gap="2xl"
            justifySelf={{ md: 'center' }}
            rowStart={{ base: 2, md: 1 }}
        >
            {children}
        </Frame>
    );
}

function SettingsNav({ location }: { location: string }) {
    const selected = location === PATHS.settings ? PATHS.general : location;
    return (
        <PageNav label="Settings">
            {settingsTabs.map(({ label, href }) => (
                <PageTab current={selected === href} href={href} key={href}>
                    {label}
                </PageTab>
            ))}
        </PageNav>
    );
}

function LibraryNav() {
    const selected = libraryView(useSearch());
    return (
        <PageNav label="Library">
            {libraryViews.map(({ label, view }) => (
                <PageTab current={selected === view} href={libraryHref(view)} key={view}>
                    {label}
                </PageTab>
            ))}
        </PageNav>
    );
}

function DigestDateNav() {
    const search = useSearch();
    const today = startOfToday();
    const selectedDate = selectedDigestDate(search, today);
    const todayKey = toDateKey(today);
    const selectedKey = toDateKey(selectedDate);
    const dates = digestDateRange(selectedDate, today);
    const emptyDateSlots = 7 - dates.length;
    const isToday = selectedKey === todayKey;
    const nextDate = toDateKey(addCalendarDays(selectedDate, 1));

    return (
        <Frame
            colSpan={{ base: 2, md: 1 }}
            colStart={{ md: 2 }}
            justifySelf={{ md: 'center' }}
            rowStart={{ base: 2, md: 1 }}
        >
            <DayStepper aria-label="Digest day">
                <Link
                    aria-label="Previous day"
                    href={digestHref(toDateKey(addCalendarDays(selectedDate, -1)))}
                    variant="step"
                >
                    <ArrowRightIcon rotate={180} size="md" />
                </Link>
                <Text align="center" aria-label={fullDate.format(selectedDate)} as="span">
                    {shortDate.format(selectedDate)}
                </Text>
                {isToday ? (
                    <Button aria-label="Next day" disabled size="icon-lg" variant="nav">
                        <ArrowRightIcon size="md" />
                    </Button>
                ) : (
                    <Link
                        aria-label="Next day"
                        href={digestHref(nextDate === todayKey ? undefined : nextDate)}
                        variant="step"
                    >
                        <ArrowRightIcon size="md" />
                    </Link>
                )}
            </DayStepper>
            <DateStrip aria-label="Digest date">
                {dates.map((date) => {
                    const key = toDateKey(date);
                    return (
                        <DateStripDay
                            aria-label={fullDate.format(date)}
                            day={dayNumber.format(date)}
                            href={digestHref(key === todayKey ? undefined : key)}
                            key={key}
                            month={monthName.format(date)}
                            selected={key === selectedKey}
                        />
                    );
                })}
                {Array.from({ length: emptyDateSlots }, (_, index) => (
                    <DateStripSpacer key={index} />
                ))}
            </DateStrip>
        </Frame>
    );
}
