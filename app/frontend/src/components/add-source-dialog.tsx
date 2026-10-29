import { Dialog } from '@base-ui/react/dialog';
import {
    AnimatePresence,
    LayoutGroup,
    motion,
    useReducedMotion,
} from 'motion/react';
import {
    useEffect,
    useId,
    useRef,
    useState,
    type FormEvent,
    type MouseEvent,
    type ReactElement,
    type ReactNode,
    type RefObject,
} from 'react';
import { toast } from 'sonner';

import {
    CheckIcon,
    CloseIcon,
    LoadingIcon,
    PlusIcon,
} from '@/components/icons';
import { GenerateFeed } from '@/components/generate-feed';
import { IconCrossfade } from '@/components/icon-crossfade';
import { NewsletterAddress } from '@/components/newsletter-address';
import { Button } from '@/components/ui/button';
import { DialogBackdrop, DialogPopup } from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { Link } from '@/components/ui/link';
import {
    discoveryError,
    discoverSources,
    saveSources,
    sourceKey,
    sourceLocation,
    sourceTitle,
    type SourceCandidate,
    type SourceDiscovery,
} from '@/lib/add-source';
import { classifyMutationError, describeMutationError } from '@/lib/api';
import { PATHS } from '@/lib/paths';
import { cn } from '@/lib/utils';

/* Discovery storyboard: the input stays anchored while attention changes below it.
 *   0ms  request starts; newsletter exits over 140ms
 * 140ms  loading/results enter over 200ms; network readiness owns the next stage
 * toggle  plus ↔ green check, IconCrossfade's blur swap; Save owns the write
 */
const TIMING = { exit: 0.14, enter: 0.2 };
const PANEL = {
    offset: 'translateY(4px)',
    rest: 'translateY(0px)',
    ease: [0.16, 1, 0.3, 1] as const,
    resize: { type: 'spring', duration: 0.24, bounce: 0 } as const,
};

export function AddSourceDialog({ trigger }: { trigger?: ReactElement }) {
    const inputRef = useRef<HTMLInputElement>(null);
    const [open, setOpen] = useState(false);
    const [saving, setSaving] = useState(false);
    const reduced = useReducedMotion();
    return (
        <Dialog.Root
            open={open}
            onOpenChange={(next, details) => {
                if (saving) details.cancel();
                else setOpen(next);
            }}
        >
            {trigger ? <Dialog.Trigger render={trigger} /> : (
                <Dialog.Trigger
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
                </Dialog.Trigger>
            )}
            <Dialog.Portal>
                <DialogBackdrop />
                <LayoutGroup>
                    <DialogPopup
                        render={
                            <motion.div
                                layout={!reduced}
                                transition={{ layout: PANEL.resize }}
                            />
                        }
                        initialFocus={(interaction) =>
                            interaction === 'touch' ? true : inputRef.current
                        }
                        className="max-w-xl sm:p-8"
                    >
                        <motion.div
                            className="relative"
                            layout={reduced ? false : 'position'}
                        >
                            <Dialog.Close
                                render={
                                    <Button
                                        aria-label="Close dialog"
                                        disabled={saving}
                                        className="absolute -top-4 -right-4 text-muted-foreground"
                                        size="icon-lg"
                                        variant="ghost"
                                    />
                                }
                            >
                                <CloseIcon className="size-5" />
                            </Dialog.Close>
                            <Dialog.Title className="pr-10 text-xl font-medium">
                                Add a source
                            </Dialog.Title>
                            <Dialog.Description className="mt-1 mb-7 pr-6 text-sm text-muted-foreground">
                                Choose what finds its way into your newspaper.
                            </Dialog.Description>
                            <FeedDiscovery
                                inputRef={inputRef}
                                saving={saving}
                                onSavingChange={setSaving}
                                onClose={() => setOpen(false)}
                            />
                        </motion.div>
                    </DialogPopup>
                </LayoutGroup>
            </Dialog.Portal>
        </Dialog.Root>
    );
}

