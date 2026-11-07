import { Dialog } from '@base-ui/react/dialog';
import { Command as Cmdk, defaultFilter } from 'cmdk';
import { useEffect, useRef, useState, useSyncExternalStore, type ReactNode } from 'react';
import { forgetShapes } from 'skullet';
import { toast } from 'sonner';
import { useLocation } from 'wouter';

import { AddSourceDialog } from '@/components/add-source-dialog';
import { CheckIcon, ChevronDownIcon, EnterIcon, LoadingIcon, PlusIcon } from '@/components/icons';
import { SourceIcon } from '@/components/item-list';
import { DialogBackdrop, DialogPopup } from '@/components/ui/dialog';
import { usePublicFollowing } from '@/hooks/use-public-following';
import { useReaderFont } from '@/hooks/use-reader-font';
import { useSources, type FeedSource, type NewsletterSource } from '@/hooks/use-sources';
import { subscriptionChanges } from '@/lib/add-source';
import { api, describeMutationError } from '@/lib/api';
import { AppProfileContext, loadProfile, type AppProfileState } from '@/lib/app-profile-context';
import { ACTIONS, CommandProblem, copied, GO_TO, pageCommands, type Command, type CommandStep } from '@/lib/commands';
import { addCalendarDays, parseDayQuery, startOfToday, toDateKey } from '@/lib/digest-dates';
import { downloadOpml } from '@/lib/opml';
import { APP_MODES, digestHref, newsletterSourceHref, PATHS, sourceHref } from '@/lib/paths';
import { isCommandMenuKey, SHORTCUTS } from '@/lib/shortcuts';
import { displayURL } from '@/lib/source-display';
import { addTags, GONE, hasOverlongTag, isGone, MAX_TAG_LENGTH, MAX_TAGS } from '@/lib/source-edit';
import { cn } from '@/lib/utils';

const FAILED = 'That didn’t work. Try again.';

export function CommandMenu() {
    const [open, setOpen] = useState(false);
    const [step, setStep] = useState<CommandStep | null>(null);
    const [pending, setPending] = useState(false);
    const [adding, setAdding] = useState<{ url: string; key: number } | null>(null);
    const [location] = useLocation();
    const popup = useRef<HTMLDivElement>(null);
    // ⌘K has no trigger to return focus to, so closing returns it to whatever had it before the menu opened.
    const opener = useRef<HTMLElement | null>(null);
    const enabled = location !== PATHS.login;

    useEffect(() => {
        if (!enabled) return;
        function onKeyDown(event: KeyboardEvent) {
            if (!isCommandMenuKey(event)) return;
            // Another open dialog keeps its keys, so the menu never stacks over a sheet.
            const target = event.target instanceof Element ? event.target : null;
            if (target?.closest('[role="dialog"], [role="alertdialog"]') && !popup.current?.contains(target)) return;
            event.preventDefault();
            if (!popup.current?.contains(document.activeElement)) opener.current = document.activeElement instanceof HTMLElement ? document.activeElement : null;
            setStep(null);
            setOpen((value) => !value);
        }
        window.addEventListener('keydown', onKeyDown);
        return () => window.removeEventListener('keydown', onKeyDown);
    }, [enabled]);

    return (
        <>
            <Dialog.Root
                onOpenChange={(next, details) => {
                    if (pending) {
                        details.cancel();
                        return;
                    }
                    // Escape inside a step steps back out of it; only Escape at the top closes the menu.
                    if (!next && step && details.reason === 'escape-key') {
                        details.cancel();
                        setStep(null);
                        return;
                    }
                    setOpen(next);
                }}
                open={open && enabled}
            >
                <Dialog.Portal>
                    {/* A keyboard palette opened many times a day appears at once, without the dialogs' entrance. */}
                    <DialogBackdrop className="transition-none" />
                    <DialogPopup
                        className="top-[min(16dvh,8rem)] max-w-xl translate-y-0 overflow-hidden p-0 transition-none"
                        finalFocus={() => (opener.current?.isConnected ? opener.current : true)}
                        ref={popup}
                    >
                        <Dialog.Title className="sr-only">Command menu</Dialog.Title>
                        <MenuBody
                            onAddSource={(url) => {
                                setOpen(false);
                                setAdding((previous) => ({ url, key: (previous?.key ?? 0) + 1 }));
                            }}
                            onClose={() => setOpen(false)}
                            onPending={setPending}
                            onStep={setStep}
                            step={step}
                        />
                    </DialogPopup>
                </Dialog.Portal>
            </Dialog.Root>
            {adding ? <AddSourceSheet initialUrl={adding.url} key={adding.key} /> : null}
        </>
    );
}

