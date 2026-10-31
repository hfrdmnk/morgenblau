import { Drawer } from '@base-ui/react/drawer';
import { Switch } from '@base-ui/react/switch';
import { motion, useReducedMotion } from 'motion/react';
import {
    useEffect,
    useId,
    useMemo,
    useRef,
    useState,
    type ComponentProps,
    type FormEvent,
    type ReactElement,
    type ReactNode,
    type RefObject,
} from 'react';
import { toast } from 'sonner';
import { useLocation } from 'wouter';

import { GenerateFeed } from '@/components/generate-feed';
import { CheckIcon, ChevronDownIcon, CloseIcon, EnterIcon, ExternalLinkIcon, LoadingIcon, MoreIcon, PenIcon, PlusIcon } from '@/components/icons';
import { NewsletterAddress } from '@/components/newsletter-address';
import { PersonAvatar } from '@/components/person-avatar';
import { RemoveDialog } from '@/components/source-menu';
import { TagChips, TagInput } from '@/components/tag-input';
import { Button } from '@/components/ui/button';
import { DrawerBackdrop, DrawerPopup, DrawerViewport } from '@/components/ui/drawer';
import { InputGroup, InputGroupButton, InputGroupInput } from '@/components/ui/input-group';
import { Link } from '@/components/ui/link';
import { useMediaQuery } from '@/hooks/use-media-query';
import { useReaderFont } from '@/hooks/use-reader-font';
import type { FeedSource } from '@/hooks/use-sources';
import {
    addSource,
    discoveryError,
    editSource,
    findSite,
    previewArticle,
    previewSource,
    profileSummary,
    sourceKey,
    sourceLocation,
    sourceTitle,
    unsubscribe,
    youtubeFeeds,
    type PreviewPost,
    type SourceChange,
    type SourceOptions,
    type SiteProfile,
    type SiteResult,
    type SourceCandidate,
} from '@/lib/add-source';
import { classifyMutationError, describeMutationError } from '@/lib/api';
import { PATHS } from '@/lib/paths';
import { hasNewReaderContent, httpURL, readerBody } from '@/lib/reader';
import { announceSourceAdded } from '@/lib/source-toast';
import { displayURL } from '@/lib/source-display';
import { addTags, canAddTags, feedEditable, GONE, hasOverlongTag, isGone, MAX_TITLE_LENGTH, titleTooLong } from '@/lib/source-edit';
import { cn } from '@/lib/utils';
import '../pages/reader.css';

/* Sheet storyboard: every level of detail is its own screen inside one sheet.
 *   0ms  sheet slides in from its edge over --motion-duration-overlay (ui/drawer.tsx)
 *  push  the next screen enters 12px from the right over 200ms; the previous one leaves at once
 *   pop  the previous screen returns 12px from the left over 200ms
 *        keyboard navigation and reduced motion swap screens instantly
 */
const TIMING = { screen: 0.2 };
const SCREEN = {
    push: 'translateX(12px)',
    pop: 'translateX(-12px)',
    rest: 'translateX(0px)',
    ease: [0.16, 1, 0.3, 1] as const,
};

// rkey is the subscription a feed screen was opened with; site is absent when it was opened from a profile's follows.
type Screen =
    | { kind: 'feed'; candidate: SourceCandidate; site?: SiteResult; rkey?: string }
    | { kind: 'edit'; source: FeedSource; onChanged: () => void; onRemoved: () => void; removedFocus?: RefObject<HTMLElement | null> }
    | { kind: 'post'; candidate: SourceCandidate; post: PreviewPost }
    | { kind: 'profile'; site: SiteResult; profile: SiteProfile };

type Discovery =
    | { stage: 'idle' }
    | { stage: 'loading' }
    | { stage: 'found'; site: SiteResult }
    | { stage: 'error'; error: string };

type Sheet = {
    instant: boolean;
    saving: boolean;
    subscriptions: Map<string, string | null>;
    push: (screen: Screen) => void;
    pop: () => void;
    close: () => void;
    onSavingChange: (saving: boolean) => void;
    record: (key: string, rkey: string | null) => void;
    announce: (rkey: string) => void;
};

export function AddSourceDialog({ trigger }: { trigger?: ReactElement }) {
    const inputRef = useRef<HTMLInputElement>(null);
    const [open, setOpen] = useState(false);
    const [saving, setSaving] = useState(false);
    return (
        <SheetDrawer
            initialFocus={(interaction) => (interaction === 'touch' ? true : inputRef.current)}
            onOpenChange={setOpen}
            open={open}
            saving={saving}
            trigger={trigger ?? (
                <Button aria-label="Add a source" className="text-subtle-foreground" size="icon-sm" variant="ghost">
                    <PlusIcon className="size-5" />
                </Button>
            )}
        >
            <SheetBody inputRef={inputRef} onClose={() => setOpen(false)} onSavingChange={setSaving} saving={saving} />
        </SheetDrawer>
    );
}

// The same sheet as Add a source, opened on one of the reader's feeds to rename, retag or re-point it.
export function EditSourceSheet({ source, onChanged, onRemoved = onChanged, removedFocus }: {
    source: FeedSource;
    onChanged: () => void;
    onRemoved?: () => void;
    removedFocus?: RefObject<HTMLElement | null>;
}) {
    const [open, setOpen] = useState(false);
    const [saving, setSaving] = useState(false);
    return (
        <SheetDrawer
            onOpenChange={setOpen}
            open={open}
            saving={saving}
            trigger={(
                <Button aria-label={`Edit ${feedSourceName(source)}`} className="text-subtle-foreground hover:text-foreground" size="icon-sm" variant="ghost">
                    <MoreIcon className="size-5" />
                </Button>
            )}
        >
            <SheetBody
                onClose={() => setOpen(false)}
                onSavingChange={setSaving}
                root={{ kind: 'edit', source, onChanged, onRemoved, removedFocus }}
                saving={saving}
            />
        </SheetDrawer>
    );
}