type DiscoveryState =
    | { stage: 0 }
    | { stage: 1 }
    | { stage: 2; result: SourceDiscovery; url: string }
    | { stage: 3; error: string };

function FeedDiscovery({
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
    const [url, setUrl] = useState('');
    const [state, setState] = useState<DiscoveryState>({ stage: 0 });
    const [keyboard, setKeyboard] = useState(false);
    const reduced = useReducedMotion();
    const instant = reduced || keyboard;
    const request = useRef<AbortController | null>(null);
    useEffect(() => () => request.current?.abort(), []);

    async function discover(event: FormEvent) {
        event.preventDefault();
        if (saving || state.stage === 1) return;
        request.current?.abort();
        const controller = new AbortController();
        request.current = controller;
        setState({ stage: 1 });
        const next = await discoverSources(url, controller.signal).then(
            (result): DiscoveryState => ({ stage: 2, result, url }),
            (error): DiscoveryState => ({
                stage: 3,
                error: discoveryError(error),
            }),
        );
        if (!controller.signal.aborted) setState(next);
    }

    function reset() {
        request.current?.abort();
        setState({ stage: 0 });
    }

    return (
        <>
            <DiscoveryForm
                inputRef={inputRef}
                stage={state.stage}
                saving={saving}
                url={url}
                onSubmit={discover}
                onKeyboardChange={setKeyboard}
                onChange={(value) => {
                    reset();
                    setUrl(value);
                }}
            />
            <div className="mt-6">
                <DiscoveryPanel
                    instant={instant}
                    panelKey={state.stage === 0 ? 'newsletter' : 'discovery'}
                >
                    {state.stage === 0 ? (
                        <NewsletterAddress onNavigate={onClose} />
                    ) : (
                        <div>
                            <p className="sr-only" role="status">
                                {foundFeeds(state)}
                            </p>
                            <DiscoveryStatus
                                state={state}
                                instant={instant}
                                saving={saving}
                                onSavingChange={onSavingChange}
                                onSaved={onClose}
                            />
                            <Button
                                className="mt-3 -ml-2.5 text-muted-foreground"
                                disabled={saving}
                                onClick={() => {
                                    reset();
                                    inputRef.current?.focus();
                                }}
                                size="sm"
                                variant="ghost"
                            >
                                Add a newsletter instead
                            </Button>
                        </div>
                    )}
                </DiscoveryPanel>
            </div>
        </>
    );
}

function foundFeeds(state: DiscoveryState) {
    if (state.stage !== 2) return '';
    const count = state.result.candidates.length;
    if (count === 0) return 'No feed found. You can generate one.';
    return count === 1 ? '1 feed found.' : `${count} feeds found.`;
}

function DiscoveryForm({
    inputRef,
    stage,
    saving,
    url,
    onSubmit,
    onChange,
    onKeyboardChange,
}: {
    inputRef: RefObject<HTMLInputElement | null>;
    stage: DiscoveryState['stage'];
    saving: boolean;
    url: string;
    onSubmit: (event: FormEvent) => void;
    onChange: (value: string) => void;
    onKeyboardChange: (keyboard: boolean) => void;
}) {
    const loading = stage === 1;
    const invalid = stage === 3;
    return (
        <section aria-labelledby="feed-heading">
            <h2 className="font-medium" id="feed-heading">
                From a website
            </h2>
            <p className="mt-1 text-sm text-muted-foreground">
                Find feeds from a site, blog, or YouTube channel.
            </p>
            <form
                className="mt-4"
                onSubmit={onSubmit}
                onKeyDown={(event) => {
                    if (event.key === 'Enter') onKeyboardChange(true);
                }}
                onPointerDown={() => onKeyboardChange(false)}
            >
                <label className="sr-only" htmlFor="source-url">
                    Website or feed URL
                </label>
                <div className="flex flex-col gap-2 sm:flex-row">
                    <Input
                        aria-describedby={
                            invalid ? 'discovery-error' : undefined
                        }
                        aria-invalid={invalid}
                        autoCapitalize="none"
                        autoComplete="url"
                        id="source-url"
                        inputMode="url"
                        onChange={(event) => onChange(event.target.value)}
                        placeholder="example.com"
                        ref={inputRef}
                        required
                        spellCheck={false}
                        disabled={saving}
                        value={url}
                    />
                    <FindFeedsButton
                        loading={loading}
                        disabled={saving || loading || !url.trim()}
                    />
                </div>
            </form>
        </section>
    );
}

function FindFeedsButton({
    loading,
    disabled,
}: {
    loading: boolean;
    disabled: boolean;
}) {
    return (
        <Button
            aria-label="Find feeds"
            aria-busy={loading}
            className="sm:w-28"
            disabled={disabled}
            type="submit"
        >
            {loading ? (
                <LoadingIcon className="size-4 motion-safe:animate-spin" />
            ) : (
                'Find feeds'
            )}
        </Button>
    );
}

function DiscoveryPanel({
    instant,
    panelKey,
    children,
}: {
    instant: boolean;
    panelKey: string;
    children: ReactNode;
}) {
    const timing = instant ? { exit: 0, enter: 0 } : TIMING;
    return (
        <AnimatePresence initial={false} mode="wait">
            <motion.div
                layout={instant ? false : 'position'}
                key={panelKey}
                initial={{
                    opacity: 0,
                    transform: instant ? PANEL.rest : PANEL.offset,
                }}
                animate={{ opacity: 1, transform: PANEL.rest }}
                exit={{ opacity: 0, transition: { duration: timing.exit } }}
                transition={{ duration: timing.enter, ease: PANEL.ease }}
            >
                {children}
            </motion.div>
        </AnimatePresence>
    );
}

function DiscoveryStatus({
    state,
    instant,
    ...actions
}: {
    state: DiscoveryState;
    instant: boolean;
    saving: boolean;
    onSavingChange: (saving: boolean) => void;
    onSaved: () => void;
}) {
    if (state.stage === 1)
        return (
            <p
                className="flex h-36 items-center justify-center text-muted-foreground"
                role="status"
            >
                <LoadingIcon className="size-5 motion-safe:animate-spin" />
                <span className="sr-only">Looking for feeds</span>
            </p>
        );
    if (state.stage === 3)
        return (
            <p
                className="rounded-lg bg-background p-4 text-sm text-destructive"
                id="discovery-error"
                role="alert"
            >
                {state.error}
            </p>
        );
    if (state.stage === 2)
        return (
            <ResolvedSources
                result={state.result}
                url={state.url}
                instant={instant}
                {...actions}
            />
        );
    return null;
}

function ResolvedSources({
    result,
    url,
    instant,
    ...actions
}: {
    result: SourceDiscovery;
    url: string;
    instant: boolean;
    saving: boolean;
    onSavingChange: (saving: boolean) => void;
    onSaved: () => void;
}) {
    return (
        <motion.div
            initial={{ opacity: instant ? 1 : 0 }}
            animate={{ opacity: 1 }}
            transition={{ duration: instant ? 0 : TIMING.enter }}
        >
            {result.candidates.length === 0 ? (
                <GenerateFeed url={url} instant={instant} {...actions} />
            ) : (
                <FeedResults result={result} {...actions} />
            )}
        </motion.div>
    );
}

function FeedResults({
    result,
    saving,
    onSavingChange,
    onSaved,
}: {
    result: SourceDiscovery;
    saving: boolean;
    onSavingChange: (saving: boolean) => void;
    onSaved: () => void;
}) {
    const [selected, setSelected] = useState<SourceCandidate[]>([]);
    const [saved, setSaved] = useState<Set<string>>(new Set());
    const [error, setError] = useState<{
        message: string;
        reauth: boolean;
    } | null>(null);
    const locked = saving || Boolean(error);
    const canSave = !locked && selected.length > 0;

    function toggle(candidate: SourceCandidate) {
        if (selected.includes(candidate)) {
            setSelected(selected.filter((item) => item !== candidate));
        } else {
            // Resolve returns one site's alternatives; native and RSS must not both be followed.
            setSelected([
                ...selected.filter(
                    (item) =>
                        (item.kind === 'standardfeed') ===
                        (candidate.kind === 'standardfeed'),
                ),
                candidate,
            ]);
        }
    }

    async function save() {
        if (!canSave) return;
        onSavingChange(true);
        try {
            await saveSources(selected, (candidate) => {
                setSaved(
                    (previous) => new Set([...previous, sourceKey(candidate)]),
                );
                setSelected((previous) =>
                    previous.filter((item) => item !== candidate),
                );
            });
            toast.success(
                selected.length === 1 ? 'Source added' : 'Sources added',
                {
                    description: 'Their latest entries are being collected.',
                },
            );
            onSaved();
        } catch (error) {
            setError({
                message: describeMutationError(
                    error,
                    'The save couldn’t be confirmed.',
                ),
                reauth: classifyMutationError(error) === 'reauth',
            });
        } finally {
            onSavingChange(false);
        }
    }

    return (
        <>
            <p
                className="mb-2 text-xs text-muted-foreground"
                id="available-feeds"
            >
                Available feeds
            </p>
            <FeedList>
                {result.candidates.map((candidate) => {
                    const key = sourceKey(candidate);
                    const existing =
                        saved.has(key) ||
                        result.existingSubscriptions.some(
                            (sub) => sub.feedUrl === key,
                        );
                    return (
                        <FeedCandidate
                            candidate={candidate}
                            existing={existing}
                            selected={selected.includes(candidate)}
                            disabled={
                                locked ||
                                existing ||
                                Boolean(candidate.subscribedVia)
                            }
                            onToggle={() => toggle(candidate)}
                            key={key}
                        />
                    );
                })}
            </FeedList>
            <SaveError error={error} />
            <SaveActions
                saving={saving}
                canSave={canSave}
                count={selected.length}
                onSave={save}
            />
        </>
    );
}

function FeedList({ children }: { children: ReactNode }) {
    const [edges, setEdges] = useState({ top: false, bottom: false });
    const listRef = useRef<HTMLUListElement>(null);
    useEffect(() => {
        const list = listRef.current;
        if (!list) return;
        const update = () => {
            const top = list.scrollTop > 1;
            const bottom = list.scrollTop + list.clientHeight < list.scrollHeight - 1;
            setEdges((current) =>
                current.top === top && current.bottom === bottom ? current : { top, bottom },
            );
        };
        const observer = new ResizeObserver(update);
        observer.observe(list);
        update();
        list.addEventListener('scroll', update, { passive: true });
        return () => {
            observer.disconnect();
            list.removeEventListener('scroll', update);
        };
    }, []);
    return (
        <div className="relative">
            <ul
                aria-labelledby="available-feeds"
                className="max-h-76 space-y-2 overflow-y-auto overscroll-contain rounded-lg [scrollbar-width:thin]"
                ref={listRef}
                tabIndex={0}
            >
                {children}
            </ul>
            {edges.top && (
                <div
                    aria-hidden="true"
                    className="pointer-events-none absolute inset-x-0 top-0 h-10 rounded-t-lg bg-gradient-to-b from-popover to-transparent"
                />
            )}
            {edges.bottom && (
                <div
                    aria-hidden="true"
                    className="pointer-events-none absolute inset-x-0 bottom-0 h-10 rounded-b-lg bg-gradient-to-t from-popover to-transparent"
                />
            )}
        </div>
    );
}

function SaveError({
    error,
}: {
    error: { message: string; reauth: boolean } | null;
}) {
    if (!error) return null;
    return (
        <p className="mt-3 text-sm text-destructive" role="alert">
            {error.message} Confirmed additions are kept. Find feeds again to
            check which sources were saved before retrying.{' '}
            {error.reauth && <Link href={PATHS.login}>Sign in again</Link>}
        </p>
    );
}

function SaveActions({
    saving,
    canSave,
    count,
    onSave,
}: {
    saving: boolean;
    canSave: boolean;
    count: number;
    onSave: () => void;
}) {
    return (
        <div className="mt-4 flex items-center justify-between gap-3">
            <p className="text-xs text-muted-foreground" aria-live="polite">
                {saving
                    ? 'Keep this dialog open while saving.'
                    : `${count} selected`}
            </p>
            <Button aria-busy={saving} disabled={!canSave} onClick={onSave}>
                {saving ? (
                    <>
                        <LoadingIcon className="motion-safe:animate-spin" />
                        <span className="sr-only">Saving sources</span>
                    </>
                ) : (
                    'Save'
                )}
            </Button>
        </div>
    );
}

function FeedCandidate({
    candidate,
    existing,
    selected,
    disabled,
    onToggle,
}: {
    candidate: SourceCandidate;
    existing: boolean;
    selected: boolean;
    disabled: boolean;
    onToggle: () => void;
}) {
    const followedViaId = useId();
    const title = sourceTitle(candidate);
    const followedVia = existing ? undefined : candidate.subscribedVia;
    return (
        <li className="rounded-lg bg-background px-4">
            <div className="flex h-20 items-center gap-3">
                <FeedDetails
                    candidate={candidate}
                    followedVia={followedVia}
                    followedViaId={followedViaId}
                />
                <FeedSelection
                    title={title}
                    existing={existing}
                    checked={existing || selected}
                    disabled={disabled}
                    descriptionId={followedVia ? followedViaId : undefined}
                    onToggle={onToggle}
                />
            </div>
        </li>
    );
}

function FeedDetails({
    candidate,
    followedVia,
    followedViaId,
}: {
    candidate: SourceCandidate;
    followedVia: SourceCandidate['subscribedVia'];
    followedViaId: string;
}) {
    const title = sourceTitle(candidate);
    const location = sourceLocation(candidate);
    return (
        <div className="min-w-0 flex-1">
            <p className="truncate text-sm font-medium" title={title}>
                {title}
            </p>
            <p
                className="mt-1 truncate text-xs text-subtle-foreground"
                title={location}
            >
                {candidate.kind === 'standardfeed' && 'Standardfeed · '}
                {location}
            </p>
            {followedVia && (
                <FollowedVia kind={followedVia.kind} id={followedViaId} />
            )}
        </div>
    );
}

function FollowedVia({ kind, id }: { kind: string; id: string }) {
    return (
        <p className="truncate text-xs text-muted-foreground" id={id}>
            Already followed via{' '}
            {kind === 'standardfeed' ? 'Standardfeed' : 'another feed'}.
        </p>
    );
}

function FeedSelection({
    title,
    existing,
    checked,
    disabled,
    descriptionId,
    onToggle,
}: {
    title: string;
    existing: boolean;
    checked: boolean;
    disabled: boolean;
    descriptionId?: string;
    onToggle: () => void;
}) {
    const [keyboard, setKeyboard] = useState(false);
    const reduced = useReducedMotion();
    const instant = reduced || keyboard;

    function toggle(event: MouseEvent) {
        setKeyboard(event.detail === 0);
        onToggle();
    }

    return (
        <Button
            aria-describedby={descriptionId}
            aria-label={
                existing ? `${title} already followed` : `Select ${title}`
            }
            aria-pressed={checked}
            className={cn('relative disabled:opacity-100', {
                'text-success': checked,
            })}
            disabled={disabled}
            onClick={toggle}
            size="icon-lg"
            variant="ghost"
        >
            <IconCrossfade
                name={checked ? 'check' : 'plus'}
                icon={checked ? CheckIcon : PlusIcon}
                instant={instant}
            />
        </Button>
    );
}