// The menu lives outside AppShell, on the reader too, so its add sheet loads the profile its newsletter address waits on.
function AddSourceSheet({ initialUrl }: { initialUrl: string }) {
    const [profile, setProfile] = useState<AppProfileState>({ kind: 'loading' });
    useEffect(() => {
        const controller = new AbortController();
        void loadProfile(controller.signal).then((next) => {
            if (next) setProfile(next);
        });
        return () => controller.abort();
    }, []);
    return (
        <AppProfileContext.Provider value={profile}>
            <AddSourceDialog initialUrl={initialUrl} />
        </AppProfileContext.Provider>
    );
}

type MenuProps = {
    onClose: () => void;
    onPending: (pending: boolean) => void;
    onStep: (step: CommandStep | null) => void;
};

function MenuBody({ step, onAddSource, ...props }: MenuProps & { step: CommandStep | null; onAddSource: (url: string) => void }) {
    const registered = useSyncExternalStore(pageCommands.subscribe, pageCommands.get);
    const global = useGlobalCommands(onAddSource);
    // A flipped toggle shows its new state at once; the page catches up when its data reloads.
    const [flipped, setFlipped] = useState<Map<string, boolean>>(new Map());
    const run = useRun(props);

    const onStep = (next: CommandStep | null) => {
        if (run.busy()) return;
        run.fail('');
        props.onStep(next);
    };
    if (step) return <StepView key={step.title} {...props} onStep={onStep} run={run} step={step} />;
    return (
        <RootView
            commands={[...global, ...registered]}
            flipped={flipped}
            onAddSource={onAddSource}
            onFlip={(id, on) => setFlipped((previous) => {
                const next = new Map(previous);
                if (on === undefined) next.delete(id);
                else next.set(id, on);
                return next;
            })}
            {...props}
            onStep={onStep}
            run={run}
        />
    );
}

type Outcome = Promise<string | void> | string | void;
type Run = ReturnType<typeof useRun>;

function useRun({ onClose, onPending }: MenuProps) {
    const [state, setState] = useState<{ pending: boolean; error: string }>({ pending: false, error: '' });
    // A second Enter can land before the pending render, so a ref, not state, keeps a command from running twice.
    const inFlight = useRef(false);
    async function run(work: () => Outcome | Promise<unknown>, { close = true } = {}) {
        if (inFlight.current) return false;
        inFlight.current = true;
        setState({ pending: true, error: '' });
        onPending(true);
        try {
            const message = await work();
            setState({ pending: false, error: '' });
            if (close) {
                onClose();
                if (typeof message === 'string') toast.success(message);
            }
            return true;
        } catch (failure) {
            setState({ pending: false, error: problemText(failure, FAILED) });
            return false;
        } finally {
            inFlight.current = false;
            onPending(false);
        }
    }
    return { ...state, run, busy: () => inFlight.current, fail: (error: string) => setState((previous) => ({ ...previous, error })) };
}

function problemText(failure: unknown, fallback: string): string {
    if (isGone(failure)) return GONE;
    if (failure instanceof CommandProblem) return failure.message;
    return describeMutationError(failure, fallback);
}

