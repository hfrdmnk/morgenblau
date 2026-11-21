import { useEffect, useId, useRef, useState, type FormEvent, type ReactNode } from 'react';

import { EnterIcon, LoadingIcon } from '@/components/icons';
import { Button } from '@/components/ui/button';
import { FieldMessage } from '@/components/ui/field-message';
import { InputGroup, InputGroupButton, InputGroupInput } from '@/components/ui/input-group';
import { Frame, type FrameProps } from '@/design/frame';
import { MotionFrame } from '@/design/motion';
import { Text } from '@/design/text';
import type { GenerationStep } from '@/hooks/use-generation';
import { anchorError, anchorRejection, generatedVerdict, type GeneratedSource } from '@/lib/add-source';

// A sample is enough to recognize the site's posts, with the post field still in view when unsure.
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

type PreviewProps = {
    saving: boolean;
    onPointAt: (anchor: string) => Promise<void>;
    // The sheet's own form for adding the feed.
    add: (source: GeneratedSource) => ReactNode;
};

export function GenerateFeed({
    step,
    instant,
    onRetry,
    ...preview
}: PreviewProps & { step: GenerationStep; instant: boolean; onRetry: () => void }) {
    const root = useRef<HTMLDivElement>(null);
    useEffect(() => {
        settleFocus(root.current, focusOrder(step));
    }, [step]);
    return (
        <Frame outline="none" ref={root} tabIndex={-1}>
            <Text role="status" visuallyHidden>
                {resultStatus(step)}
            </Text>
            <MotionFrame
                key={step.step}
                initial={{ opacity: instant ? 1 : 0 }}
                animate={{ opacity: 1 }}
                transition={{ duration: instant ? 0 : 0.2 }}
            >
                {step.step === 'pending' && (
                    <Pending>Reading the page and a few of its posts. This can take a little while.</Pending>
                )}
                {step.step === 'failed' && (
                    <GenerationFailed message={step.message} onRetry={step.retry ? onRetry : undefined} />
                )}
                {step.step === 'preview' && <GeneratedPreview source={step.source} {...preview} />}
            </MotionFrame>
        </Frame>
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

// The add form belongs to the sheet, so its submit button is found inside the slot rather than marked itself.
function focusTarget(root: HTMLElement, name: string) {
    return root.querySelector<HTMLElement>(
        name === 'add' ? '[data-focus="add"] [type="submit"]' : `[data-focus="${name}"]`,
    );
}

// Each step unmounts the control that started it, and the sheet would park focus on its popup after a long wait.
function settleFocus(root: HTMLElement | null, order: string[]) {
    if (!root) return;
    const active = document.activeElement;
    const ours =
        !active || !active.isConnected || active === document.body || active.contains(root) || root.contains(active);
    if (!ours) return;
    const target = order.map((name) => focusTarget(root, name)).find(Boolean);
    (target ?? root).focus();
}

// The sheet's inset panel; start alignment keeps a button in it at its own width.
function Panel(props: FrameProps) {
    return <Frame align="start" background="background" gap="m" padding="l" radius="lg" {...props} />;
}

export function GenerateOffer({ onGenerate }: { onGenerate: () => void }) {
    return (
        <Panel>
            <Text size="sm" tone="muted">
                No feed found on this page. Morgenblau can generate one from the posts it lists.
            </Text>
            <Button onClick={onGenerate} size="sm" variant="secondary">
                Generate a feed
            </Button>
        </Panel>
    );
}

function Pending({ children }: { children: string }) {
    return (
        <Frame align="center" gap="m" height="status" justify="center" paddingX="2xl" role="status">
            <LoadingIcon size="xl" spin tone="muted" />
            <Text align="center" size="sm" tone="muted">
                {children}
            </Text>
        </Frame>
    );
}

function GenerationFailed({ message, onRetry }: { message: string; onRetry?: () => void }) {
    return (
        <Panel>
            <Text role="alert" size="sm" tone="muted">
                {message}
            </Text>
            {onRetry && (
                <Button data-focus="retry" onClick={onRetry} size="sm" variant="secondary">
                    Try again
                </Button>
            )}
        </Panel>
    );
}

function GeneratedPreview({ source, saving, onPointAt, add }: PreviewProps & { source: GeneratedSource }) {
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
            <Panel>
                <Text size="sm" tone="muted">
                    This site blocks feed readers.
                </Text>
            </Panel>
        );
    const unsure = verdict.askAnchor && verdict.subscribable;
    const previewed = source.items.length > 0;
    return (
        // Without the post list, the form still sits as far below the sheet's header as it would below the list.
        <Frame gap="l" paddingTop={previewed ? undefined : 'l'}>
            {previewed && <PostPreview source={source} limit={unsure ? PREVIEW_POSTS.unsure : PREVIEW_POSTS.sure} />}
            {verdict.askAnchor && (
                <AnchorForm
                    key={source.id + source.itemCount}
                    prompt={anchorPrompt(source, verdict.subscribable)}
                    rejected={source.anchorRejected}
                    anchoring={anchoring}
                    disabled={saving}
                    onSubmit={pointAt}
                />
            )}
            {verdict.subscribable && (
                // A post being looked up may still change the feed, so its form waits.
                <Frame as="fieldset" data-focus="add" disabled={anchoring} minWidth={0} paddingTop="l">
                    {add(source)}
                </Frame>
            )}
        </Frame>
    );
}

function PostPreview({ source, limit }: { source: GeneratedSource; limit: number }) {
    const headingId = useId();
    const host = source.siteUrl.replace(/^https?:\/\//, '').replace(/\/$/, '');
    return (
        <Frame as="section" aria-labelledby={headingId} gap="s">
            <Text size="xs" tone="muted">
                Generated feed
            </Text>
            <Frame background="background" gap="l" padding="l" radius="lg">
                <Frame gap="2xs">
                    <Text
                        as="h3"
                        data-focus="heading"
                        id={headingId}
                        outline="none"
                        size="sm"
                        tabIndex={-1}
                        title={source.title}
                        truncate
                        weight="medium"
                    >
                        {source.title}
                    </Text>
                    <Text size="xs" title={source.siteUrl} tone="subtle" truncate>
                        {host}
                    </Text>
                </Frame>
                <Frame as="ul" gap="s">
                    {source.items.slice(0, limit).map((item) => (
                        <PostRow item={item} key={item.url} />
                    ))}
                </Frame>
                <Text size="xs" tone="muted">
                    {postCount(source.itemCount)} found
                </Text>
            </Frame>
        </Frame>
    );
}

function PostRow({ item }: { item: GeneratedSource['items'][number] }) {
    return (
        <Frame as="li" align="baseline" direction="row" gap="l" justify="between">
            <Text as="span" size="sm" title={item.title} truncate>
                {item.title}
            </Text>
            {item.date && (
                <Text as="time" dateTime={item.date} numeric="tabular" size="xs" tone="subtle" wrap="nowrap">
                    {postDate.format(new Date(item.date))}
                </Text>
            )}
        </Frame>
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
    return source.health === 'broken' ? ANCHOR_PROMPT.stale : ANCHOR_PROMPT.unsure;
}

function useAnchorField(
    rejected: GeneratedSource['anchorRejected'],
    anchoring: boolean,
    onSubmit: (anchor: string) => Promise<void>,
) {
    const [anchor, setAnchor] = useState('');
    const [edited, setEdited] = useState(false);
    const [error, setError] = useState<ReturnType<typeof anchorError> | null>(null);
    const notice = rejected && !edited && !error ? anchorRejection(rejected) : undefined;

    async function submit(event: FormEvent) {
        event.preventDefault();
        if (anchoring || !anchor.trim()) return;
        setError(null);
        setEdited(false);
        await onSubmit(anchor).catch((failure) => setError(anchorError(failure)));
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
        <Frame as="form" gap="m" onSubmit={field.submit}>
            <Text as="label" htmlFor={inputId} size="sm" tone="muted">
                {prompt}
            </Text>
            <InputGroup>
                <InputGroupInput
                    aria-describedby={field.described ? `${inputId}-message` : undefined}
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
                <UsePostButton anchoring={anchoring} disabled={disabled || !field.anchor.trim()} />
            </InputGroup>
            <AnchorMessage anchoring={anchoring} error={field.error} notice={field.notice} id={`${inputId}-message`} />
        </Frame>
    );
}

function UsePostButton({ anchoring, disabled }: { anchoring: boolean; disabled: boolean }) {
    return (
        <InputGroupButton
            aria-busy={anchoring}
            aria-label="Use this post"
            disabled={disabled || anchoring}
            focusableWhenDisabled={anchoring}
            type="submit"
        >
            {anchoring ? <LoadingIcon spin /> : <EnterIcon />}
        </InputGroupButton>
    );
}

const SEARCHING = 'Looking for posts like that one. This can take a little while.';

function anchorMessage(anchoring: boolean, error: ReturnType<typeof anchorError> | null, notice?: string) {
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
        <FieldMessage id={anchoring ? undefined : id} invalid={message.invalid} ref={line} role={message.role}>
            {message.text}
        </FieldMessage>
    );
}