function SheetDrawer({ open, saving, trigger, initialFocus, onOpenChange, children }: {
    open: boolean;
    saving: boolean;
    trigger: ReactElement;
    initialFocus?: ComponentProps<typeof DrawerPopup>['initialFocus'];
    onOpenChange: (open: boolean) => void;
    children: ReactNode;
}) {
    const side = useMediaQuery('(min-width: 40rem)') ? 'right' : 'bottom';
    return (
        <Drawer.Root
            open={open}
            onOpenChange={(next, details) => {
                if (saving) details.cancel();
                else onOpenChange(next);
            }}
            swipeDirection={side === 'right' ? 'right' : 'down'}
        >
            <Drawer.Trigger render={trigger} />
            <Drawer.Portal>
                <DrawerBackdrop />
                <DrawerViewport side={side}>
                    <DrawerPopup initialFocus={initialFocus} side={side}>
                        {side === 'bottom' && (
                            <div aria-hidden="true" className="mx-auto mt-2 h-1 w-10 shrink-0 rounded-full bg-accent" />
                        )}
                        {children}
                    </DrawerPopup>
                </DrawerViewport>
            </Drawer.Portal>
        </Drawer.Root>
    );
}

function feedSourceName(source: FeedSource) {
    return source.title || displayURL(source.siteUrl || source.feedUrl);
}

// A root screen replaces the search screen (which only the add sheet, with its inputRef, shows), so the sheet opens on it and Back never leaves it.
function SheetBody({
    inputRef,
    root,
    saving,
    onSavingChange,
    onClose,
}: {
    inputRef?: RefObject<HTMLInputElement | null>;
    root?: Screen;
    saving: boolean;
    onSavingChange: (saving: boolean) => void;
    onClose: () => void;
}) {
    const [, navigate] = useLocation();
    const [url, setUrl] = useState('');
    const [discovery, setDiscovery] = useState<Discovery>({ stage: 'idle' });
    const { stack, canGoBack, push, pop } = useScreenStack(root);
    const { nameOf, formOf, rename } = useScreenNames();
    const [keyboard, setKeyboard] = useState(false);
    // Sources added (rkey) or removed (null) in this sheet, overriding what earlier lookups reported.
    const [subscriptions, setSubscriptions] = useState<Map<string, string | null>>(new Map());
    const reduced = useReducedMotion();
    const request = useRef<AbortController | null>(null);
    useEffect(() => () => request.current?.abort(), []);
    const top = stack.at(-1);
    const screen = top?.screen;

    const sheet: Sheet = {
        instant: Boolean(reduced) || keyboard,
        saving,
        subscriptions,
        push,
        pop,
        close: onClose,
        onSavingChange,
        record: (key, rkey) => setSubscriptions((previous) => new Map(previous).set(key, rkey)),
        announce: (rkey) =>
            announceSourceAdded(rkey, (to) => {
                onClose();
                navigate(to);
            }),
    };

    async function find(event: FormEvent) {
        event.preventDefault();
        if (saving || discovery.stage === 'loading') return;
        request.current?.abort();
        const controller = new AbortController();
        request.current = controller;
        setDiscovery({ stage: 'loading' });
        const next = await findSite(url, controller.signal).then(
            (site): Discovery => ({ stage: 'found', site }),
            (error): Discovery => ({ stage: 'error', error: discoveryError(error) }),
        );
        if (!controller.signal.aborted) setDiscovery(next);
    }

    function reset() {
        request.current?.abort();
        setDiscovery({ stage: 'idle' });
    }

    return (
        <div
            className="flex min-h-0 flex-1 flex-col"
            onKeyDown={(event) => {
                if (event.key === 'Enter' || event.key === ' ') setKeyboard(true);
            }}
            onPointerDown={() => setKeyboard(false)}
        >
            <SheetHeader
                canGoBack={canGoBack}
                formOf={formOf}
                nameOf={nameOf}
                onBack={sheet.pop}
                onName={rename}
                saving={saving}
                subscriptions={subscriptions}
                top={top}
            />
            <Drawer.Content className="min-h-0 flex-1 overflow-y-auto overscroll-contain px-6 pb-6">
                {inputRef ? <ScreenTransition active={!screen} focusOnMount={false} instant={sheet.instant}>
                    <SearchScreen
                            discovery={discovery}
                            inputRef={inputRef}
                            onChange={(value) => {
                                reset();
                                setUrl(value);
                            }}
                            onReset={() => {
                                reset();
                                inputRef.current?.focus();
                            }}
                            onSubmit={find}
                            sheet={sheet}
                            url={url}
                        />
                </ScreenTransition> : null}
                {stack.map((entry, index) => (
                    <ScreenTransition active={index === stack.length - 1} focusOnMount instant={sheet.instant} key={entry.id}>
                        <ScreenView form={formOf(entry)} name={nameOf(entry)} screen={entry.screen} sheet={sheet} />
                    </ScreenTransition>
                ))}
            </Drawer.Content>
        </div>
    );
}

// The name a nameable screen will save sits in the header; its length error sits just under it.
function SheetHeader({ top, subscriptions, nameOf, formOf, canGoBack, saving, onBack, onName }: {
    top?: { id: number; screen: Screen };
    subscriptions: Map<string, string | null>;
    nameOf: (entry: { id: number; screen: Screen }) => string;
    formOf: (entry: { id: number }) => string;
    canGoBack: boolean;
    saving: boolean;
    onBack: () => void;
    onName: (id: number, name: string) => void;
}) {
    const named = top && nameable(top.screen, subscriptions) ? { id: top.id, name: nameOf(top), form: formOf(top), screen: top.screen } : undefined;
    return (
        <>
            <header className="flex h-14 shrink-0 items-center gap-1 px-3">
                {canGoBack ? (
                    <Button aria-label="Back" className="text-muted-foreground" disabled={saving} onClick={onBack} size="icon" variant="ghost">
                        <ChevronDownIcon className="size-4 rotate-90" />
                    </Button>
                ) : null}
                {named ? (
                    <>
                        <Drawer.Title className="sr-only">{named.name}</Drawer.Title>
                        <NameInput
                            disabled={saving}
                            form={named.form}
                            inset={!canGoBack}
                            invalid={nameTooLong(named.name, named.screen)}
                            onChange={(name) => onName(named.id, name)}
                            value={named.name}
                        />
                    </>
                ) : (
                    <Drawer.Title className={cn('min-w-0 flex-1 truncate text-base font-medium', !canGoBack && 'pl-3')}>
                        {top ? screenTitle(top.screen) : 'Add a source'}
                    </Drawer.Title>
                )}
                <Drawer.Close
                    render={<Button aria-label="Close" className="text-muted-foreground" disabled={saving} size="icon" variant="ghost" />}
                >
                    <CloseIcon className="size-5" />
                </Drawer.Close>
            </header>
            {named ? <NameLengthHint canGoBack={canGoBack} form={named.form} name={named.name} screen={named.screen} /> : null}
        </>
    );
}

