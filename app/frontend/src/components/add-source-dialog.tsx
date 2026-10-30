import { Drawer } from '@base-ui/react/drawer';
import { Switch } from '@base-ui/react/switch';
import { motion, useReducedMotion } from 'motion/react';
import {
    useEffect,
    useId,
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
import { CheckIcon, ChevronDownIcon, CloseIcon, EnterIcon, ExternalLinkIcon, LoadingIcon, PlusIcon } from '@/components/icons';
import { NewsletterAddress } from '@/components/newsletter-address';
import { PersonAvatar } from '@/components/person-avatar';
import { TagChips, TagInput } from '@/components/tag-input';
import { Button } from '@/components/ui/button';
import { DrawerBackdrop, DrawerPopup, DrawerViewport } from '@/components/ui/drawer';
import { InputGroup, InputGroupButton, InputGroupInput } from '@/components/ui/input-group';
import { Link } from '@/components/ui/link';
import { useMediaQuery } from '@/hooks/use-media-query';
import { useReaderFont } from '@/hooks/use-reader-font';
import {
    addSource,
    discoveryError,
    findSite,
    previewArticle,
    previewSource,
    sourceKey,
    sourceLocation,
    sourceTitle,
    unsubscribe,
    type PreviewPost,
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
import { addTags, canAddTags, hasOverlongTag } from '@/lib/source-edit';
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
    const side = useMediaQuery('(min-width: 40rem)') ? 'right' : 'bottom';
    return (
        <Drawer.Root
            open={open}
            onOpenChange={(next, details) => {
                if (saving) details.cancel();
                else setOpen(next);
            }}
            swipeDirection={side === 'right' ? 'right' : 'down'}
        >
            {trigger ? <Drawer.Trigger render={trigger} /> : (
                <Drawer.Trigger
                    render={
                        <Button
                            aria-label="Add a source"
                            className="text-subtle-foreground"
                            size="icon-sm"
                            variant="ghost"
                        />
                    }
                >
                    <PlusIcon className="size-5" />
                </Drawer.Trigger>
            )}
            <Drawer.Portal>
                <DrawerBackdrop />
                <DrawerViewport side={side}>
                    <DrawerPopup
                        initialFocus={(interaction) =>
                            interaction === 'touch' ? true : inputRef.current
                        }
                        side={side}
                    >
                        {side === 'bottom' && (
                            <div aria-hidden="true" className="mx-auto mt-2 h-1 w-10 shrink-0 rounded-full bg-accent" />
                        )}
                        <SheetBody
                            inputRef={inputRef}
                            saving={saving}
                            onSavingChange={setSaving}
                            onClose={() => setOpen(false)}
                        />
                    </DrawerPopup>
                </DrawerViewport>
            </Drawer.Portal>
        </Drawer.Root>
    );
}

function SheetBody({
    inputRef,
    saving,
    onSavingChange,
    onClose,
}: {
    inputRef: RefObject<HTMLInputElement | null>;
    saving: boolean;
    onSavingChange: (saving: boolean) => void;
    onClose: () => void;
}) {
    const [, navigate] = useLocation();
    const [url, setUrl] = useState('');
    const [discovery, setDiscovery] = useState<Discovery>({ stage: 'idle' });
    // Screens below the top stay mounted, so going back keeps their drafts and loaded posts; ids keep a re-pushed screen from inheriting a popped one's state.
    const [stack, setStack] = useState<{ id: number; screen: Screen }[]>([]);
    const nextId = useRef(1);
    const [keyboard, setKeyboard] = useState(false);
    // Sources added (rkey) or removed (null) in this sheet, overriding what earlier lookups reported.
    const [subscriptions, setSubscriptions] = useState<Map<string, string | null>>(new Map());
    const reduced = useReducedMotion();
    const request = useRef<AbortController | null>(null);
    useEffect(() => () => request.current?.abort(), []);
    const screen = stack.at(-1)?.screen;

    const sheet: Sheet = {
        instant: Boolean(reduced) || keyboard,
        saving,
        subscriptions,
        push: (next) => setStack([...stack, { id: nextId.current++, screen: next }]),
        pop: () => setStack(stack.slice(0, -1)),
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
            <header className="flex h-14 shrink-0 items-center gap-1 px-3">
                {screen ? (
                    <Button aria-label="Back" className="text-muted-foreground" disabled={saving} onClick={sheet.pop} size="icon" variant="ghost">
                        <ChevronDownIcon className="size-4 rotate-90" />
                    </Button>
                ) : null}
                <Drawer.Title className={cn('min-w-0 flex-1 truncate text-base font-medium', !screen && 'pl-3')}>
                    {screen ? screenTitle(screen) : 'Add a source'}
                </Drawer.Title>
                <Drawer.Close
                    render={<Button aria-label="Close" className="text-muted-foreground" disabled={saving} size="icon" variant="ghost" />}
                >
                    <CloseIcon className="size-5" />
                </Drawer.Close>
            </header>
            <Drawer.Content className="min-h-0 flex-1 overflow-y-auto overscroll-contain px-6 pb-6">
                <ScreenTransition active={!screen} focusOnMount={false} instant={sheet.instant}>
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
                </ScreenTransition>
                {stack.map((entry, index) => (
                    <ScreenTransition active={index === stack.length - 1} focusOnMount instant={sheet.instant} key={entry.id}>
                        <ScreenView screen={entry.screen} sheet={sheet} />
                    </ScreenTransition>
                ))}
            </Drawer.Content>
        </div>
    );
}

function screenTitle(screen: Screen) {
    if (screen.kind === 'feed') return sourceTitle(screen.candidate);
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

function ScreenView({ screen, sheet }: { screen: Screen; sheet: Sheet }) {
    if (screen.kind === 'feed') return <FeedScreen candidate={screen.candidate} known={screen.rkey} sheet={sheet} site={screen.site} />;
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
                        {profile.about?.[0] ? (
                            <span className="mt-2 line-clamp-2 block text-sm text-pretty text-muted-foreground">{profile.about[0]}</span>
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

function FeedScreen({ candidate, site, known, sheet }: { candidate: SourceCandidate; site?: SiteResult; known?: string; sheet: Sheet }) {
    const status = candidateStatus(candidate, site, sheet.subscriptions, known);
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

    function save(options: SourceOptions) {
        void run(async () => {
            const { rkey } = await addSource(candidate, options);
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

    const errorLine = error ? (
        <p className="text-sm text-destructive" role="alert">
            {error.message} {error.reauth ? <Link href={PATHS.login}>Sign in again</Link> : null}
        </p>
    ) : null;

    return (
        <div className="space-y-8">
            <p className="truncate text-xs text-subtle-foreground" title={sourceLocation(candidate)}>{feedMeta(candidate)}</p>
            <PostPreview candidate={candidate} disabled={sheet.saving} onOpen={(post) => sheet.push({ kind: 'post', candidate, post })} />
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
                <AddSourceForm errorLine={errorLine} onSave={save} saving={sheet.saving} />
            )}
        </div>
    );
}

function AddSourceForm({ saving, errorLine, onSave }: { saving: boolean; errorLine: ReactNode; onSave: (options: SourceOptions) => void }) {
    const tagInput = useRef<HTMLInputElement>(null);
    const [tags, setTags] = useState<string[]>([]);
    const [draft, setDraft] = useState('');
    const [primary, setPrimary] = useState(false);
    const overlong = hasOverlongTag(draft);
    return (
        <form
            className="space-y-8"
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
            <PrimarySwitch checked={primary} disabled={saving} onChange={setPrimary} />
            {errorLine}
            <SheetAction busyLabel="Adding source" disabled={overlong} saving={saving} type="submit">
                Add to sources
            </SheetAction>
        </form>
    );
}

function SheetAction({ saving, busyLabel, disabled = false, children, ...props }: Omit<ComponentProps<typeof Button>, 'disabled'> & {
    saving: boolean;
    busyLabel: string;
    disabled?: boolean;
}) {
    return (
        <div className="sticky bottom-0 -mx-6 bg-popover px-6 pt-2 pb-1">
            <Button aria-busy={saving} className="w-full" disabled={saving || disabled} size="lg" {...props}>
                {saving ? (
                    <>
                        <LoadingIcon className="motion-safe:animate-spin" />
                        <span className="sr-only">{busyLabel}</span>
                    </>
                ) : children}
            </Button>
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
                    <p className="pt-3 text-sm text-muted-foreground">Recent posts couldn’t be loaded. You can still add this feed.</p>
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

function PrimarySwitch({ checked, disabled, onChange }: { checked: boolean; disabled: boolean; onChange: (checked: boolean) => void }) {
    const id = useId();
    return (
        <div className="flex items-center justify-between gap-4">
            <label className="cursor-pointer" htmlFor={id}>
                <span className="block text-sm font-medium">Primary</span>
                <span className="block text-xs text-muted-foreground">Shown first in your newspaper.</span>
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
    return (
        <div className="space-y-8">
            <div className="flex items-center gap-4">
                <PersonAvatar className="size-14" seed={siteHost(site.url)} />
                <div className="min-w-0">
                    <p className="truncate text-xl font-medium">{name}</p>
                    <p className="truncate text-sm text-subtle-foreground">{displayURL(site.url)}</p>
                </div>
            </div>
            {profile.about?.length ? (
                <div className="space-y-3 text-sm text-pretty">
                    {profile.about.map((paragraph, index) => <p key={index}>{paragraph}</p>)}
                </div>
            ) : null}
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
