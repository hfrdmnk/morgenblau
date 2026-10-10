import { motion } from 'motion/react';
import { useId, useState, type FormEvent } from 'react';
import { toast } from 'sonner';

import { LoadingIcon } from '@/components/icons';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Link } from '@/components/ui/link';
import { useGeneration } from '@/hooks/use-generation';
import {
    addSource,
    anchorError,
    generatedCandidate,
    generatedVerdict,
    type GeneratedSource,
} from '@/lib/add-source';
import { classifyMutationError, describeMutationError } from '@/lib/api';
import { PATHS } from '@/lib/paths';
import { cn } from '@/lib/utils';

// A sample is enough to recognize the site's posts and keeps Add source on screen.
const PREVIEW_POSTS = 5;

const postDate = new Intl.DateTimeFormat(undefined, {
    day: 'numeric',
    month: 'short',
    year: 'numeric',
});

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
    return (
        <motion.div
            key={state.step}
            initial={{ opacity: instant ? 1 : 0 }}
            animate={{ opacity: 1 }}
            transition={{ duration: instant ? 0 : 0.2 }}
        >
            {state.step === 'offer' && <GenerateOffer onGenerate={generate} />}
            {state.step === 'pending' && (
                <Pending>
                    Reading the page and a few of its posts. This can take a
                    little while.
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
    );
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
    return (
        <>
            {source.items.length > 0 && <PostPreview source={source} />}
            {verdict.askAnchor && (
                <AnchorForm
                    key={source.id + source.itemCount}
                    found={verdict.subscribable}
                    anchoring={anchoring}
                    disabled={save.saving}
                    onSubmit={pointAt}
                />
            )}
            {verdict.subscribable && (
                <AddGeneratedSource
                    source={source}
                    anchoring={anchoring}
                    {...save}
                />
            )}
        </>
    );
}

function PostPreview({ source }: { source: GeneratedSource }) {
    const headingId = useId();
    const host = source.siteUrl.replace(/^https?:\/\//, '').replace(/\/$/, '');
    return (
        <section aria-labelledby={headingId}>
            <p className="mb-2 text-xs text-muted-foreground">Generated feed</p>
            <div className="rounded-lg bg-background p-4">
                <h3
                    className="truncate text-sm font-medium"
                    id={headingId}
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
                    {source.items.slice(0, PREVIEW_POSTS).map((item) => (
                        <PostRow item={item} key={item.url} />
                    ))}
                </ul>
                <p className="mt-4 text-xs text-muted-foreground">
                    {source.itemCount === 1
                        ? '1 post found'
                        : `${source.itemCount} posts found`}
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
    const [error, setError] = useState<{
        message: string;
        reauth: boolean;
    } | null>(null);

    async function save() {
        onSavingChange(true);
        try {
            await addSource(generatedCandidate(source));
            toast.success('Source added', {
                description:
                    'New posts appear in your newspaper as the site publishes them.',
            });
            onSaved();
        } catch (failure) {
            setError({
                message: describeMutationError(
                    failure,
                    'The save couldn’t be confirmed.',
                ),
                reauth: classifyMutationError(failure) === 'reauth',
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
    unsure: 'This list looks unsure. Paste the address of one post from this page, and Morgenblau will look for posts like it.',
    empty: 'No posts found on this page yet. Paste the address of one post from it, and Morgenblau will look for posts like it.',
};

function AnchorForm({
    found,
    anchoring,
    disabled,
    onSubmit,
}: {
    found: boolean;
    anchoring: boolean;
    disabled: boolean;
    onSubmit: (anchor: string) => Promise<void>;
}) {
    const inputId = useId();
    const [anchor, setAnchor] = useState('');
    const [error, setError] = useState<ReturnType<typeof anchorError> | null>(
        null,
    );

    async function submit(event: FormEvent) {
        event.preventDefault();
        if (anchoring || !anchor.trim()) return;
        setError(null);
        await onSubmit(anchor).catch((failure) =>
            setError(anchorError(failure)),
        );
    }

    return (
        <form className="mt-4" onSubmit={submit}>
            <label
                className="block text-sm text-muted-foreground"
                htmlFor={inputId}
            >
                {found ? ANCHOR_PROMPT.unsure : ANCHOR_PROMPT.empty}
            </label>
            <div className="mt-3 flex flex-col gap-2 sm:flex-row">
                <Input
                    aria-describedby={error ? `${inputId}-error` : undefined}
                    aria-invalid={Boolean(error?.invalid)}
                    autoCapitalize="none"
                    disabled={disabled || anchoring}
                    id={inputId}
                    inputMode="url"
                    onChange={(event) => {
                        setAnchor(event.target.value);
                        setError(null);
                    }}
                    placeholder="example.com/posts/one-post"
                    spellCheck={false}
                    value={anchor}
                />
                <UsePostButton
                    anchoring={anchoring}
                    disabled={disabled || !anchor.trim()}
                />
            </div>
            <AnchorMessage
                anchoring={anchoring}
                error={error}
                id={`${inputId}-error`}
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

function AnchorMessage({
    anchoring,
    error,
    id,
}: {
    anchoring: boolean;
    error: ReturnType<typeof anchorError> | null;
    id: string;
}) {
    if (anchoring)
        return (
            <p className="mt-3 text-sm text-muted-foreground" role="status">
                Looking for posts like that one. This can take a little while.
            </p>
        );
    if (!error) return null;
    return (
        <p
            className={cn(
                'mt-3 text-sm',
                error.invalid ? 'text-destructive' : 'text-muted-foreground',
            )}
            id={id}
            role="alert"
        >
            {error.message}
        </p>
    );
}