// Screens below the top stay mounted, so going back keeps their drafts and loaded posts; ids keep a re-pushed screen from inheriting a popped one's state. A root screen is never popped.
function useScreenStack(root?: Screen) {
    const [stack, setStack] = useState<{ id: number; screen: Screen }[]>(root ? [{ id: 0, screen: root }] : []);
    const nextId = useRef(1);
    return {
        stack,
        canGoBack: stack.length > (root ? 1 : 0),
        push: (next: Screen) => setStack([...stack, { id: nextId.current++, screen: next }]),
        pop: () => setStack(stack.slice(0, -1)),
    };
}

// Names typed into the header, per screen, until that screen saves them; each screen's form id ties the header field to its save.
function useScreenNames() {
    const [names, setNames] = useState<Map<number, string>>(new Map());
    const formPrefix = useId();
    return {
        nameOf: (entry: { id: number; screen: Screen }) => names.get(entry.id) ?? screenTitle(entry.screen),
        formOf: (entry: { id: number }) => `${formPrefix}-${entry.id}`,
        rename: (id: number, name: string) => setNames((previous) => new Map(previous).set(id, name)),
    };
}

function NameLengthHint({ name, screen, form, canGoBack }: { name: string; screen: Screen; form: string; canGoBack: boolean }) {
    if (!nameTooLong(name, screen)) return null;
    return (
        <p className={cn('-mt-2 pr-6 pb-2 text-sm text-destructive', canGoBack ? 'pl-16' : 'pl-6')} id={nameHintId(form)}>
            Names can be up to {MAX_TITLE_LENGTH} characters.
        </p>
    );
}

function screenTitle(screen: Screen) {
    if (screen.kind === 'feed') return sourceTitle(screen.candidate);
    if (screen.kind === 'edit') return feedSourceName(screen.source);
    if (screen.kind === 'post') return screen.post.title || 'Untitled';
    return 'Profile';
}

// Focus follows the screen, since the row or button that opened it is gone. A screen that is covered waits at the pop offset, so it returns from the left.
function ScreenTransition({ active, instant, focusOnMount, children }: {
    active: boolean;
    instant: boolean;
    focusOnMount: boolean;
    children: ReactNode;
}) {
    const ref = useRef<HTMLDivElement>(null);
    const mounted = useRef(false);
    useEffect(() => {
        if (active && (focusOnMount || mounted.current)) ref.current?.focus({ preventScroll: true });
        mounted.current = true;
    }, [active, focusOnMount]);
    return (
        <motion.div
            animate={active ? { opacity: 1, transform: SCREEN.rest } : { opacity: 0, transform: SCREEN.pop }}
            className="outline-none"
            hidden={!active}
            initial={instant ? false : { opacity: 0, transform: SCREEN.push }}
            ref={ref}
            tabIndex={-1}
            transition={active && !instant ? { duration: TIMING.screen, ease: SCREEN.ease } : { duration: 0 }}
        >
            {children}
        </motion.div>
    );
}

// Only a screen that will save a name lets the reader type one.
function nameable(screen: Screen, subscriptions: Map<string, string | null>) {
    if (screen.kind === 'edit') return true;
    if (screen.kind !== 'feed') return false;
    const status = candidateStatus(screen.candidate, screen.site, subscriptions, screen.rkey);
    return !status.rkey && !status.note;
}

// An untouched name is never saved, so a long default (an untitled feed's address) does not block the form.
function nameTooLong(name: string, screen: Screen) {
    return name.trim() !== screenTitle(screen) && titleTooLong(name);
}

// The label spans the name and its pen, so a click anywhere on either edits the name. The name sits in the header, outside its screen's form, so form ties Enter to that form's save.
function NameInput({ value, form, invalid, inset, disabled, onChange }: {
    value: string;
    form: string;
    invalid: boolean;
    inset: boolean;
    disabled: boolean;
    onChange: (name: string) => void;
}) {
    return (
        <div className={cn('flex min-w-0 flex-1', inset ? 'ml-3' : 'ml-2')}>
            <label className={cn('group flex h-9 max-w-full min-w-0 cursor-text items-center gap-1.5', disabled && 'pointer-events-none')}>
                <span className="relative flex min-w-0">
                    <input
                        aria-describedby={invalid ? nameHintId(form) : undefined}
                        aria-invalid={invalid}
                        aria-label="Name"
                        form={form}
                        autoComplete="off"
                        className="min-w-0 truncate bg-transparent text-base font-medium outline-none field-sizing-content"
                        disabled={disabled}
                        onChange={(event) => onChange(event.target.value)}
                        spellCheck={false}
                        value={value}
                    />
                    <span
                        aria-hidden="true"
                        className={cn(
                            'absolute inset-x-0 bottom-0 h-px origin-left transition-transform duration-(--motion-duration-ui) ease-(--ease-out) group-focus-within:scale-x-100',
                            invalid ? 'scale-x-100 bg-destructive' : 'scale-x-0 bg-subtle-foreground/50',
                        )}
                    />
                </span>
                <PenIcon aria-hidden="true" className="size-3.5 shrink-0 text-subtle-foreground" />
            </label>
        </div>
    );
}

function nameHintId(form: string) {
    return `${form}-name-hint`;
}

