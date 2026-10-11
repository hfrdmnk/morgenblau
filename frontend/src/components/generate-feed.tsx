import { motion } from 'motion/react';
import { useEffect, useId, useRef, useState, type FormEvent } from 'react';
import { toast } from 'sonner';

import { LoadingIcon } from '@/components/icons';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Link } from '@/components/ui/link';
import { useGeneration, type GenerationStep } from '@/hooks/use-generation';
import {
    addSource,
    anchorError,
    anchorRejection,
    generatedCandidate,
    generatedVerdict,
    type GeneratedSource,
} from '@/lib/add-source';
import { classifyMutationError, describeMutationError } from '@/lib/api';
import { PATHS } from '@/lib/paths';
import { cn } from '@/lib/utils';

// A sample is enough to recognize the site's posts and keeps Add source on screen, with the post field still in view when unsure.
const PREVIEW_POSTS = { sure: 5, unsure: 3 };

// Listing pages print a day, which the server keeps as midnight UTC.
const postDate = new Intl.DateTimeFormat(undefined, {
    day: 'numeric',
    month: 'short',
    year: 'numeric',
    timeZone: 'UTC',
});

function postCount(count: number) {
    return count === 1 ? '1 post' : `${count} posts`;
}

type SaveProps = {
    saving: boolean;
    onSavingChange: (saving: boolean) => void;
    onSaved: () => void;
};

export function GenerateFeed({
    url,
    instant,
    ...save
}: SaveProps & { url: string; instant: boolean }) {
    const { state, generate, pointAt } = useGeneration(url);
    const root = useRef<HTMLDivElement>(null);
    useEffect(() => {
        if (state.step !== 'offer') settleFocus(root.current, focusOrder(state));
    }, [state]);
    return (
        <div className="outline-none" ref={root} tabIndex={-1}>
            <p className="sr-only" role="status">
                {resultStatus(state)}
            </p>
            <motion.div
                key={state.step}
                initial={{ opacity: instant ? 1 : 0 }}
                animate={{ opacity: 1 }}
                transition={{ duration: instant ? 0 : 0.2 }}
            >
                {state.step === 'offer' && (
                    <GenerateOffer onGenerate={generate} />
                )}
                {state.step === 'pending' && (
                    <Pending>
                        Reading the page and a few of its posts. This can take
                        a little while.
                    </Pending>
                )}
                {state.step === 'failed' && (
                    <GenerationFailed
                        message={state.message}
                        onRetry={state.retry ? generate : undefined}
                    />
                )}
                {state.step === 'preview' && (
                    <GeneratedPreview
                        source={state.source}
                        onPointAt={pointAt}
                        {...save}
                    />
                )}
            </motion.div>
        </div>
    );
}

function resultStatus(state: GenerationStep) {
    if (state.step !== 'preview') return '';
    const { source } = state;
    if (source.anchorRejected) return anchorRejection(source.anchorRejected);
    if (!generatedVerdict(source).subscribable) return '';
    return state.anchored
        ? `Found ${postCount(source.itemCount)} like that one.`
        : `Generated feed, ${postCount(source.itemCount)} found.`;
}

function focusOrder(state: GenerationStep) {
    if (state.step === 'failed') return ['retry'];
    if (state.step !== 'preview') return [];
    if (state.source.anchorRejected) return ['anchor', 'add'];
    return state.anchored ? ['add', 'anchor'] : ['heading', 'anchor'];
}

// Each step unmounts the control that started it, and the dialog would park focus on its popup after a long wait.
function settleFocus(root: HTMLElement | null, order: string[]) {
    if (!root) return;
    const active = document.activeElement;
    const ours =
        !active ||
        !active.isConnected ||
        active === document.body ||
        active.contains(root) ||
        root.contains(active);
    if (!ours) return;
    const target = order
        .map((name) =>
            root.querySelector<HTMLElement>(`[data-focus="${name}"]`),
        )
        .find(Boolean);
    (target ?? root).focus();
}

function GenerateOffer({ onGenerate }: { onGenerate: () => void }) {
    return (
        <div className="rounded-lg bg-background p-4">
            <p className="text-sm text-muted-foreground">
                No feed found on this page. Morgenblau can generate one from the
                posts it lists.
            </p>
            <Button
                className="mt-3"
                onClick={onGenerate}
                size="sm"
                variant="secondary"
            >
                Generate a feed
            </Button>
        </div>
    );
}

function Pending({ children }: { children: string }) {
    return (
        <div
            className="flex h-36 flex-col items-center justify-center gap-3 px-6 text-center text-sm text-muted-foreground"
            role="status"
        >
            <LoadingIcon className="size-5 motion-safe:animate-spin" />
            <p>{children}</p>
        </div>
    );
}

function GenerationFailed({
    message,
    onRetry,
}: {
    message: string;
    onRetry?: () => void;
}) {
    return (
        <div className="rounded-lg bg-background p-4">
            <p className="text-sm text-muted-foreground" role="alert">
                {message}
            </p>
            {onRetry && (
                <Button
                    className="mt-3"
                    data-focus="retry"
                    onClick={onRetry}
                    size="sm"
                    variant="secondary"
                >
                    Try again
                </Button>
            )}
        </div>
    );
}