// A row read from the query itself renders only when it applies, so it always leads.
const FROM_QUERY = new Set(['create-tag']);

// Matching reads the keywords, never the row's id. cmdk's fuzzy score lets letters scattered across a title match; those score
// under 0.05, while word starts and substrings score far higher, so the scattered ones are dropped.
function filter(value: string, search: string, keywords?: string[]) {
    if (FROM_QUERY.has(value)) return 1;
    const score = defaultFilter(keywords?.join(' ') ?? '', search);
    return score < 0.05 ? 0 : score;
}

function Shell({ query, onQuery, chip, placeholder, shouldFilter = true, error, onBackspace, children }: {
    query: string;
    onQuery: (query: string) => void;
    chip?: string;
    placeholder: string;
    shouldFilter?: boolean;
    error: string;
    onBackspace?: () => void;
    children: ReactNode;
}) {
    const input = useRef<HTMLInputElement>(null);
    // A step opens on its current value, selected so typing replaces it.
    useEffect(() => input.current?.select(), []);
    return (
        <Cmdk filter={filter} label="Command menu" loop shouldFilter={shouldFilter}>
            <div className="flex h-14 items-center gap-2 border-b border-border px-5">
                {chip ? (
                    <span className="flex shrink-0 items-center gap-1.5 text-muted-foreground motion-safe:animate-in motion-safe:fade-in-0 motion-safe:slide-in-from-left-1 motion-safe:duration-(--motion-duration-ui)">
                        {chip}
                        <ChevronDownIcon aria-hidden="true" className="size-3.5 -rotate-90 text-subtle-foreground" />
                    </span>
                ) : null}
                <Cmdk.Input
                    aria-label={chip ?? 'Search commands'}
                    autoFocus
                    className="h-full min-w-0 flex-1 bg-transparent text-base outline-none placeholder:text-subtle-foreground"
                    onKeyDown={(event) => {
                        if (event.key === 'Backspace' && !query && onBackspace) {
                            event.preventDefault();
                            onBackspace();
                        }
                    }}
                    onValueChange={onQuery}
                    placeholder={placeholder}
                    ref={input}
                    value={query}
                />
            </div>
            {error ? <p className="px-5 pt-3 text-sm text-destructive" role="alert">{error}</p> : null}
            <Cmdk.List className="max-h-[min(26rem,60dvh)] scroll-py-2 overflow-y-auto overscroll-contain p-2">
                {children}
            </Cmdk.List>
        </Cmdk>
    );
}

const GROUP = 'px-0 [&_[cmdk-group-heading]]:px-3 [&_[cmdk-group-heading]]:pt-3 [&_[cmdk-group-heading]]:pb-1.5 [&_[cmdk-group-heading]]:text-xs [&_[cmdk-group-heading]]:font-medium [&_[cmdk-group-heading]]:text-subtle-foreground';

function Row({ id, keywords, onSelect, disabled, destructive, children }: {
    id: string;
    keywords?: string[];
    onSelect: () => void;
    disabled?: boolean;
    destructive?: boolean;
    children: ReactNode;
}) {
    return (
        <Cmdk.Item
            className={cn(
                'flex min-h-10 cursor-default items-center gap-3 rounded-md px-3 text-sm select-none data-[disabled=true]:opacity-50 data-[selected=true]:bg-secondary',
                destructive && 'text-destructive',
            )}
            disabled={disabled}
            keywords={keywords}
            onSelect={onSelect}
            value={id}
        >
            {children}
        </Cmdk.Item>
    );
}

function Label({ children, detail }: { children: ReactNode; detail?: string }) {
    return (
        <span className="flex min-w-0 flex-1 items-baseline gap-2">
            <span className="truncate">{children}</span>
            {detail ? <span className="min-w-0 truncate text-xs text-subtle-foreground">{detail}</span> : null}
        </span>
    );
}