function ScreenView({ screen, form, name, sheet }: { screen: Screen; form: string; name: string; sheet: Sheet }) {
    const tooLong = nameTooLong(name, screen);
    if (screen.kind === 'feed') return <FeedScreen candidate={screen.candidate} form={form} known={screen.rkey} name={name} nameTooLong={tooLong} sheet={sheet} site={screen.site} />;
    if (screen.kind === 'edit') return <EditScreen form={form} name={name} nameTooLong={tooLong} screen={screen} sheet={sheet} />;
    if (screen.kind === 'post') return <PostScreen post={screen.post} />;
    return <ProfileScreen profile={screen.profile} sheet={sheet} site={screen.site} />;
}

function SearchScreen({ inputRef, url, discovery, sheet, onChange, onSubmit, onReset }: {
    inputRef: RefObject<HTMLInputElement | null>;
    url: string;
    discovery: Discovery;
    sheet: Sheet;
    onChange: (value: string) => void;
    onSubmit: (event: FormEvent) => void;
    onReset: () => void;
}) {
    return (
        <>
            <Drawer.Description className="text-sm text-muted-foreground">
                Paste a site, blog, or YouTube channel.
            </Drawer.Description>
            <UrlForm discovery={discovery} inputRef={inputRef} onChange={onChange} onSubmit={onSubmit} saving={sheet.saving} url={url} />
            <div className="mt-8">
                {discovery.stage === 'loading' ? <Pending label="Looking for feeds" /> : null}
                {discovery.stage === 'found' ? (
                    <div>
                        <p className="sr-only" role="status">{foundFeeds(discovery.site)}</p>
                        <SiteResults sheet={sheet} site={discovery.site} />
                        <Button className="mt-4 -ml-2.5 text-muted-foreground" disabled={sheet.saving} onClick={onReset} size="sm" variant="ghost">
                            Add a newsletter instead
                        </Button>
                    </div>
                ) : null}
                {discovery.stage === 'idle' || discovery.stage === 'error' ? <NewsletterAddress onNavigate={sheet.close} /> : null}
            </div>
        </>
    );
}

function UrlForm({ inputRef, url, discovery, saving, onChange, onSubmit }: {
    inputRef: RefObject<HTMLInputElement | null>;
    url: string;
    discovery: Discovery;
    saving: boolean;
    onChange: (value: string) => void;
    onSubmit: (event: FormEvent) => void;
}) {
    const loading = discovery.stage === 'loading';
    const invalid = discovery.stage === 'error';
    return (
        <form className="mt-4" onSubmit={onSubmit}>
            <label className="sr-only" htmlFor="source-url">Website or feed URL</label>
            <InputGroup>
                <InputGroupInput
                    aria-describedby={invalid ? 'discovery-error' : undefined}
                    aria-invalid={invalid}
                    autoCapitalize="none"
                    autoComplete="url"
                    disabled={saving}
                    id="source-url"
                    inputMode="url"
                    onChange={(event) => onChange(event.target.value)}
                    placeholder="example.com"
                    ref={inputRef}
                    required
                    spellCheck={false}
                    value={url}
                />
                <InputGroupButton aria-busy={loading} aria-label="Find feeds" disabled={saving || loading || !url.trim()} type="submit">
                    {loading ? <LoadingIcon className="motion-safe:animate-spin" /> : <EnterIcon />}
                </InputGroupButton>
            </InputGroup>
            {discovery.stage === 'error' ? (
                <p className="mt-3 text-sm text-destructive" id="discovery-error" role="alert">{discovery.error}</p>
            ) : null}
        </form>
    );
}

function foundFeeds(site: SiteResult) {
    const count = site.candidates.length;
    if (count === 0) return 'No feed found. You can generate one.';
    return count === 1 ? '1 feed found.' : `${count} feeds found.`;
}

function Pending({ label }: { label: string }) {
    return (
        <p className="flex h-36 items-center justify-center text-muted-foreground" role="status">
            <LoadingIcon className="size-5 motion-safe:animate-spin" />
            <span className="sr-only">{label}</span>
        </p>
    );
}

function SiteResults({ site, sheet }: { site: SiteResult; sheet: Sheet }) {
    const feedsLabel = useId();
    if (site.candidates.length === 0)
        return (
            <GenerateFeed
                instant={sheet.instant}
                onSaved={sheet.close}
                onSavingChange={sheet.onSavingChange}
                saving={sheet.saving}
                url={site.url}
            />
        );
    const profile = site.profile;
    const summary = profile && profileSummary(profile);
    return (
        <div className="space-y-6">
            {profile ? (
                <ul>
                    <Row onClick={() => sheet.push({ kind: 'profile', site, profile })}>
                        <span className="flex items-center gap-3">
                            <PersonAvatar className="size-10" seed={siteHost(site.url)} />
                            <span className="min-w-0">
                                <span className="block truncate text-sm font-medium">{profileName(profile, site)}</span>
                                <span className="block truncate text-xs text-subtle-foreground">
                                    {profile.following?.length ? `Profile · follows ${profile.following.length} sites` : 'Profile'}
                                </span>
                            </span>
                        </span>
                        {summary ? (
                            <span className="mt-2 line-clamp-2 block text-sm text-pretty text-muted-foreground">{summary}</span>
                        ) : null}
                    </Row>
                </ul>
            ) : null}
            <div>
                <p className="mb-1 text-xs text-muted-foreground" id={feedsLabel}>Available feeds</p>
                <ul aria-labelledby={feedsLabel}>
                    {site.candidates.map((candidate) => {
                        const status = candidateStatus(candidate, site, sheet.subscriptions);
                        const title = sourceTitle(candidate);
                        return (
                            <Row
                                key={sourceKey(candidate)}
                                label={status.note ? `${title}, ${status.note}` : undefined}
                                onClick={() => sheet.push({ kind: 'feed', candidate, site })}
                                trailing={status.rkey ? <CheckIcon className="size-4 shrink-0 text-success" /> : undefined}
                            >
                                <span className="block truncate text-sm font-medium" title={title}>{title}</span>
                                <span className="mt-0.5 block truncate text-xs text-subtle-foreground">{status.note ?? feedMeta(candidate)}</span>
                            </Row>
                        );
                    })}
                </ul>
            </div>
        </div>
    );
}