function GeneratedPreview({
    source,
    onPointAt,
    ...save
}: SaveProps & {
    source: GeneratedSource;
    onPointAt: (anchor: string) => Promise<void>;
}) {
    const [anchoring, setAnchoring] = useState(false);
    const verdict = generatedVerdict(source);

    async function pointAt(anchor: string) {
        setAnchoring(true);
        try {
            await onPointAt(anchor);
        } finally {
            setAnchoring(false);
        }
    }

    if (verdict.blocked)
        return (
            <p className="rounded-lg bg-background p-4 text-sm text-muted-foreground">
                This site blocks feed readers.
            </p>
        );
    const unsure = verdict.askAnchor && verdict.subscribable;
    return (
        <>
            {source.items.length > 0 && (
                <PostPreview
                    source={source}
                    limit={unsure ? PREVIEW_POSTS.unsure : PREVIEW_POSTS.sure}
                />
            )}
            {verdict.subscribable && (
                <AddGeneratedSource
                    source={source}
                    anchoring={anchoring}
                    {...save}
                />
            )}
            {verdict.askAnchor && (
                <AnchorForm
                    key={source.id + source.itemCount}
                    prompt={anchorPrompt(source, verdict.subscribable)}
                    rejected={source.anchorRejected}
                    anchoring={anchoring}
                    disabled={save.saving}
                    onSubmit={pointAt}
                />
            )}
        </>
    );
}