const APPLE = typeof navigator !== 'undefined' && /Mac|iPhone|iPad/.test(navigator.userAgent);
const KEY_NAMES: Record<string, string> = {
    Mod: APPLE ? '⌘' : 'Ctrl',
    Shift: '⇧',
    ArrowLeft: '←',
    ArrowRight: '→',
    ArrowUp: '↑',
    ArrowDown: '↓',
    Escape: 'Esc',
};

function Keys({ keys }: { keys: string }) {
    return (
        <kbd className="shrink-0 rounded-sm px-1.5 font-sans text-xs text-subtle-foreground shadow-[inset_0_0_0_1px_var(--border)]">
            {keys.split('+').map((key) => KEY_NAMES[key] ?? key.toUpperCase()).join(APPLE ? '' : ' ')}
        </kbd>
    );
}

function Toggle({ on }: { on: boolean }) {
    return (
        <span aria-hidden="true" className={cn('flex h-4 w-7 shrink-0 rounded-full p-0.5 transition-[background-color] duration-(--motion-duration-ui) ease-out', on ? 'bg-primary' : 'bg-accent')}>
            <span className={cn('size-3 rounded-full bg-popover transition-transform duration-(--motion-duration-ui) ease-(--ease-out)', on && 'translate-x-3')} />
        </span>
    );
}

const Busy = () => <LoadingIcon aria-label="Working" className="size-4 shrink-0 text-subtle-foreground motion-safe:animate-spin" />;

const dayLabel = new Intl.DateTimeFormat(undefined, { weekday: 'long', day: 'numeric', month: 'long' });
const dayWithYear = new Intl.DateTimeFormat(undefined, { weekday: 'long', day: 'numeric', month: 'long', year: 'numeric' });

function formatDay(day: Date, today: Date): string {
    const ago = Math.round((today.getTime() - day.getTime()) / 86_400_000);
    if (ago === 0) return 'Today';
    if (ago === 1) return 'Yesterday';
    return (day.getFullYear() === today.getFullYear() ? dayLabel : dayWithYear).format(day);
}