function profileName(profile: SiteProfile, site: SiteResult) {
    return profile.name || displayURL(site.url);
}

function feedMeta(candidate: SourceCandidate) {
    const location = displayURL(sourceLocation(candidate));
    return candidate.kind === 'standardfeed' ? `Standardfeed · ${location}` : location;
}

function candidateStatus(candidate: SourceCandidate, site: SiteResult | undefined, subscriptions: Map<string, string | null>, known?: string) {
    const key = sourceKey(candidate);
    const rkey = subscriptions.has(key)
        ? subscriptions.get(key)
        : (known ?? site?.existingSubscriptions.find((sub) => sub.feedUrl === key)?.rkey);
    if (rkey) return { rkey, note: 'In your sources' };
    if (followsSibling(candidate, site, subscriptions))
        return { rkey: undefined, note: `You follow this site through its ${candidate.kind === 'standardfeed' ? 'RSS feed' : 'Standardfeed publication'}` };
    return { rkey: undefined, note: undefined };
}

// A site is followed through its RSS feed or its Standardfeed publication, never both.
function followsSibling(candidate: SourceCandidate, site: SiteResult | undefined, subscriptions: Map<string, string | null>) {
    const native = candidate.kind === 'standardfeed';
    const siblings = (site?.candidates ?? [])
        .filter((other) => (other.kind === 'standardfeed') !== native)
        .map((other) => sourceKey(other));
    const existing = new Set(site?.existingSubscriptions.map((sub) => sub.feedUrl));
    const followedSibling = siblings.some((other) => (subscriptions.has(other) ? Boolean(subscriptions.get(other)) : existing.has(other)));
    // subscribedVia may name a feed this lookup didn't list; one it did list is covered above, including its removal here.
    const followedElsewhere = Boolean(candidate.subscribedVia) && !siblings.some((other) => existing.has(other));
    return followedSibling || followedElsewhere;
}

function Row({ onClick, label, trailing, children }: {
    onClick: () => void;
    label?: string;
    trailing?: ReactNode;
    children: ReactNode;
}) {
    return (
        <li>
            <button
                aria-label={label}
                className="-mx-3 flex min-h-16 w-[calc(100%+1.5rem)] cursor-pointer items-center gap-3 rounded-lg px-3 py-3 text-left transition-[background-color] duration-(--motion-duration-fast) hover:bg-secondary/60"
                onClick={onClick}
                type="button"
            >
                <span className="min-w-0 flex-1">{children}</span>
                {trailing ?? <ChevronDownIcon className="size-4 shrink-0 -rotate-90 text-subtle-foreground" />}
            </button>
        </li>
    );
}

// Turning Shorts off follows the channel's UULF feed instead; a feed left as it was keeps its URL exactly.
function useShortsChoice(feedUrl: string | undefined) {
    const feeds = feedUrl ? youtubeFeeds(feedUrl) : undefined;
    const initial = feeds?.longForm === feedUrl;
    const [exclude, setExclude] = useState(initial);
    const chosen = !feeds || exclude === initial ? feedUrl : exclude ? feeds.longForm : feeds.all;
    return { offered: Boolean(feeds), exclude, setExclude, feedUrl: chosen };
}

function ShortsSwitch({ choice, disabled }: { choice: ReturnType<typeof useShortsChoice>; disabled: boolean }) {
    if (!choice.offered) return null;
    return (
        <OptionSwitch
            checked={choice.exclude}
            description="Only regular videos from this channel."
            disabled={disabled}
            label="Exclude Shorts"
            onChange={choice.setExclude}
        />
    );
}

function useSheetMutation(sheet: Sheet) {
    const [error, setError] = useState<{ message: string; reauth: boolean } | null>(null);

    async function run(action: () => Promise<void>, failed: string) {
        if (sheet.saving) return;
        sheet.onSavingChange(true);
        setError(null);
        try {
            await action();
        } catch (problem) {
            setError({ message: describeMutationError(problem, failed), reauth: classifyMutationError(problem) === 'reauth' });
        } finally {
            sheet.onSavingChange(false);
        }
    }

    const errorLine = error ? (
        <p className="text-sm text-destructive" role="alert">
            {error.message} {error.reauth ? <Link href={PATHS.login}>Sign in again</Link> : null}
        </p>
    ) : null;

    return { run, errorLine };
}

function FeedScreen({ candidate, site, known, form, name, nameTooLong, sheet }: {
    candidate: SourceCandidate;
    site?: SiteResult;
    known?: string;
    form: string;
    name: string;
    nameTooLong: boolean;
    sheet: Sheet;
}) {
    const status = candidateStatus(candidate, site, sheet.subscriptions, known);
    const shorts = useShortsChoice(candidate.feedUrl);
    // Saves are recorded under the candidate's key, so the channel reads as followed whichever of its feeds was added.
    const source = useMemo<SourceCandidate>(
        () => (candidate.kind !== 'standardfeed' && shorts.feedUrl ? { ...candidate, feedUrl: shorts.feedUrl } : candidate),
        [candidate, shorts.feedUrl],
    );
    const { run, errorLine } = useSheetMutation(sheet);
    const renamed = name.trim() !== sourceTitle(candidate);

    function save(options: SourceOptions) {
        void run(async () => {
            const { rkey } = await addSource(source, { ...options, title: renamed ? name.trim() || undefined : undefined });
            sheet.record(sourceKey(candidate), rkey);
            sheet.announce(rkey);
            sheet.pop();
        }, 'The save couldn’t be confirmed.');
    }

    function remove(rkey: string) {
        void run(async () => {
            await unsubscribe(rkey);
            sheet.record(sourceKey(candidate), null);
            toast.success('Source removed');
        }, 'The removal couldn’t be confirmed.');
    }

    return (
        <div className="space-y-8">
            <p className="truncate text-xs text-subtle-foreground" title={sourceLocation(source)}>{feedMeta(source)}</p>
            <PostPreview candidate={source} disabled={sheet.saving} onOpen={(post) => sheet.push({ kind: 'post', candidate: source, post })} />
            {status.rkey ? (
                <>
                    <p className="flex items-center gap-2 rounded-lg bg-background p-4 text-sm text-muted-foreground">
                        <CheckIcon className="size-4 text-success" />
                        {status.note}
                    </p>
                    {errorLine}
                    <SheetAction busyLabel="Removing source" onClick={() => remove(status.rkey)} saving={sheet.saving} variant="secondary">
                        Remove from sources
                    </SheetAction>
                </>
            ) : status.note ? (
                <p className="rounded-lg bg-background p-4 text-sm text-muted-foreground">{status.note}</p>
            ) : (
                <SourceForm action="Add to sources" busyLabel="Adding source" errorLine={errorLine} id={form} nameTooLong={nameTooLong} onSave={save} saving={sheet.saving}>
                    <ShortsSwitch choice={shorts} disabled={sheet.saving} />
                </SourceForm>
            )}
        </div>
    );
}