function PostPreview({
    source,
    limit,
}: {
    source: GeneratedSource;
    limit: number;
}) {
    const headingId = useId();
    const host = source.siteUrl.replace(/^https?:\/\//, '').replace(/\/$/, '');
    return (
        <section aria-labelledby={headingId}>
            <p className="mb-2 text-xs text-muted-foreground">Generated feed</p>
            <div className="rounded-lg bg-background p-4">
                <h3
                    className="truncate text-sm font-medium outline-none"
                    data-focus="heading"
                    id={headingId}
                    tabIndex={-1}
                    title={source.title}
                >
                    {source.title}
                </h3>
                <p
                    className="mt-1 truncate text-xs text-subtle-foreground"
                    title={source.siteUrl}
                >
                    {host}
                </p>
                <ul className="mt-4 space-y-2.5">
                    {source.items.slice(0, limit).map((item) => (
                        <PostRow item={item} key={item.url} />
                    ))}
                </ul>
                <p className="mt-4 text-xs text-muted-foreground">
                    {postCount(source.itemCount)} found
                </p>
            </div>
        </section>
    );
}

function PostRow({ item }: { item: GeneratedSource['items'][number] }) {
    return (
        <li className="flex items-baseline justify-between gap-4">
            <span className="min-w-0 truncate text-sm" title={item.title}>
                {item.title}
            </span>
            {item.date && (
                <time
                    className="shrink-0 text-xs text-subtle-foreground tabular-nums"
                    dateTime={item.date}
                >
                    {postDate.format(new Date(item.date))}
                </time>
            )}
        </li>
    );
}

function AddGeneratedSource({
    source,
    anchoring,
    saving,
    onSavingChange,
    onSaved,
}: SaveProps & { source: GeneratedSource; anchoring: boolean }) {
    const [failure, setFailure] = useState<{
        source: GeneratedSource;
        message: string;
        reauth: boolean;
    } | null>(null);
    const error = failure?.source === source ? failure : null;

    async function save() {
        onSavingChange(true);
        try {
            await addSource(generatedCandidate(source));
            toast.success('Source added', {
                description:
                    'New posts appear in your newspaper as the site publishes them.',
            });
            onSaved();
        } catch (problem) {
            setFailure({
                source,
                message: describeMutationError(
                    problem,
                    'The save couldn’t be confirmed.',
                ),
                reauth: classifyMutationError(problem) === 'reauth',
            });
        } finally {
            onSavingChange(false);
        }
    }

    return (
        <>
            {error && (
                <p className="mt-3 text-sm text-destructive" role="alert">
                    {error.message}{' '}
                    {error.reauth && (
                        <Link href={PATHS.login}>Sign in again</Link>
                    )}
                </p>
            )}
            <div className="mt-4 flex items-center justify-between gap-3">
                <p className="text-xs text-muted-foreground" aria-live="polite">
                    {saving && 'Keep this dialog open while saving.'}
                </p>
                <Button
                    aria-busy={saving}
                    data-focus="add"
                    disabled={saving || anchoring || Boolean(error)}
                    onClick={save}
                >
                    {saving ? (
                        <>
                            <LoadingIcon className="motion-safe:animate-spin" />
                            <span className="sr-only">Adding source</span>
                        </>
                    ) : (
                        'Add source'
                    )}
                </Button>
            </div>
        </>
    );
}

const ANCHOR_PROMPT = {
    unsure: 'Morgenblau isn’t sure these are the site’s posts. Paste the address of one post from this page to find more like it.',
    stale: 'Morgenblau couldn’t read this page on its last visit. If these aren’t the site’s posts, paste the address of one post from this page to find more like it.',
    empty: 'No posts found on this page yet. Paste the address of one post from it, and Morgenblau will look for posts like it.',
};

// A shared feed can be broken by one failed poll while it still serves the posts it stored.
function anchorPrompt(source: GeneratedSource, found: boolean) {
    if (!found) return ANCHOR_PROMPT.empty;
    return source.health === 'broken'
        ? ANCHOR_PROMPT.stale
        : ANCHOR_PROMPT.unsure;
}

function useAnchorField(
    rejected: GeneratedSource['anchorRejected'],
    anchoring: boolean,
    onSubmit: (anchor: string) => Promise<void>,
) {
    const [anchor, setAnchor] = useState('');
    const [edited, setEdited] = useState(false);
    const [error, setError] = useState<ReturnType<typeof anchorError> | null>(
        null,
    );
    const notice =
        rejected && !edited && !error ? anchorRejection(rejected) : undefined;

    async function submit(event: FormEvent) {
        event.preventDefault();
        if (anchoring || !anchor.trim()) return;
        setError(null);
        setEdited(false);
        await onSubmit(anchor).catch((failure) =>
            setError(anchorError(failure)),
        );
    }

    function change(value: string) {
        setAnchor(value);
        setError(null);
        setEdited(true);
    }

    return {
        anchor,
        error,
        notice,
        described: !anchoring && Boolean(error || notice),
        submit,
        change,
    };
}

function AnchorForm({
    prompt,
    rejected,
    anchoring,
    disabled,
    onSubmit,
}: {
    prompt: string;
    rejected: GeneratedSource['anchorRejected'];
    anchoring: boolean;
    disabled: boolean;
    onSubmit: (anchor: string) => Promise<void>;
}) {
    const inputId = useId();
    const field = useAnchorField(rejected, anchoring, onSubmit);

    return (
        <form className="mt-4" onSubmit={field.submit}>
            <label
                className="block text-sm text-muted-foreground"
                htmlFor={inputId}
            >
                {prompt}
            </label>
            <div className="mt-3 flex flex-col gap-2 sm:flex-row">
                <Input
                    aria-describedby={
                        field.described ? `${inputId}-message` : undefined
                    }
                    aria-invalid={Boolean(field.error?.invalid)}
                    aria-busy={anchoring}
                    autoCapitalize="none"
                    data-focus="anchor"
                    disabled={disabled}
                    id={inputId}
                    inputMode="url"
                    onChange={(event) => field.change(event.target.value)}
                    placeholder="example.com/posts/one-post"
                    readOnly={anchoring}
                    spellCheck={false}
                    value={field.anchor}
                />
                <UsePostButton
                    anchoring={anchoring}
                    disabled={disabled || !field.anchor.trim()}
                />
            </div>
            <AnchorMessage
                anchoring={anchoring}
                error={field.error}
                notice={field.notice}
                id={`${inputId}-message`}
            />
        </form>
    );
}

function UsePostButton({
    anchoring,
    disabled,
}: {
    anchoring: boolean;
    disabled: boolean;
}) {
    return (
        <Button
            aria-busy={anchoring}
            className="sm:w-32"
            disabled={disabled || anchoring}
            focusableWhenDisabled={anchoring}
            type="submit"
            variant="secondary"
        >
            {anchoring ? (
                <LoadingIcon className="size-4 motion-safe:animate-spin" />
            ) : (
                'Use this post'
            )}
        </Button>
    );
}

const SEARCHING = 'Looking for posts like that one. This can take a little while.';

function anchorMessage(
    anchoring: boolean,
    error: ReturnType<typeof anchorError> | null,
    notice?: string,
) {
    if (anchoring) return { text: SEARCHING, role: 'status' };
    // The dialog's status region already announced the notice; a second live region would repeat it.
    if (notice) return { text: notice };
    if (!error) return null;
    return { text: error.message, role: 'alert', invalid: error.invalid };
}

function AnchorMessage({
    anchoring,
    error,
    notice,
    id,
}: {
    anchoring: boolean;
    error: ReturnType<typeof anchorError> | null;
    notice?: string;
    id: string;
}) {
    const message = anchorMessage(anchoring, error, notice);
    const line = useRef<HTMLParagraphElement>(null);
    // The dialog can be taller than the screen, and the field's focus alone leaves the line below its fold.
    useEffect(() => {
        line.current?.scrollIntoView({ block: 'nearest' });
    }, [message?.text]);
    if (!message) return null;
    return (
        <p
            className={cn(
                'mt-3 scroll-mb-6 text-sm sm:scroll-mb-8',
                message.invalid ? 'text-destructive' : 'text-muted-foreground',
            )}
            id={anchoring ? undefined : id}
            ref={line}
            role={message.role}
        >
            {message.text}
        </p>
    );
}