const URL_LIKE = /^(?:https?:\/\/\S+|(?:[\w-]+\.)+[a-z][a-z-]+(?::\d+)?(?:[/?#]\S*)?)$/i;

function pastedSite(query: string): string | null {
    const value = query.trim();
    if (!URL_LIKE.test(value)) return null;
    try {
        return new URL(/^https?:\/\//i.test(value) ? value : `https://${value}`).host;
    } catch {
        return null;
    }
}

function RootView({ commands, flipped, onFlip, run, onStep, onAddSource }: MenuProps & {
    commands: Command[];
    flipped: Map<string, boolean>;
    onFlip: (id: string, on: boolean | undefined) => void;
    run: Run;
    onAddSource: (url: string) => void;
}) {
    const [query, setQuery] = useState('');
    const [busy, setBusy] = useState<string | null>(null);
    const [, navigate] = useLocation();
    const today = startOfToday();
    const day = parseDayQuery(query, today);
    const site = pastedSite(query);

    function choose(command: Command) {
        if (run.busy()) return;
        if ('step' in command) return onStep(command.step);
        setBusy(command.id);
        if ('toggle' in command) {
            const on = !(flipped.get(command.id) ?? command.toggle.on);
            onFlip(command.id, on);
            void run.run(() => command.toggle.set(on), { close: false }).then((done) => {
                if (!done) onFlip(command.id, undefined);
                setBusy(null);
            });
            return;
        }
        void run.run(command.run).then(() => setBusy(null));
    }

    const groups = rankedGroups(commands, query);

    return (
        <Shell error={run.error} onQuery={setQuery} placeholder="Type a command, source or day" query={query} shouldFilter={false}>
            {day ? (
                <Row id="typed-day" onSelect={() => void run.run(() => navigate(digestHref(toDateKey(day) === toDateKey(today) ? undefined : toDateKey(day))))}>
                    <Label detail="Digest">Go to {formatDay(day, today)}</Label>
                </Row>
            ) : null}
            {site ? (
                <Row id="pasted-site" onSelect={() => onAddSource(query.trim())}>
                    <PlusIcon aria-hidden="true" className="size-4 shrink-0 text-subtle-foreground" />
                    <Label>Add {site} as a source</Label>
                </Row>
            ) : null}
            {groups.map(({ group, commands: listed }) => (
                <Cmdk.Group className={GROUP} heading={group} key={group}>
                    {listed.map((command) => (
                        <Row
                            disabled={run.pending && busy !== command.id}
                            id={command.id}
                            key={command.id}
                            onSelect={() => choose(command)}
                        >
                            {command.icon ? <SourceIcon src={command.icon} /> : null}
                            <Label detail={command.detail}>{command.label}</Label>
                            {busy === command.id && run.pending ? <Busy /> : <RowEnd command={command} on={flipped.get(command.id)} />}
                        </Row>
                    ))}
                </Cmdk.Group>
            ))}
            <Cmdk.Empty className="px-3 py-6 text-center text-sm text-muted-foreground">Nothing matches “{query.trim()}”.</Cmdk.Empty>
        </Shell>
    );
}

// The top level ranks its own rows: cmdk re-sorts groups only on a keystroke, so a group that mounts mid-search, such as
// the sources, would keep its mount position under a weaker match.
function rankedGroups(commands: Command[], query: string) {
    const searching = query.trim() !== '';
    const groups = new Map<string, { command: Command; score: number }[]>();
    for (const command of commands) {
        if (command.searchOnly && !searching) continue;
        const score = searching ? filter(command.id, query, [command.label, ...(command.keywords ?? []), command.detail ?? '']) : 1;
        if (score > 0) groups.set(command.group, [...(groups.get(command.group) ?? []), { command, score }]);
    }
    // Without a query the page's own commands come first, then navigation, then actions; with one, the best match leads.
    const rank = (group: string) => (group === GO_TO ? 1 : group === ACTIONS ? 2 : 0);
    const best = (group: string) => Math.max(...groups.get(group)!.map(({ score }) => score));
    return [...groups.keys()]
        .sort((a, b) => (searching ? best(b) - best(a) : 0) || rank(a) - rank(b))
        .map((group) => ({
            group,
            commands: groups.get(group)!.sort((a, b) => b.score - a.score).map(({ command }) => command),
        }));
}

function RowEnd({ command, on }: { command: Command; on?: boolean }) {
    if ('toggle' in command) return <><span className="sr-only">{(on ?? command.toggle.on) ? 'On' : 'Off'}</span><Toggle on={on ?? command.toggle.on} /></>;
    return (
        <>
            {command.shortcut ? <Keys keys={command.shortcut} /> : null}
            {'step' in command ? <ChevronDownIcon aria-hidden="true" className="size-3.5 shrink-0 -rotate-90 text-subtle-foreground" /> : null}
        </>
    );
}

function StepView({ step, run, onStep }: MenuProps & { step: CommandStep; run: Run }) {
    const back = () => onStep(null);
    if (step.kind === 'text') return <TextStep back={back} run={run} step={step} />;
    if (step.kind === 'choice') return <ChoiceStep back={back} run={run} step={step} />;
    if (step.kind === 'tags') return <TagsStep back={back} run={run} step={step} />;
    return <ConfirmStep back={back} run={run} step={step} />;
}

type StepProps<K extends CommandStep['kind']> = { step: Extract<CommandStep, { kind: K }>; run: Run; back: () => void };

function TextStep({ step, run, back }: StepProps<'text'>) {
    const [value, setValue] = useState(step.initial);
    const problem = step.problem?.(value) ?? null;
    return (
        <Shell chip={step.title} error={run.error || problem || ''} onBackspace={back} onQuery={setValue} placeholder={step.placeholder} query={value} shouldFilter={false}>
            <Row disabled={Boolean(problem) || run.pending} id="submit" onSelect={() => void run.run(() => step.submit(value))}>
                <Label>{step.action(value)}</Label>
                {run.pending ? <Busy /> : <EnterIcon aria-hidden="true" className="size-4 shrink-0 text-subtle-foreground" />}
            </Row>
        </Shell>
    );
}

function ChoiceStep({ step, run, back }: StepProps<'choice'>) {
    const [query, setQuery] = useState('');
    const [busy, setBusy] = useState<string | null>(null);
    return (
        <Shell chip={step.title} error={run.error} onBackspace={back} onQuery={setQuery} placeholder={step.placeholder} query={query}>
            {step.options.map((option) => (
                <Row
                    disabled={run.pending}
                    id={option.value}
                    key={option.value}
                    keywords={[option.label, option.detail ?? '']}
                    onSelect={() => {
                        setBusy(option.value);
                        void run.run(() => step.submit(option.value));
                    }}
                >
                    <Label detail={option.detail}>{option.label}</Label>
                    {option.shortcut ? <Keys keys={option.shortcut} /> : null}
                    {busy === option.value && run.pending ? <Busy /> : option.value === step.current ? <CheckIcon aria-label="Current" className="size-4 shrink-0 text-primary" /> : null}
                </Row>
            ))}
            <Cmdk.Empty className="px-3 py-6 text-center text-sm text-muted-foreground">Nothing matches “{query.trim()}”.</Cmdk.Empty>
        </Shell>
    );
}

// Each tag change saves on its own, in order, so the list can be edited without leaving the menu.
function TagsStep({ step, run, back }: StepProps<'tags'>) {
    const [query, setQuery] = useState('');
    const { tags, change } = useTagEdits(step, run);
    const offered = useOfferedTags(step.suggestionsPath, tags);
    const draft = query.trim();
    const full = tags.length >= MAX_TAGS;
    const creatable = draft !== '' && !offered.some((tag) => tag.toLowerCase() === draft.toLowerCase());
    const choose = (next: string[]) => {
        setQuery('');
        change(next);
    };
    return (
        <Shell chip={step.title} error={run.error || tagHint(draft, creatable, full)} onBackspace={back} onQuery={setQuery} placeholder={full ? 'Remove a tag' : 'Find or create a tag'} query={query}>
            {creatable ? (
                <Row disabled={full || hasOverlongTag(draft)} id="create-tag" onSelect={() => choose(addTags(tags, draft))}>
                    <PlusIcon aria-hidden="true" className="size-4 shrink-0 text-subtle-foreground" />
                    <Label>Create tag “{draft}”</Label>
                </Row>
            ) : null}
            {offered.map((tag) => <TagRow full={full} key={tag} onChange={choose} tag={tag} tags={tags} />)}
            {!creatable && !offered.length ? <p className="px-3 py-6 text-center text-sm text-muted-foreground">Type a tag to create it.</p> : null}
        </Shell>
    );
}

// A failure puts the list back to what last saved and drops the edits queued behind it, which were built on the failed one.
function useTagEdits(step: Extract<CommandStep, { kind: 'tags' }>, run: Run) {
    const [tags, setTags] = useState(step.tags);
    const confirmed = useRef(step.tags);
    const queue = useRef(Promise.resolve());
    const failures = useRef(0);
    function change(next: string[]) {
        setTags(next);
        const queuedIn = failures.current;
        queue.current = queue.current
            .then(async () => {
                if (queuedIn !== failures.current) return;
                await step.save(next);
                confirmed.current = next;
            })
            .catch((failure: unknown) => {
                failures.current += 1;
                setTags(confirmed.current);
                run.fail(problemText(failure, 'Couldn’t save these tags. Try again.'));
            });
    }
    return { tags, change };
}

// The source's own tags first, then the reader's other tags as suggestions.
function useOfferedTags(path: string, tags: string[]): string[] {
    const suggestions = useSources<{ tags: string[] }>(path);
    const chosen = new Set(tags.map((tag) => tag.toLowerCase()));
    const known = suggestions.status === 'loaded' ? suggestions.data.tags : [];
    return [...tags, ...known.filter((tag) => !chosen.has(tag.toLowerCase()))];
}

function tagHint(draft: string, creatable: boolean, full: boolean): string {
    if (creatable && hasOverlongTag(draft)) return `Tags can be up to ${MAX_TAG_LENGTH} characters.`;
    return full ? `Up to ${MAX_TAGS} tags.` : '';
}

function TagRow({ tag, tags, full, onChange }: { tag: string; tags: string[]; full: boolean; onChange: (tags: string[]) => void }) {
    const on = tags.some((other) => other.toLowerCase() === tag.toLowerCase());
    return (
        <Row disabled={!on && full} id={`tag:${tag}`} keywords={[tag]} onSelect={() => onChange(on ? tags.filter((other) => other !== tag) : [...tags, tag])}>
            <span className="flex size-4 shrink-0 items-center justify-center">{on ? <CheckIcon aria-hidden="true" className="size-4 text-primary" /> : null}</span>
            <Label>{tag}</Label>
            <span className="sr-only">{on ? 'Tagged' : 'Not tagged'}</span>
        </Row>
    );
}

function ConfirmStep({ step, run, back }: StepProps<'confirm'>) {
    return (
        <Shell chip={step.title} error={run.error} onBackspace={back} onQuery={() => {}} placeholder="Press Enter to confirm, Esc to go back" query="" shouldFilter={false}>
            <p className="px-3 pt-2 pb-3 text-sm text-muted-foreground">{step.description}</p>
            <Row destructive disabled={run.pending} id="confirm" onSelect={() => void run.run(step.submit)}>
                <Label>{step.action}</Label>
                {run.pending ? <Busy /> : <EnterIcon aria-hidden="true" className="size-4 shrink-0" />}
            </Row>
        </Shell>
    );
}

function logOut() {
    forgetShapes();
    const form = document.createElement('form');
    form.method = 'POST';
    form.action = PATHS.oauthLogout;
    document.body.append(form);
    form.submit();
}

function useGlobalCommands(onAddSource: (url: string) => void): Command[] {
    const [, navigate] = useLocation();
    const [font, setFont] = useReaderFont();
    const following = usePublicFollowing();
    const feeds = useSources<FeedSource[]>('/api/subscriptions');
    const newsletters = useSources<{ active: NewsletterSource[]; stopped: NewsletterSource[] }>('/api/newsletters');
    const today = startOfToday();
    const go = (href: string) => () => navigate(href);

    const navigation: Command[] = [
        ...APP_MODES.map(({ label, href, shortcut }) => ({ id: `go:${href}`, group: GO_TO, label, shortcut, run: go(href) })),
        {
            id: 'go:day',
            group: GO_TO,
            label: 'Day…',
            keywords: ['date', 'calendar', 'digest'],
            step: {
                kind: 'choice',
                title: 'Go to day',
                placeholder: 'Find a day, or type one like “14.10”',
                options: Array.from({ length: 28 }, (_, ago) => {
                    const date = addCalendarDays(today, -ago);
                    return { value: ago ? toDateKey(date) : '', label: formatDay(date, today) };
                }),
                submit: (key) => navigate(digestHref(key || undefined)),
            },
        },
        { id: 'go:general', group: GO_TO, label: 'General settings', keywords: ['settings', 'preferences'], run: go(PATHS.general) },
        { id: 'go:import', group: GO_TO, label: 'Import sources', keywords: ['settings', 'opml', 'youtube', 'takeout'], run: go(PATHS.import) },
        { id: 'go:export', group: GO_TO, label: 'Export sources', keywords: ['settings', 'opml', 'public', 'following'], run: go(PATHS.export) },
    ];

    const sources: Command[] = [
        ...(feeds.status === 'loaded' ? feeds.data : []).map((feed) => {
            const location = displayURL(feed.siteUrl || feed.feedUrl);
            return {
                id: `source:${feed.rkey}`,
                group: 'Sources',
                label: feed.title || location,
                detail: feed.title ? location : undefined,
                keywords: [feed.feedUrl, feed.siteUrl ?? '', ...(feed.tags ?? [])],
                icon: feed.faviconUrl ?? null,
                searchOnly: true,
                run: go(sourceHref(feed.rkey)),
            };
        }),
        ...(newsletters.status === 'loaded' ? [...newsletters.data.active.map((source) => [source, false] as const), ...newsletters.data.stopped.map((source) => [source, true] as const)] : [])
            .map(([source, stopped]) => ({
                id: `newsletter:${source.id}`,
                group: 'Sources',
                label: source.title || source.senderAddress,
                detail: stopped ? `Stopped · ${source.senderAddress}` : source.senderAddress,
                keywords: source.tags,
                icon: null,
                searchOnly: true,
                run: go(newsletterSourceHref(source.id)),
            })),
    ];

    const actions: Command[] = [
        { id: 'add-source', group: ACTIONS, label: 'Add a source…', keywords: ['subscribe', 'follow', 'feed', 'new'], run: () => onAddSource('') },
        {
            id: 'refresh',
            group: ACTIONS,
            label: 'Refresh sources',
            keywords: ['sync', 'fetch', 'update', 'reload'],
            run: async () => {
                await api('/api/digest/refresh', { method: 'POST' });
                subscriptionChanges.dispatchEvent(new Event('change'));
                return 'Refreshing your sources';
            },
        },
        {
            id: 'reader-font',
            group: ACTIONS,
            label: 'Reader font',
            detail: font === 'serif' ? 'Serif' : 'Sans',
            keywords: ['typeface', 'serif', 'sans'],
            step: {
                kind: 'choice',
                title: 'Reader font',
                placeholder: 'Choose a font',
                current: font,
                options: [{ value: 'sans', label: 'Sans' }, { value: 'serif', label: 'Serif' }],
                submit: (value) => setFont(value === 'serif' ? 'serif' : 'sans'),
            },
        },
        {
            id: 'newsletter-address',
            group: ACTIONS,
            label: 'Copy newsletter address',
            keywords: ['email', 'inbound', 'subscribe'],
            run: async () => {
                const { address } = await api<{ address?: string }>('/api/newsletters/address');
                if (!address) throw new CommandProblem('Newsletter delivery isn’t set up for your account.');
                return copied(address, 'Newsletter address copied');
            },
        },
        { id: 'export-opml', group: ACTIONS, label: 'Download OPML export', keywords: ['export', 'backup', 'opml'], run: downloadOpml },
        ...(following.enabled === undefined ? [] : [{
            id: 'public-following',
            group: ACTIONS,
            label: 'Public following list',
            keywords: ['blogroll', 'opml', 'share', 'publish'],
            toggle: { on: following.enabled, set: following.set },
        }]),
        {
            id: 'shortcuts',
            group: ACTIONS,
            label: 'Keyboard shortcuts',
            keywords: ['keys', 'help', 'hotkeys'],
            step: {
                kind: 'choice',
                title: 'Keyboard shortcuts',
                placeholder: 'Find a shortcut',
                options: SHORTCUTS.map(({ keys, label, where }) => ({ value: `${where}:${label}`, label, detail: where, shortcut: keys })),
                submit: () => {},
            },
        },
        { id: 'log-out', group: ACTIONS, label: 'Log out', keywords: ['sign out', 'logout'], run: logOut },
    ];

    return [...navigation, ...actions, ...sources];
}