function EditScreen({ screen, form, name, nameTooLong, sheet }: {
    screen: Extract<Screen, { kind: 'edit' }>;
    form: string;
    name: string;
    nameTooLong: boolean;
    sheet: Sheet;
}) {
    const { source, onChanged, onRemoved, removedFocus } = screen;
    const standard = source.kind === 'standardfeed';
    const shorts = useShortsChoice(standard ? undefined : source.feedUrl);
    const feedUrl = shorts.feedUrl ?? source.feedUrl;
    const candidate = useMemo<SourceCandidate>(
        () => (standard
            ? { kind: 'standardfeed', publication: source.publication ?? source.feedUrl, siteUrl: source.siteUrl }
            : { feedUrl, siteUrl: source.siteUrl }),
        [standard, source.publication, source.feedUrl, source.siteUrl, feedUrl],
    );
    const { run, errorLine } = useSheetMutation(sheet);
    const [removing, setRemoving] = useState(false);

    function changeFor({ tags, primary }: SourceOptions) {
        const change: SourceChange = {};
        // An emptied name resets the source to its feed's own title.
        if (name.trim() !== feedSourceName(source)) change.title = name.trim();
        if (tags.join('\n') !== (source.tags ?? []).join('\n')) change.tags = tags;
        if (primary !== source.primary) change.primary = primary;
        if (feedUrl !== source.feedUrl) change.feedUrl = feedUrl;
        return change;
    }

    function save(options: SourceOptions) {
        const change = changeFor(options);
        void run(async () => {
            try {
                await editSource(source.rkey, change);
            } catch (problem) {
                if (!isGone(problem)) throw problem;
                toast.error(GONE);
                onChanged();
                sheet.close();
                return;
            }
            toast.success('Source updated');
            onChanged();
            sheet.close();
        }, 'The change couldn’t be confirmed.');
    }

    return (
        <div className="space-y-8">
            <p className="truncate text-xs text-subtle-foreground" title={sourceLocation(candidate)}>{feedMeta(candidate)}</p>
            <PostPreview candidate={candidate} disabled={sheet.saving} onOpen={(post) => sheet.push({ kind: 'post', candidate, post })} />
            <SourceForm
                action="Save changes"
                busyLabel="Saving source"
                errorLine={errorLine}
                initial={{ tags: source.tags ?? [], primary: source.primary }}
                changed={(options) => Object.keys(changeFor(options)).length > 0}
                id={form}
                nameTooLong={nameTooLong}
                onSave={save}
                saving={sheet.saving}
                secondary={(
                    <Button className="w-full" disabled={sheet.saving} onClick={() => setRemoving(true)} size="lg" type="button" variant="destructive">
                        Delete source
                    </Button>
                )}
            >
                <ShortsSwitch choice={shorts} disabled={sheet.saving} />
            </SourceForm>
            <RemoveDialog
                onChanged={onChanged}
                onClose={() => setRemoving(false)}
                onRemoved={() => {
                    sheet.close();
                    onRemoved();
                }}
                open={removing}
                removedFocus={removedFocus}
                source={feedEditable(source, feedSourceName(source))}
            />
        </div>
    );
}

function SourceForm({ id, action, busyLabel, nameTooLong, initial = { tags: [], primary: false }, changed, secondary, saving, errorLine, onSave, children }: {
    id: string;
    action: string;
    secondary?: ReactNode;
    // Absent when any submit is a change, as an add always is.
    changed?: (options: SourceOptions) => boolean;
    busyLabel: string;
    nameTooLong: boolean;
    initial?: SourceOptions;
    saving: boolean;
    errorLine: ReactNode;
    onSave: (options: SourceOptions) => void;
    children?: ReactNode;
}) {
    const tagInput = useRef<HTMLInputElement>(null);
    const [tags, setTags] = useState<string[]>(initial.tags);
    const [draft, setDraft] = useState('');
    const [primary, setPrimary] = useState(initial.primary);
    const overlong = hasOverlongTag(draft) || nameTooLong;
    return (
        <form
            className="space-y-8"
            id={id}
            onSubmit={(event) => {
                event.preventDefault();
                if (!overlong) onSave({ tags: addTags(tags, draft), primary });
            }}
        >
            <div>
                <TagInput
                    canAdd={canAddTags(tags, draft)}
                    draft={draft}
                    inputRef={tagInput}
                    onAdd={(input) => {
                        if (!canAddTags(tags, input)) return;
                        setTags(addTags(tags, input));
                        setDraft('');
                    }}
                    onDraft={setDraft}
                    overlong={overlong}
                    tags={tags}
                />
                {tags.length ? <TagChips inputRef={tagInput} onRemove={(tag) => setTags(tags.filter((other) => other !== tag))} tags={tags} /> : null}
            </div>
            {children}
            <OptionSwitch
                checked={primary}
                description="Shown first in your newspaper."
                disabled={saving}
                label="Primary"
                onChange={setPrimary}
            />
            {errorLine}
            <SheetAction
                busyLabel={busyLabel}
                disabled={overlong || (changed && !changed({ tags: addTags(tags, draft), primary }))}
                saving={saving}
                secondary={secondary}
                type="submit"
            >
                {action}
            </SheetAction>
        </form>
    );
}

