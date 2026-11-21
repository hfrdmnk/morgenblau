import { Drawer } from '@base-ui/react/drawer';
import { useReducedMotion } from 'motion/react';
import {
    useEffect,
    useEffectEvent,
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

import { GenerateFeed, GenerateOffer } from '@/components/generate-feed';
import { CheckIcon, ChevronDownIcon, CloseIcon, EnterIcon, ExternalLinkIcon, LoadingIcon, MoreIcon, PlusIcon } from '@/components/icons';
import { NewsletterAddress } from '@/components/newsletter-address';
import { PersonAvatar } from '@/components/person-avatar';
import { PublicDataHint } from '@/components/public-data-hint';
import { Skeleton } from '@/components/ui/skeleton';
import { DeleteNewsletterDialog, RemoveDialog } from '@/components/source-removal';
import { TagChips, TagInput } from '@/components/tag-input';
import { Button } from '@/components/ui/button';
import { DescriptionList } from '@/components/ui/description-list';
import { DrawerBackdrop, DrawerContent, DrawerFooter, DrawerHandle, DrawerPopup, DrawerViewport } from '@/components/ui/drawer';
import { InputGroup, InputGroupButton, InputGroupInput } from '@/components/ui/input-group';
import { Link } from '@/components/ui/link';
import { NameField } from '@/components/ui/name-field';
import { ReaderBody } from '@/components/ui/reader-body';
import { RowButton, RowSpace } from '@/components/ui/row-button';
import { Switch, SwitchLabel } from '@/components/ui/switch';
import { Frame } from '@/design/frame';
import { MotionFrame } from '@/design/motion';
import { Text } from '@/design/text';
import { useGeneration, type GenerationStep } from '@/hooks/use-generation';
import { useMediaQuery } from '@/hooks/use-media-query';
import { useReaderFont } from '@/hooks/use-reader-font';
import type { FeedSource, NewsletterSource } from '@/hooks/use-sources';
import {
    addSource,
    discoveryError,
    editSource,
    findSite,
    generatedCandidate,
    generatedVerdict,
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
import { ApiError, classifyMutationError, describeMutationError } from '@/lib/api';
import { PATHS } from '@/lib/paths';
import { hasNewReaderContent, httpURL, readerBody } from '@/lib/reader';
import { announceSourceAdded } from '@/lib/source-toast';
import { displayURL } from '@/lib/source-display';
import {
    addTags,
    canAddTags,
    editNewsletter,
    feedEditable,
    GONE,
    hasOverlongTag,
    isGone,
    MAX_TITLE_LENGTH,
    newsletterEditable,
    resumeNewsletter,
    titleTooLong,
} from '@/lib/source-edit';

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
    | { kind: 'newsletter'; source: NewsletterSource; stopped: boolean; onChanged: () => void; onRemoved: () => void; removedFocus?: RefObject<HTMLElement | null> }
    | { kind: 'post'; candidate: SourceCandidate; post: PreviewPost }
    | { kind: 'profile'; site: SiteResult; profile: SiteProfile }
    | { kind: 'generated'; url: string; step: GenerationStep };

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
    announce: (rkey: string, description?: string) => void;
    generate: (url: string) => void;
    pointAt: (url: string, anchor: string) => Promise<void>;
};

// With an initialUrl the sheet opens at once, already looking that site up, and has no trigger of its own.
export function AddSourceDialog({ trigger, initialUrl }: { trigger?: ReactElement; initialUrl?: string }) {
    const inputRef = useRef<HTMLInputElement>(null);
    const [open, setOpen] = useState(initialUrl !== undefined);
    const [saving, setSaving] = useState(false);
    return (
        <SheetDrawer
            initialFocus={(interaction) => (interaction === 'touch' ? true : inputRef.current)}
            onOpenChange={setOpen}
            open={open}
            saving={saving}
            trigger={initialUrl !== undefined ? undefined : trigger ?? (
                <Button aria-label="Add a source" size="icon-sm" variant="subtle">
                    <PlusIcon size="xl" />
                </Button>
            )}
        >
            <SheetBody initialUrl={initialUrl} inputRef={inputRef} onClose={() => setOpen(false)} onSavingChange={setSaving} saving={saving} />
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
    return <EditSheet root={{ kind: 'edit', source, onChanged, onRemoved, removedFocus }} />;
}

// The same sheet for a newsletter: rename, retag, stop or resume, and unsubscribe or delete it.
export function EditNewsletterSheet({ source, stopped, onChanged, onRemoved = onChanged, removedFocus }: {
    source: NewsletterSource;
    stopped: boolean;
    onChanged: () => void;
    onRemoved?: () => void;
    removedFocus?: RefObject<HTMLElement | null>;
}) {
    return <EditSheet root={{ kind: 'newsletter', source, stopped, onChanged, onRemoved, removedFocus }} />;
}

function EditSheet({ root }: { root: Extract<Screen, { kind: 'edit' | 'newsletter' }> }) {
    const [open, setOpen] = useState(false);
    const [saving, setSaving] = useState(false);
    return (
        <SheetDrawer
            onOpenChange={setOpen}
            open={open}
            saving={saving}
            trigger={(
                <Button aria-label={`Edit ${screenTitle(root)}`} size="icon-sm" variant="subtle">
                    <MoreIcon size="xl" />
                </Button>
            )}
        >
            <SheetBody onClose={() => setOpen(false)} onSavingChange={setSaving} root={root} saving={saving} />
        </SheetDrawer>
    );
}

function SheetDrawer({ open, saving, trigger, initialFocus, onOpenChange, children }: {
    open: boolean;
    saving: boolean;
    trigger?: ReactElement;
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
            {trigger ? <Drawer.Trigger render={trigger} /> : null}
            <Drawer.Portal>
                <DrawerBackdrop />
                <DrawerViewport side={side}>
                    <DrawerPopup initialFocus={initialFocus} side={side}>
                        {side === 'bottom' && <DrawerHandle />}
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

function newsletterName(source: NewsletterSource) {
    return source.title || source.senderAddress;
}

function lookUp(input: string, signal: AbortSignal): Promise<Discovery> {
    return findSite(input, signal).then(
        (site): Discovery => ({ stage: 'found', site }),
        (error): Discovery => ({ stage: 'error', error: discoveryError(error) }),
    );
}

// A sheet opened with a URL starts looking it up at once, as if the reader had pasted it and pressed Enter.
function useInitialLookup(initialUrl: string, requestRef: RefObject<AbortController | null>) {
    const [discovery, setDiscovery] = useState<Discovery>({ stage: initialUrl ? 'loading' : 'idle' });
    useEffect(() => {
        if (!initialUrl) return;
        const controller = new AbortController();
        requestRef.current = controller;
        void lookUp(initialUrl, controller.signal).then((next) => {
            if (!controller.signal.aborted) setDiscovery(next);
        });
        return () => controller.abort();
    }, [initialUrl, requestRef]);
    return [discovery, setDiscovery] as const;
}

// A root screen replaces the search screen (which only the add sheet, with its inputRef, shows), so the sheet opens on it and Back never leaves it.
function SheetBody({
    initialUrl = '',
    inputRef,
    root,
    saving,
    onSavingChange,
    onClose,
}: {
    initialUrl?: string;
    inputRef?: RefObject<HTMLInputElement | null>;
    root?: Screen;
    saving: boolean;
    onSavingChange: (saving: boolean) => void;
    onClose: () => void;
}) {
    const [, navigate] = useLocation();
    const [url, setUrl] = useState(initialUrl);
    const request = useRef<AbortController | null>(null);
    const [discovery, setDiscovery] = useInitialLookup(initialUrl, request);
    const { stack, canGoBack, push, pop, showGeneration } = useScreenStack(root);
    const generation = useGeneration(showGeneration);
    const { nameOf, formOf, rename } = useScreenNames();
    const [keyboard, setKeyboard] = useState(false);
    // Sources added (rkey) or removed (null) in this sheet, overriding what earlier lookups reported.
    const [subscriptions, setSubscriptions] = useState<Map<string, string | null>>(new Map());
    const reduced = useReducedMotion();
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
        announce: (rkey, description) =>
            announceSourceAdded(rkey, (to) => {
                onClose();
                navigate(to);
            }, description),
        generate: (url) => void generation.generate(url),
        pointAt: generation.pointAt,
    };

    async function find(event: FormEvent) {
        event.preventDefault();
        if (saving || discovery.stage === 'loading') return;
        request.current?.abort();
        const controller = new AbortController();
        request.current = controller;
        setDiscovery({ stage: 'loading' });
        const next = await lookUp(url, controller.signal);
        if (!controller.signal.aborted) setDiscovery(next);
    }

    function reset() {
        request.current?.abort();
        setDiscovery({ stage: 'idle' });
    }

    return (
        <Frame
            grow
            minHeight={0}
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
            <DrawerContent>
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
            </DrawerContent>
        </Frame>
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
    const hinted = named && nameTooLong(named.name, named.screen);
    return (
        <>
            {/* A length hint takes the header's bottom padding, so it sits just under the name. */}
            <Frame
                align="center"
                as="header"
                direction="row"
                gap="2xs"
                paddingBottom={hinted ? 'none' : 's'}
                paddingTop="s"
                paddingX="m"
                shrink={false}
            >
                {canGoBack ? (
                    <Button aria-label="Back" disabled={saving} onClick={onBack} size="icon" variant="quiet">
                        <ChevronDownIcon rotate={90} size="md" />
                    </Button>
                ) : null}
                {named ? (
                    <>
                        <Drawer.Title render={<Text as="h2" visuallyHidden />}>{named.name}</Drawer.Title>
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
                    <SheetTitle inset={!canGoBack} screen={top?.screen} />
                )}
                <Drawer.Close
                    render={<Button aria-label="Close" disabled={saving} size="icon" variant="quiet" />}
                >
                    <CloseIcon size="xl" />
                </Drawer.Close>
            </Frame>
            {hinted ? <NameLengthHint canGoBack={canGoBack} form={named.form} /> : null}
        </>
    );
}

// Starts where NameInput's text does, so a screen's title sits in the same place whether or not it can be renamed.
function SheetTitle({ screen, inset }: { screen?: Screen; inset: boolean }) {
    return (
        <Frame grow minWidth={0} paddingStart={inset ? 'm' : 's'}>
            <Drawer.Title render={<Text as="h2" size="base" truncate weight="medium" />}>
                {screen ? screenTitle(screen) : 'Add a source'}
            </Drawer.Title>
        </Frame>
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
        // Only one generation runs at a time, so its steps land on whichever generated screen is open.
        showGeneration: (step: GenerationStep) =>
            setStack((previous) => previous.map((entry) => (entry.screen.kind === 'generated' ? { ...entry, screen: { ...entry.screen, step } } : entry))),
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

// Lines up under the name: the header's inset, plus the back button and its gap when there is one.
function NameLengthHint({ form, canGoBack }: { form: string; canGoBack: boolean }) {
    return (
        <Frame paddingBottom="s" paddingEnd="2xl" paddingStart={canGoBack ? '6xl' : '2xl'}>
            <Text id={nameHintId(form)} size="sm" tone="destructive">
                Names can be up to {MAX_TITLE_LENGTH} characters.
            </Text>
        </Frame>
    );
}

function screenTitle(screen: Screen) {
    if (screen.kind === 'feed') return sourceTitle(screen.candidate);
    if (screen.kind === 'edit') return feedSourceName(screen.source);
    if (screen.kind === 'newsletter') return newsletterName(screen.source);
    if (screen.kind === 'post') return screen.post.title || 'Untitled';
    if (screen.kind === 'generated') return screen.step.step === 'preview' ? screen.step.source.title || displayURL(screen.step.source.siteUrl) : displayURL(screen.url);
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
        <MotionFrame
            animate={active ? { opacity: 1, transform: SCREEN.rest } : { opacity: 0, transform: SCREEN.pop }}
            hidden={!active}
            initial={instant ? false : { opacity: 0, transform: SCREEN.push }}
            outline="none"
            ref={ref}
            tabIndex={-1}
            transition={active && !instant ? { duration: TIMING.screen, ease: SCREEN.ease } : { duration: 0 }}
        >
            {children}
        </MotionFrame>
    );
}

// Only a screen that will save a name lets the reader type one.
function nameable(screen: Screen, subscriptions: Map<string, string | null>) {
    if (screen.kind === 'edit' || screen.kind === 'newsletter') return true;
    if (screen.kind === 'generated') return screen.step.step === 'preview' && generatedVerdict(screen.step.source).subscribable;
    if (screen.kind !== 'feed') return false;
    const status = candidateStatus(screen.candidate, screen.site, subscriptions, screen.rkey);
    return !status.rkey && !status.note;
}

// An untouched name is never saved, so a long default (an untitled feed's address) does not block the form.
function nameTooLong(name: string, screen: Screen) {
    return name.trim() !== screenTitle(screen) && titleTooLong(name);
}

// The name sits in the header, outside its screen's form, so form ties Enter to that form's save.
function NameInput({ value, form, invalid, inset, disabled, onChange }: {
    value: string;
    form: string;
    invalid: boolean;
    inset: boolean;
    disabled: boolean;
    onChange: (name: string) => void;
}) {
    return (
        <Frame direction="row" grow minWidth={0} paddingStart={inset ? 'm' : 's'}>
            <NameField
                aria-describedby={invalid ? nameHintId(form) : undefined}
                aria-label="Name"
                disabled={disabled}
                form={form}
                invalid={invalid}
                onChange={(event) => onChange(event.target.value)}
                value={value}
            />
        </Frame>
    );
}

function nameHintId(form: string) {
    return `${form}-name-hint`;
}

function ScreenView({ screen, form, name, sheet }: { screen: Screen; form: string; name: string; sheet: Sheet }) {
    const tooLong = nameTooLong(name, screen);
    if (screen.kind === 'feed') return <FeedScreen candidate={screen.candidate} form={form} known={screen.rkey} name={name} nameTooLong={tooLong} sheet={sheet} site={screen.site} />;
    if (screen.kind === 'edit') return <EditScreen form={form} name={name} nameTooLong={tooLong} screen={screen} sheet={sheet} />;
    if (screen.kind === 'newsletter') return <NewsletterScreen form={form} name={name} nameTooLong={tooLong} screen={screen} sheet={sheet} />;
    if (screen.kind === 'post') return <PostScreen post={screen.post} />;
    if (screen.kind === 'generated') return <GeneratedScreen form={form} name={name} nameTooLong={tooLong} screen={screen} sheet={sheet} />;
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
        <Frame gap="3xl">
            <Frame gap="l">
                <Drawer.Description render={<Text size="sm" tone="muted" />}>
                    Paste a site, blog, or YouTube channel.
                </Drawer.Description>
                <UrlForm discovery={discovery} inputRef={inputRef} onChange={onChange} onSubmit={onSubmit} saving={sheet.saving} url={url} />
            </Frame>
            <Frame>
                {discovery.stage === 'loading' ? <Pending label="Looking for feeds" /> : null}
                {discovery.stage === 'found' ? (
                    <Frame gap="l">
                        <Text role="status" visuallyHidden>{foundFeeds(discovery.site)}</Text>
                        <SiteResults sheet={sheet} site={discovery.site} />
                        <Button disabled={sheet.saving} onClick={onReset} size="text" variant="quiet">
                            Add a newsletter instead
                        </Button>
                    </Frame>
                ) : null}
                {discovery.stage === 'idle' || discovery.stage === 'error' ? <NewsletterAddress onNavigate={sheet.close} /> : null}
            </Frame>
        </Frame>
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
        <Frame as="form" gap="m" onSubmit={onSubmit}>
            <Text as="label" htmlFor="source-url" visuallyHidden>Website or feed URL</Text>
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
                    {loading ? <LoadingIcon spin /> : <EnterIcon />}
                </InputGroupButton>
            </InputGroup>
            {discovery.stage === 'error' ? (
                <Text id="discovery-error" role="alert" size="sm" tone="destructive">{discovery.error}</Text>
            ) : null}
        </Frame>
    );
}

function foundFeeds(site: SiteResult) {
    const count = site.candidates.length;
    if (count === 0) return 'No feed found. You can generate one.';
    return count === 1 ? '1 feed found.' : `${count} feeds found.`;
}

function Pending({ label }: { label: string }) {
    return (
        <Frame align="center" height="status" justify="center" role="status">
            <LoadingIcon size="xl" spin tone="muted" />
            <Text visuallyHidden>{label}</Text>
        </Frame>
    );
}

function SiteResults({ site, sheet }: { site: SiteResult; sheet: Sheet }) {
    const feedsLabel = useId();
    if (site.candidates.length === 0)
        return (
            <GenerateOffer
                onGenerate={() => {
                    sheet.push({ kind: 'generated', url: site.url, step: { step: 'pending' } });
                    sheet.generate(site.url);
                }}
            />
        );
    const profile = site.profile;
    const summary = profile && profileSummary(profile);
    return (
        <Frame gap="2xl">
            {profile ? (
                <Frame as="ul">
                    <Row gap="s" onClick={() => sheet.push({ kind: 'profile', site, profile })}>
                        <Frame align="center" as="span" direction="row" gap="m">
                            <PersonAvatar seed={siteHost(site.url)} size="lg" />
                            <Frame as="span" minWidth={0}>
                                <Text as="span" size="sm" truncate weight="medium">{profileName(profile, site)}</Text>
                                <Text as="span" size="xs" tone="subtle" truncate>
                                    {profile.following?.length ? `Profile · follows ${profile.following.length} sites` : 'Profile'}
                                </Text>
                            </Frame>
                        </Frame>
                        {summary ? (
                            <Text as="span" lineClamp={2} size="sm" tone="muted" wrap="pretty">{summary}</Text>
                        ) : null}
                    </Row>
                </Frame>
            ) : null}
            <Frame gap="2xs">
                <Text id={feedsLabel} size="xs" tone="muted">Available feeds</Text>
                <Frame aria-labelledby={feedsLabel} as="ul">
                    {site.candidates.map((candidate) => {
                        const status = candidateStatus(candidate, site, sheet.subscriptions);
                        const title = sourceTitle(candidate);
                        return (
                            <Row
                                key={sourceKey(candidate)}
                                label={status.note ? `${title}, ${status.note}` : undefined}
                                onClick={() => sheet.push({ kind: 'feed', candidate, site })}
                                trailing={status.rkey ? <CheckIcon size="md" tone="success" /> : undefined}
                            >
                                <Text as="span" size="sm" title={title} truncate weight="medium">{title}</Text>
                                <Text as="span" size="xs" tone="subtle" truncate>{status.note ?? feedMeta(candidate)}</Text>
                            </Row>
                        );
                    })}
                </Frame>
            </Frame>
        </Frame>
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

// gap separates the lines of a row's text: a title and its meta sit close, a summary further apart.
function Row({ onClick, label, trailing, gap = '3xs', children }: {
    onClick: () => void;
    label?: string;
    trailing?: ReactNode;
    gap?: 's' | '3xs';
    children: ReactNode;
}) {
    return (
        <Frame as="li">
            <RowButton aria-label={label} onClick={onClick}>
                <Frame as="span" gap={gap} grow minWidth={0}>{children}</Frame>
                {trailing ?? <ChevronDownIcon rotate={270} size="md" tone="subtle" />}
            </RowButton>
        </Frame>
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
        <Text role="alert" size="sm" tone="destructive">
            {error.message} {error.reauth ? <Link href={PATHS.login}>Sign in again</Link> : null}
        </Text>
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
    const [youtubeDown, setYouTubeDown] = useState(false);

    function save(options: SourceOptions) {
        void run(async () => {
            const { rkey } = await addSource(source, { ...options, title: renamed ? name.trim() || undefined : undefined });
            sheet.record(sourceKey(candidate), rkey);
            sheet.announce(rkey, youtubeDown ? 'Waiting on YouTube. Its videos arrive once YouTube is back.' : undefined);
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
        <Frame gap="3xl">
            <Text size="xs" title={sourceLocation(source)} tone="subtle" truncate>{feedMeta(source)}</Text>
            <PostPreview candidate={source} disabled={sheet.saving} onOpen={(post) => sheet.push({ kind: 'post', candidate: source, post })} onYouTubeDown={() => setYouTubeDown(true)} />
            {status.rkey ? (
                <>
                    <Frame align="center" background="background" direction="row" gap="s" padding="l" radius="lg">
                        <CheckIcon size="md" tone="success" />
                        <Text size="sm" tone="muted">{status.note}</Text>
                    </Frame>
                    {errorLine}
                    <SheetAction busyLabel="Removing source" onClick={() => remove(status.rkey)} saving={sheet.saving} variant="secondary">
                        Remove from sources
                    </SheetAction>
                </>
            ) : status.note ? (
                <Notice>{status.note}</Notice>
            ) : (
                <SourceForm action="Add to sources" busyLabel="Adding source" errorLine={errorLine} id={form} nameTooLong={nameTooLong} onSave={save} saving={sheet.saving} secondary={<PublicDataHint centered />}>
                    <ShortsSwitch choice={shorts} disabled={sheet.saving} />
                </SourceForm>
            )}
        </Frame>
    );
}

// The add closes the sheet: the feed is the site's only one, so there is nothing left to pick.
function GeneratedScreen({ screen, form, name, nameTooLong, sheet }: {
    screen: Extract<Screen, { kind: 'generated' }>;
    form: string;
    name: string;
    nameTooLong: boolean;
    sheet: Sheet;
}) {
    const { run, errorLine } = useSheetMutation(sheet);
    const renamed = name.trim() !== screenTitle(screen);

    function save(source: SourceCandidate, options: SourceOptions) {
        void run(async () => {
            const { rkey } = await addSource(source, { ...options, title: renamed ? name.trim() || undefined : undefined });
            sheet.announce(rkey, 'New posts appear in your newspaper as the site publishes them.');
            sheet.close();
        }, 'The save couldn’t be confirmed.');
    }

    return (
        <GenerateFeed
            add={(source) => (
                <SourceForm
                    action="Add to sources"
                    busyLabel="Adding source"
                    errorLine={errorLine}
                    id={form}
                    nameTooLong={nameTooLong}
                    onSave={(options) => save(generatedCandidate(source), options)}
                    saving={sheet.saving}
                    secondary={<PublicDataHint centered />}
                />
            )}
            instant={sheet.instant}
            onPointAt={(anchor) => sheet.pointAt(screen.url, anchor)}
            onRetry={() => sheet.generate(screen.url)}
            saving={sheet.saving}
            step={screen.step}
        />
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
        <Frame gap="3xl">
            <Text size="xs" title={sourceLocation(candidate)} tone="subtle" truncate>{feedMeta(candidate)}</Text>
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
                    <Button disabled={sheet.saving} onClick={() => setRemoving(true)} size="lg" type="button" variant="destructive">
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
        </Frame>
    );
}

function NewsletterScreen({ screen, form, name, nameTooLong, sheet }: {
    screen: Extract<Screen, { kind: 'newsletter' }>;
    form: string;
    name: string;
    nameTooLong: boolean;
    sheet: Sheet;
}) {
    const { source, stopped, onChanged, onRemoved, removedFocus } = screen;
    const editable = newsletterEditable(source, stopped);
    const { run, errorLine } = useSheetMutation(sheet);
    const [dialog, setDialog] = useState<'stop' | 'delete' | null>(null);

    function changeFor({ tags, primary }: SourceOptions) {
        const change: { title?: string; tags?: string[]; primary?: boolean } = {};
        // A newsletter has no title of its own to fall back to, so an emptied name keeps the current one.
        const title = name.trim();
        if (title && title !== newsletterName(source)) change.title = title;
        if (tags.join('\n') !== source.tags.join('\n')) change.tags = tags;
        if (primary !== source.primary) change.primary = primary;
        return change;
    }

    function write(action: () => Promise<unknown>, done: string, failed: string) {
        void run(async () => {
            try {
                await action();
            } catch (problem) {
                if (!isGone(problem)) throw problem;
                toast.error(GONE);
                onChanged();
                sheet.close();
                return;
            }
            toast.success(done);
            onChanged();
            sheet.close();
        }, failed);
    }

    return (
        <Frame gap="3xl">
            <Text size="xs" title={source.senderAddress} tone="subtle" truncate>{source.senderAddress}</Text>
            {stopped ? <Notice>Stopped. New mail from this sender is discarded until you resume it.</Notice> : null}
            <SourceForm
                action="Save changes"
                busyLabel="Saving newsletter"
                changed={(options) => Object.keys(changeFor(options)).length > 0}
                errorLine={errorLine}
                id={form}
                initial={{ tags: source.tags, primary: source.primary }}
                nameTooLong={nameTooLong}
                onSave={(options) => write(() => editNewsletter(editable, changeFor(options)), 'Newsletter updated', 'The change couldn’t be confirmed.')}
                saving={sheet.saving}
                secondary={(
                    <>
                        {stopped ? (
                            <Button disabled={sheet.saving} onClick={() => write(() => resumeNewsletter(editable), 'Newsletter resumed', 'Couldn’t resume this newsletter.')} size="lg" type="button" variant="secondary">
                                Resume newsletter
                            </Button>
                        ) : (
                            <Button disabled={sheet.saving} onClick={() => setDialog('stop')} size="lg" type="button" variant="secondary">
                                Stop newsletter
                            </Button>
                        )}
                        <Button disabled={sheet.saving} onClick={() => setDialog('delete')} size="lg" type="button" variant="destructive">
                            {source.canUnsubscribe ? 'Unsubscribe and delete' : 'Delete newsletter'}
                        </Button>
                    </>
                )}
            />
            <RemoveDialog
                onChanged={onChanged}
                onClose={() => setDialog(null)}
                onRemoved={() => {
                    sheet.close();
                    onChanged();
                }}
                open={dialog === 'stop'}
                removedFocus={removedFocus}
                source={editable}
            />
            <DeleteNewsletterDialog
                onChanged={onChanged}
                onClose={() => setDialog(null)}
                onRemoved={() => {
                    sheet.close();
                    onRemoved();
                }}
                open={dialog === 'delete'}
                removedFocus={removedFocus}
                source={editable}
            />
        </Frame>
    );
}

function Notice({ children }: { children: ReactNode }) {
    return (
        <Frame background="background" padding="l" radius="lg">
            <Text size="sm" tone="muted">{children}</Text>
        </Frame>
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
        <Frame
            as="form"
            gap="3xl"
            id={id}
            onSubmit={(event) => {
                event.preventDefault();
                if (!overlong) onSave({ tags: addTags(tags, draft), primary });
            }}
        >
            <Frame gap="m">
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
            </Frame>
            {children}
            <OptionSwitch
                checked={primary}
                description="Shown first in your newspaper."
                disabled={saving}
                label="Primary"
                onChange={setPrimary}
            />
            <SheetAction
                busyLabel={busyLabel}
                disabled={overlong || (changed && !changed({ tags: addTags(tags, draft), primary }))}
                error={errorLine}
                saving={saving}
                secondary={secondary}
                type="submit"
            >
                {action}
            </SheetAction>
        </Frame>
    );
}

// error and secondary ride in the same sticky footer, so a screen taller than the sheet never hides them behind it.
function SheetAction({ saving, busyLabel, disabled = false, error, secondary, children, ...props }: Omit<ComponentProps<typeof Button>, 'disabled'> & {
    saving: boolean;
    busyLabel: string;
    disabled?: boolean;
    error?: ReactNode;
    secondary?: ReactNode;
}) {
    return (
        <DrawerFooter>
            {error}
            <Button aria-busy={saving} disabled={saving || disabled} size="lg" {...props}>
                {saving ? (
                    <>
                        <LoadingIcon spin />
                        <Text as="span" visuallyHidden>{busyLabel}</Text>
                    </>
                ) : children}
            </Button>
            {secondary}
        </DrawerFooter>
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

const SAMPLE_POSTS: PreviewPost[] = ['A sample post title of typical length', 'A shorter sample title', 'Another sample post title'].map((title) => (
    { title, publishedAt: '2026-01-01T00:00:00Z' }
));

// A save pops its feed screen when it lands, so nothing may open on top of it meanwhile.
function PostPreview({ candidate, disabled, onOpen, onYouTubeDown }: {
    candidate: SourceCandidate;
    disabled: boolean;
    onOpen: (post: PreviewPost) => void;
    onYouTubeDown?: () => void;
}) {
    const label = useId();
    const [posts, setPosts] = useState<PreviewPost[] | 'loading' | 'failed' | 'youtubeDown'>('loading');
    const reportYouTubeDown = useEffectEvent(() => onYouTubeDown?.());
    useEffect(() => {
        const controller = new AbortController();
        previewSource(candidate, controller.signal).then(
            (items) => setPosts(items),
            (error) => {
                if (controller.signal.aborted) return;
                const youtubeDown = error instanceof ApiError && error.code === 'youtube_down';
                setPosts(youtubeDown ? 'youtubeDown' : 'failed');
                if (youtubeDown) reportYouTubeDown();
            },
        );
        return () => controller.abort();
    }, [candidate]);
    return (
        <Frame aria-labelledby={label} as="section" gap="s">
            <Text as="h3" id={label} size="xs" tone="muted">Latest posts</Text>
            {/* Room for three posts while they load; an error is one line and needs none. */}
            <RowSpace reserve={posts === 'loading' || Array.isArray(posts)}>
                <Skeleton
                    data={Array.isArray(posts) ? posts : null}
                    error={posts === 'failed' ? <PreviewNote>Recent posts couldn’t be loaded.</PreviewNote>
                        : posts === 'youtubeDown' ? (
                            <PreviewNote outage>
                                YouTube’s feeds are down right now, so videos can’t be previewed. Following the channel still works: its videos arrive once YouTube is back.
                            </PreviewNote>
                        ) : undefined}
                    label="Loading latest posts"
                    sample={SAMPLE_POSTS}
                >
                    {(items) => items.length ? (
                        <Frame as="ol">
                            {items.map((post, index) => (
                                <Frame as="li" key={index}>
                                    <RowButton disabled={disabled} onClick={() => onOpen(post)} size="compact">
                                        <Text as="span" title={post.title} truncate>{post.title || 'Untitled'}</Text>
                                        <Text as="span" numeric="tabular" size="xs" tone="subtle">{postDate(post.publishedAt)}</Text>
                                    </RowButton>
                                </Frame>
                            ))}
                        </Frame>
                    ) : (
                        <PreviewNote>Nothing published yet. New posts reach your newspaper as they appear.</PreviewNote>
                    )}
                </Skeleton>
            </RowSpace>
        </Frame>
    );
}

// A line in place of the posts; an outage is set in italics, as the one notice that is not about the feed itself.
function PreviewNote({ outage = false, children }: { outage?: boolean; children: ReactNode }) {
    return (
        <Frame paddingTop="m">
            <Text as={outage ? 'em' : 'p'} size="sm" tone="muted" wrap={outage ? 'pretty' : undefined}>{children}</Text>
        </Frame>
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
        <Frame as="article" gap="3xl">
            {date ? <Text size="xs" tone="subtle">{date}</Text> : null}
            <ArticleBody body={body} hasNotice={Boolean(notice)} original={original} pending={pending} url={post.url} />
            {original && body && !extracted ? (
                <Frame align="start" gap="m">
                    <Text size="sm" tone="muted">Only seeing part of the article?</Text>
                    <Button disabled={pending} onClick={() => void loadFull(original)} variant="secondary">
                        {pending ? 'Loading full article…' : 'Load full article'}
                    </Button>
                </Frame>
            ) : null}
            {notice ? <Text role="status" size="sm" tone="muted">{notice}</Text> : null}
            {original ? (
                <Link href={original} rel="noopener noreferrer" target="_blank" variant="external">
                    Read original
                    <ExternalLinkIcon size="sm" />
                </Link>
            ) : null}
        </Frame>
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
            <ReaderBody
                dangerouslySetInnerHTML={{ __html: readerBody({ body, url, contentType: 'blogpost' }) }}
                data-font={font}
            />
        );
    if (pending)
        return (
            <Skeleton label="Loading article">
                <ReaderBody data-font={font}>
                    <Text>A paragraph of sample text that stands in for the article while it loads, running across a few lines.</Text>
                </ReaderBody>
            </Skeleton>
        );
    if (hasNotice) return null;
    return (
        <Text tone="muted">
            {original ? 'There is no readable content here. Open the original to continue.' : 'No content is available for this post.'}
        </Text>
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
        <Frame align="center" direction="row" gap="l" justify="between">
            <SwitchLabel htmlFor={id}>
                <Text as="span" display="block" size="sm" weight="medium">{label}</Text>
                <Text as="span" display="block" size="xs" tone="muted">{description}</Text>
            </SwitchLabel>
            <Switch checked={checked} disabled={disabled} id={id} onCheckedChange={onChange} />
        </Frame>
    );
}

function siteHost(url: string) {
    return displayURL(url).split('/')[0];
}

const PROFILE_PREVIEW_COUNT = 5;

function ProfileScreen({ profile, site, sheet }: { profile: SiteProfile; site: SiteResult; sheet: Sheet }) {
    const name = profileName(profile, site);
    const [showAllFollows, setShowAllFollows] = useState(false);
    return (
        <Frame gap="3xl">
            <Frame align="center" direction="row" gap="l">
                <PersonAvatar seed={siteHost(site.url)} size="xl" />
                <Frame minWidth={0}>
                    <Text size="xl" truncate weight="medium">{name}</Text>
                    <Text size="sm" tone="subtle" truncate>{displayURL(site.url)}</Text>
                </Frame>
            </Frame>
            {profile.summary ? <Text tone="muted" wrap="pretty">{profile.summary}</Text> : null}
            {profile.aboutHtml ? <ProfileAbout html={profile.aboutHtml} /> : null}
            {profile.contact?.length ? (
                <Frame aria-labelledby="profile-contact" as="section" gap="m">
                    <Text as="h3" id="profile-contact" size="xs" tone="muted">Contact</Text>
                    <DescriptionList>
                        {profile.contact.map((contact) => (
                            <Frame gap="3xs" key={contact.value} minWidth={0}>
                                <Text as="dt" size="xs" tone="subtle">
                                    {contact.kind === 'email' ? 'Email' : 'Website'}
                                    {contact.preferred ? ' · preferred' : ''}
                                </Text>
                                <Text as="dd" size="sm" truncate>
                                    <Link
                                        href={contact.kind === 'email' ? `mailto:${contact.value}` : contact.value}
                                        rel="noreferrer"
                                        target={contact.kind === 'url' ? '_blank' : undefined}
                                        variant="underline"
                                    >
                                        {contact.kind === 'email' ? contact.value : displayURL(contact.value)}
                                    </Link>
                                </Text>
                            </Frame>
                        ))}
                    </DescriptionList>
                </Frame>
            ) : null}
            {profile.following?.length ? (
                <Frame aria-labelledby="profile-following" as="section" gap="m">
                    <Text as="h3" id="profile-following" size="xs" tone="muted">Follows {profile.following.length} sites</Text>
                    <Frame gap="s">
                    <Frame as="ul">
                        {(showAllFollows ? profile.following : profile.following.slice(0, PROFILE_PREVIEW_COUNT)).map((follow) => {
                            const candidate: SourceCandidate = { feedUrl: follow.feedUrl, title: follow.title || undefined, siteUrl: follow.siteUrl };
                            const status = candidateStatus(candidate, undefined, sheet.subscriptions, follow.rkey);
                            const title = sourceTitle(candidate);
                            return (
                                <Row
                                    key={follow.feedUrl}
                                    label={status.rkey ? `${title}, ${status.note}` : undefined}
                                    onClick={() => sheet.push({ kind: 'feed', candidate, rkey: follow.rkey })}
                                    trailing={status.rkey ? <CheckIcon size="md" tone="success" /> : undefined}
                                >
                                    <Text as="span" size="sm" title={title} truncate weight="medium">{title}</Text>
                                    <Text as="span" size="xs" tone="subtle" truncate>{feedMeta(candidate)}</Text>
                                </Row>
                            );
                        })}
                    </Frame>
                    {profile.following.length > PROFILE_PREVIEW_COUNT ? (
                        <ShowMoreButton expanded={showAllFollows} onToggle={() => setShowAllFollows(!showAllFollows)} />
                    ) : null}
                    </Frame>
                </Frame>
            ) : null}
        </Frame>
    );
}

function ProfileAbout({ html }: { html: string }) {
    const [font] = useReaderFont();
    const ref = useRef<HTMLDivElement>(null);
    const [expanded, setExpanded] = useState(false);
    const [overflows, setOverflows] = useState(false);
    // A new object makes React rewrite the HTML on every toggle, which drops focus from a link inside it.
    const content = useMemo(() => ({ __html: html }), [html]);
    useEffect(() => {
        const element = ref.current;
        if (!element || expanded) return;
        const observer = new ResizeObserver(() => setOverflows(element.scrollHeight > element.clientHeight));
        observer.observe(element);
        return () => observer.disconnect();
    }, [html, expanded]);
    return (
        <Frame gap="s">
            <ReaderBody
                clamped={!expanded}
                dangerouslySetInnerHTML={content}
                data-font={font}
                data-size="compact"
                // A link past the clamp is still tabbable, and focusing it scrolls the clipped box.
                onFocus={(event) => event.target.matches(':focus-visible') && setExpanded(true)}
                ref={ref}
            />
            {overflows ? <ShowMoreButton expanded={expanded} onToggle={() => setExpanded(!expanded)} /> : null}
        </Frame>
    );
}

function ShowMoreButton({ expanded, onToggle }: { expanded: boolean; onToggle: () => void }) {
    return (
        <Button onClick={onToggle} size="text" variant="quiet">
            {expanded ? 'Show less' : 'Show more'}
        </Button>
    );
}