// secondary rides in the same sticky footer, so it stays beside the main action.
function SheetAction({ saving, busyLabel, disabled = false, secondary, children, ...props }: Omit<ComponentProps<typeof Button>, 'disabled'> & {
    saving: boolean;
    busyLabel: string;
    disabled?: boolean;
    secondary?: ReactNode;
}) {
    return (
        <div className="sticky bottom-0 -mx-6 space-y-2 bg-popover px-6 pt-2 pb-1">
            <Button aria-busy={saving} className="w-full" disabled={saving || disabled} size="lg" {...props}>
                {saving ? (
                    <>
                        <LoadingIcon className="motion-safe:animate-spin" />
                        <span className="sr-only">{busyLabel}</span>
                    </>
                ) : children}
            </Button>
            {secondary}
        </div>
    );
}

const POST_DAY = new Intl.DateTimeFormat(undefined, { day: 'numeric', month: 'short' });
const POST_DATE = new Intl.DateTimeFormat(undefined, { day: 'numeric', month: 'short', year: 'numeric' });

function postDate(value?: string) {
    if (!value) return undefined;
    const date = new Date(value);
    if (Number.isNaN(date.getTime())) return undefined;
    return (date.getFullYear() === new Date().getFullYear() ? POST_DAY : POST_DATE).format(date);
}

// A save pops its feed screen when it lands, so nothing may open on top of it meanwhile.
function PostPreview({ candidate, disabled, onOpen }: { candidate: SourceCandidate; disabled: boolean; onOpen: (post: PreviewPost) => void }) {
    const label = useId();
    const [posts, setPosts] = useState<PreviewPost[] | 'loading' | 'failed'>('loading');
    useEffect(() => {
        const controller = new AbortController();
        previewSource(candidate, controller.signal).then(
            (items) => setPosts(items),
            () => {
                if (!controller.signal.aborted) setPosts('failed');
            },
        );
        return () => controller.abort();
    }, [candidate]);
    return (
        <section aria-labelledby={label}>
            <h3 className="text-xs text-muted-foreground" id={label}>Latest posts</h3>
            <div className="mt-2 min-h-27 pointer-coarse:min-h-33">
                {posts === 'loading' ? (
                    <ul aria-label="Loading latest posts" role="status">
                        {[72, 48, 60].map((width) => (
                            <li className="flex h-9 items-center justify-between gap-4 pointer-coarse:h-11" key={width}>
                                <span className="h-3.5 animate-pulse rounded bg-muted motion-reduce:animate-none" style={{ width: `${width}%` }} />
                                <span className="h-3 w-10 animate-pulse rounded bg-muted motion-reduce:animate-none" />
                            </li>
                        ))}
                    </ul>
                ) : posts === 'failed' ? (
                    <p className="pt-3 text-sm text-muted-foreground">Recent posts couldn’t be loaded.</p>
                ) : posts.length ? (
                    <ol>
                        {posts.map((post, index) => (
                            <li key={index}>
                                <button
                                    className="-mx-2 flex h-9 w-[calc(100%+1rem)] pointer-coarse:h-11 cursor-pointer items-center justify-between disabled:pointer-events-none gap-4 rounded-md px-2 text-left text-sm transition-[background-color] duration-(--motion-duration-fast) hover:bg-secondary/60"
                                    disabled={disabled}
                                    onClick={() => onOpen(post)}
                                    type="button"
                                >
                                    <span className="min-w-0 truncate" title={post.title}>{post.title || 'Untitled'}</span>
                                    <span className="shrink-0 text-xs text-subtle-foreground tabular-nums">{postDate(post.publishedAt)}</span>
                                </button>
                            </li>
                        ))}
                    </ol>
                ) : (
                    <p className="pt-3 text-sm text-muted-foreground">Nothing published yet. New posts reach your newspaper as they appear.</p>
                )}
            </div>
        </section>
    );
}

const ARTICLE_NOTICE = {
    empty: 'No additional content was found. You can read the original or try again.',
    failed: 'The full article could not be loaded. You can try again or open the original.',
};

// Mirrors the reader: the feed's body first, the extracted article when the feed carries none, and a way to load it when the body is only a summary.
function PostScreen({ post }: { post: PreviewPost }) {
    const original = httpURL(post.url);
    const { body, extracted, pending, notice, loadFull } = useArticle(original, post.contentHtml?.trim() ?? '');
    const date = postDate(post.publishedAt);
    return (
        <article className="space-y-8">
            {date ? <p className="text-xs text-subtle-foreground">{date}</p> : null}
            <ArticleBody body={body} hasNotice={Boolean(notice)} original={original} pending={pending} url={post.url} />
            {original && body && !extracted ? (
                <div className="space-y-3 text-sm text-muted-foreground">
                    <p>Only seeing part of the article?</p>
                    <Button disabled={pending} onClick={() => void loadFull(original)} variant="secondary">
                        {pending ? 'Loading full article…' : 'Load full article'}
                    </Button>
                </div>
            ) : null}
            {notice ? <p className="text-sm text-muted-foreground" role="status">{notice}</p> : null}
            {original ? (
                <a
                    className="inline-flex min-h-8 items-center gap-2 rounded-sm text-sm text-muted-foreground underline decoration-border underline-offset-4 hover:text-foreground"
                    href={original}
                    rel="noopener noreferrer"
                    target="_blank"
                >
                    Read original
                    <ExternalLinkIcon className="size-3.5" />
                </a>
            ) : null}
        </article>
    );
}

function useArticle(original: string | null, summary: string) {
    const [body, setBody] = useState(summary);
    const [extracted, setExtracted] = useState(false);
    const [pending, setPending] = useState(!summary && Boolean(original));
    const [notice, setNotice] = useState('');

    useEffect(() => {
        if (summary || !original) return;
        const controller = new AbortController();
        previewArticle(original, controller.signal).then(
            (content) => {
                setPending(false);
                if (!content.trim()) return setNotice(ARTICLE_NOTICE.empty);
                setBody(content);
                setExtracted(true);
            },
            () => {
                if (controller.signal.aborted) return;
                setPending(false);
                setNotice(ARTICLE_NOTICE.failed);
            },
        );
        return () => controller.abort();
    }, [summary, original]);

    async function loadFull(url: string) {
        setPending(true);
        setNotice('');
        try {
            const content = await previewArticle(url);
            if (!hasNewReaderContent(body, content)) return setNotice(ARTICLE_NOTICE.empty);
            setBody(content);
            setExtracted(true);
        } catch {
            setNotice(ARTICLE_NOTICE.failed);
        } finally {
            setPending(false);
        }
    }

    return { body, extracted, pending, notice, loadFull };
}

function ArticleBody({ body, url, original, pending, hasNotice }: {
    body: string;
    url?: string;
    original: string | null;
    pending: boolean;
    hasNotice: boolean;
}) {
    const [font] = useReaderFont();
    if (body)
        return (
            <div
                className="reader-body"
                dangerouslySetInnerHTML={{ __html: readerBody({ body, url, contentType: 'blogpost' }) }}
                data-font={font}
            />
        );
    if (pending)
        return (
            <div aria-label="Loading article" className="space-y-3" role="status">
                {[100, 100, 70].map((width, index) => (
                    <div className="h-5 animate-pulse rounded bg-muted motion-reduce:animate-none" key={index} style={{ width: `${width}%` }} />
                ))}
            </div>
        );
    if (hasNotice) return null;
    return (
        <p className="text-muted-foreground">
            {original ? 'There is no readable content here. Open the original to continue.' : 'No content is available for this post.'}
        </p>
    );
}

function OptionSwitch({ label, description, checked, disabled, onChange }: {
    label: string;
    description: string;
    checked: boolean;
    disabled: boolean;
    onChange: (checked: boolean) => void;
}) {
    const id = useId();
    return (
        <div className="flex items-center justify-between gap-4">
            <label className="cursor-pointer" htmlFor={id}>
                <span className="block text-sm font-medium">{label}</span>
                <span className="block text-xs text-muted-foreground">{description}</span>
            </label>
            <Switch.Root
                checked={checked}
                className="relative flex h-6 w-10 shrink-0 cursor-pointer rounded-full bg-accent p-0.5 transition-[background-color] duration-(--motion-duration-ui) ease-out data-checked:bg-primary data-disabled:cursor-not-allowed"
                disabled={disabled}
                id={id}
                onCheckedChange={onChange}
            >
                <Switch.Thumb className="size-5 rounded-full bg-popover shadow-[0_1px_2px_oklch(0.216_0.006_56.043/0.2)] transition-transform duration-(--motion-duration-ui) ease-(--ease-out) data-checked:translate-x-4" />
            </Switch.Root>
        </div>
    );
}

function siteHost(url: string) {
    return displayURL(url).split('/')[0];
}

function ProfileScreen({ profile, site, sheet }: { profile: SiteProfile; site: SiteResult; sheet: Sheet }) {
    const name = profileName(profile, site);
    const [font] = useReaderFont();
    return (
        <div className="space-y-8">
            <div className="flex items-center gap-4">
                <PersonAvatar className="size-14" seed={siteHost(site.url)} />
                <div className="min-w-0">
                    <p className="truncate text-xl font-medium">{name}</p>
                    <p className="truncate text-sm text-subtle-foreground">{displayURL(site.url)}</p>
                </div>
            </div>
            {profile.summary ? <p className="text-pretty text-muted-foreground">{profile.summary}</p> : null}
            {profile.aboutHtml ? <div className="reader-body" dangerouslySetInnerHTML={{ __html: profile.aboutHtml }} data-font={font} data-size="compact" /> : null}
            {profile.contact?.length ? (
                <section aria-labelledby="profile-contact">
                    <h3 className="mb-3 text-xs text-muted-foreground" id="profile-contact">Contact</h3>
                    <dl className="space-y-3">
                        {profile.contact.map((contact) => (
                            <div className="min-w-0" key={contact.value}>
                                <dt className="text-xs text-subtle-foreground">
                                    {contact.kind === 'email' ? 'Email' : 'Website'}
                                    {contact.preferred ? ' · preferred' : ''}
                                </dt>
                                <dd className="mt-0.5 truncate text-sm">
                                    <a
                                        className="underline decoration-border underline-offset-4 hover:decoration-foreground"
                                        href={contact.kind === 'email' ? `mailto:${contact.value}` : contact.value}
                                        rel="noreferrer"
                                        target={contact.kind === 'url' ? '_blank' : undefined}
                                    >
                                        {contact.kind === 'email' ? contact.value : displayURL(contact.value)}
                                    </a>
                                </dd>
                            </div>
                        ))}
                    </dl>
                </section>
            ) : null}
            {profile.following?.length ? (
                <section aria-labelledby="profile-following">
                    <h3 className="mb-3 text-xs text-muted-foreground" id="profile-following">Follows {profile.following.length} sites</h3>
                    <ul>
                        {profile.following.map((follow) => {
                            const candidate: SourceCandidate = { feedUrl: follow.feedUrl, title: follow.title || undefined, siteUrl: follow.siteUrl };
                            const status = candidateStatus(candidate, undefined, sheet.subscriptions, follow.rkey);
                            const title = sourceTitle(candidate);
                            return (
                                <Row
                                    key={follow.feedUrl}
                                    label={status.rkey ? `${title}, ${status.note}` : undefined}
                                    onClick={() => sheet.push({ kind: 'feed', candidate, rkey: follow.rkey })}
                                    trailing={status.rkey ? <CheckIcon className="size-4 shrink-0 text-success" /> : undefined}
                                >
                                    <span className="block truncate text-sm font-medium" title={title}>{title}</span>
                                    <span className="mt-0.5 block truncate text-xs text-subtle-foreground">{feedMeta(candidate)}</span>
                                </Row>
                            );
                        })}
                    </ul>
                </section>
            ) : null}
        </div>
    );
}
