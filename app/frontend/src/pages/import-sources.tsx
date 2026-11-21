import { Dialog } from '@base-ui/react/dialog';
import { useRef } from 'react';

import { UploadIcon } from '@/components/icons';
import { PublicDataHint } from '@/components/public-data-hint';
import { Accordion, AccordionContent, AccordionItem, AccordionTrigger } from '@/components/ui/accordion';
import { BulletItem, BulletList } from '@/components/ui/bullet-list';
import { Button } from '@/components/ui/button';
import { DialogBackdrop, DialogPopup } from '@/components/ui/dialog';
import { Link } from '@/components/ui/link';
import { ProgressBar } from '@/components/ui/progress-bar';
import { Frame } from '@/design/frame';
import { Text } from '@/design/text';
import { useDocumentTitle } from '@/hooks/use-document-title';
import { useSourceImport, type ImportState } from '@/hooks/use-source-import';
import { useSources, type FeedSource } from '@/hooks/use-sources';
import { DataSettings } from '@/layouts/data-settings';
import { PATHS } from '@/lib/paths';
import type { ImportOutcome, ImportPlan, ImportSource } from '@/lib/source-import';

export function ImportSources() {
    const { state, prepare, startImport, cancel } = useSourceImport();
    const input = useRef<HTMLInputElement>(null);
    useDocumentTitle('Import sources');
    const busy = state.kind === 'preparing' || state.kind === 'importing';
    const preparingProvider = state.kind === 'preparing' ? state.provider : null;
    const failure = (provider: string) =>
        state.kind === 'failed' && state.provider === provider ? state.message : null;

    return (
        <DataSettings>
            <Frame gap="5xl">
                <Frame gap="m">
                    <Text as="h1" size="2xl" weight="medium">
                        Import sources
                    </Text>
                    <Frame maxWidth="xl">
                        <Text tone="muted">
                            Bring your feeds with you. Folders become tags, and sources you already follow are kept.
                        </Text>
                    </Frame>
                </Frame>

                <Frame gap="4xl">
                    <Frame as="section" aria-labelledby="opml-title" gap="l">
                        <Frame gap="2xs" maxWidth="lg">
                            <Text as="h2" id="opml-title" size="lg" weight="medium">
                                From an OPML file
                            </Text>
                            <Text size="sm" tone="muted">
                                Upload an export from Reeder, NetNewsWire, or another RSS reader. A feed in several
                                folders gets each folder’s tag.
                            </Text>
                        </Frame>
                        <input
                            ref={input}
                            type="file"
                            accept=".opml,.xml,text/xml,application/xml,text/x-opml"
                            hidden
                            aria-label="OPML file"
                            onChange={(event) => {
                                const file = event.target.files?.[0];
                                event.target.value = '';
                                if (file) void prepare('opml', file);
                            }}
                        />
                        <Frame align="start" gap="s">
                            <Button variant="secondary" disabled={busy} onClick={() => input.current?.click()}>
                                <UploadIcon />
                                {preparingProvider === 'opml' ? 'Loading' : 'Choose OPML file'}
                            </Button>
                            <ImportError message={failure('opml')} />
                        </Frame>
                    </Frame>

                    <YouTubeImport
                        busy={busy}
                        error={failure('youtube')}
                        loading={preparingProvider === 'youtube'}
                        onFile={(file) => void prepare('youtube', file)}
                    />

                    <Frame as="section" aria-labelledby="apps-title" gap="xl">
                        <Frame gap="2xs" maxWidth="lg">
                            <Text as="h2" id="apps-title" size="lg" weight="medium">
                                From another app
                            </Text>
                            <Text size="sm" tone="muted">
                                Read the subscriptions on your current ATProto account. Your sources in the other app
                                stay unchanged.
                            </Text>
                        </Frame>
                        <Frame gap="xl">
                            {(
                                [
                                    {
                                        provider: 'skyreader',
                                        name: 'Skyreader',
                                        description: 'RSS feeds, categories, and tags',
                                    },
                                    {
                                        provider: 'glean',
                                        name: 'Glean',
                                        description: 'RSS feeds and categories',
                                    },
                                ] as const
                            ).map(({ provider, name, description }) => (
                                <Frame
                                    key={provider}
                                    direction={{ base: 'column', sm: 'row' }}
                                    align={{ base: 'start', sm: 'center' }}
                                    justify={{ sm: 'between' }}
                                    gap="m"
                                >
                                    <Frame gap="2xs">
                                        <Text as="h3" size="sm" weight="medium">
                                            {name}
                                        </Text>
                                        <Text size="sm" tone="subtle">
                                            {description}
                                        </Text>
                                    </Frame>
                                    <Frame align={{ base: 'start', sm: 'end' }} gap="s">
                                        <Button
                                            variant="secondary"
                                            disabled={busy}
                                            onClick={() => void prepare(provider)}
                                        >
                                            {preparingProvider === provider ? 'Loading' : `Import from ${name}`}
                                        </Button>
                                        <ImportError message={failure(provider)} end />
                                    </Frame>
                                </Frame>
                            ))}
                        </Frame>
                    </Frame>
                </Frame>
                <Frame maxWidth="lg">
                    <Text size="xs" tone="subtle">
                        Imported feed subscriptions are public records on your PDS, like sources you add in Morgenblau.
                        Newsletters and native Standardfeed subscriptions are not imported here.
                    </Text>
                </Frame>
            </Frame>

            <ImportDialog
                state={state}
                onCancel={cancel}
                onConfirm={(plan) => void startImport(plan)}
                onRetry={(outcome) => void startImport({ sources: outcome.remaining, warnings: [] }, outcome)}
            />
        </DataSettings>
    );
}

// end: in a row whose action sits at the far edge from sm up, the message lines up with it.
function ImportError({ message, end = false }: { message: string | null; end?: boolean }) {
    if (!message) return null;
    return (
        <Frame alignSelf="stretch" maxWidth="lg">
            <Text role="alert" size="sm" tone="destructive" align={end ? { sm: 'end' } : undefined}>
                {message}
            </Text>
        </Frame>
    );
}

function YouTubeImport({
    busy,
    error,
    loading,
    onFile,
}: {
    busy: boolean;
    error: string | null;
    loading: boolean;
    onFile: (file: File) => void;
}) {
    const input = useRef<HTMLInputElement>(null);
    return (
        <Frame as="section" aria-labelledby="youtube-title" gap="l">
            <Text as="h2" id="youtube-title" size="lg" weight="medium">
                From YouTube
            </Text>
            <input
                ref={input}
                type="file"
                accept=".csv,text/csv"
                hidden
                aria-label="YouTube subscriptions CSV"
                onChange={(event) => {
                    const file = event.target.files?.[0];
                    event.target.value = '';
                    if (file) onFile(file);
                }}
            />
            <Frame align="start" gap="s">
                <Button
                    variant="secondary"
                    disabled={busy}
                    aria-describedby="youtube-help"
                    onClick={() => input.current?.click()}
                >
                    <UploadIcon />
                    {loading ? 'Loading' : 'Import YouTube subscriptions'}
                </Button>
                <ImportError message={error} />
                <Frame alignSelf="stretch" maxWidth="lg">
                    <Text id="youtube-help" size="sm" tone="muted">
                        Export your YouTube subscriptions from{' '}
                        <Link
                            href="https://takeout.google.com/takeout/custom/youtube"
                            target="_blank"
                            rel="noopener noreferrer"
                            variant="underline"
                        >
                            Google Takeout
                        </Link>
                        , then extract and upload the subscriptions CSV file.
                    </Text>
                </Frame>
            </Frame>
        </Frame>
    );
}

function ImportDialog({
    state,
    onCancel,
    onConfirm,
    onRetry,
}: {
    state: ImportState;
    onCancel: () => void;
    onConfirm: (plan: ImportPlan) => void;
    onRetry: (outcome: ImportOutcome) => void;
}) {
    if (state.kind === 'idle' || state.kind === 'preparing' || state.kind === 'failed') return null;
    return (
        <Dialog.Root
            open
            onOpenChange={(open, details) => {
                if (state.kind === 'importing') details.cancel();
                else if (!open) onCancel();
            }}
        >
            <Dialog.Portal>
                <DialogBackdrop />
                <DialogPopup size="md" short>
                    <ImportDialogContent state={state} onConfirm={onConfirm} onRetry={onRetry} />
                </DialogPopup>
            </Dialog.Portal>
        </Dialog.Root>
    );
}

function ImportDialogContent({
    state,
    onConfirm,
    onRetry,
}: {
    state: Exclude<ImportState, { kind: 'idle' | 'preparing' | 'failed' }>;
    onConfirm: (plan: ImportPlan) => void;
    onRetry: (outcome: ImportOutcome) => void;
}) {
    if (state.kind === 'confirm') {
        return <ImportConfirmation plan={state.plan} onConfirm={() => onConfirm(state.plan)} />;
    }
    if (state.kind === 'done') {
        return <ImportReceipt outcome={state.outcome} onRetry={() => onRetry(state.outcome)} />;
    }
    return (
        <Frame gap="2xl">
            <Frame gap="m">
                <Dialog.Title render={<Text as="h2" size="lg" weight="medium" />}>Importing sources</Dialog.Title>
                <Dialog.Description render={<Text size="sm" tone="muted" />} role="status">
                    {state.completed} of {state.total} processed. Keep this page open.
                </Dialog.Description>
            </Frame>
            <ProgressBar value={state.completed} max={state.total} aria-label="Sources processed" />
        </Frame>
    );
}

function ImportReceipt({ outcome, onRetry }: { outcome: ImportOutcome; onRetry: () => void }) {
    const paused = outcome.remaining.length > 0;
    return (
        <Frame gap="2xl">
            <Frame gap="m">
                <Dialog.Title render={<Text as="h2" size="lg" weight="medium" />}>
                    {paused ? 'Import paused' : 'Subscriptions saved'}
                </Dialog.Title>
                {outcome.imported.length > 0 && (
                    <Frame paddingBottom="2xs">
                        <Dialog.Description render={<Text size="sm" tone="muted" />}>
                            {paused
                                ? 'Successfully imported subscriptions are saved.'
                                : 'Your subscriptions are saved.'}{' '}
                            Their posts are checked separately. You can close this window; collection continues
                            automatically.
                        </Dialog.Description>
                    </Frame>
                )}
                <ImportResult outcome={outcome} onRetry={onRetry} />
            </Frame>
            <Frame direction="row" justify="end" gap="s">
                <Dialog.Close
                    nativeButton={false}
                    render={
                        <Button
                            variant="ghost"
                            nativeButton={false}
                            render={<Link href={PATHS.sources} variant="plain" />}
                        />
                    }
                >
                    View sources
                </Dialog.Close>
                <Dialog.Close render={<Button variant="secondary" />}>Done</Dialog.Close>
            </Frame>
        </Frame>
    );
}

function ImportConfirmation({ plan, onConfirm }: { plan: ImportPlan; onConfirm: () => void }) {
    const count = plan.sources.length;
    return (
        <Frame gap="2xl">
            <Frame gap="l">
                <Frame gap="m">
                    <Dialog.Title render={<Text as="h2" size="lg" weight="medium" />}>
                        Import {count} {count === 1 ? 'source' : 'sources'}?
                    </Dialog.Title>
                    <Dialog.Description render={<Text size="sm" tone="muted" />}>
                        New sources will be added. Existing sources keep their titles and primary status; incoming tags
                        are merged with yours.
                    </Dialog.Description>
                </Frame>
                {plan.warnings.length > 0 && (
                    <Accordion tone="muted">
                        <AccordionItem value="warnings">
                            <AccordionTrigger>Import warnings ({plan.warnings.length})</AccordionTrigger>
                            <AccordionContent>
                                <BulletList>
                                    {plan.warnings.map((warning, index) => (
                                        <BulletItem key={index}>{warning}</BulletItem>
                                    ))}
                                </BulletList>
                            </AccordionContent>
                        </AccordionItem>
                    </Accordion>
                )}
                <PublicDataHint />
            </Frame>
            <Frame direction="row" justify="end" gap="s">
                <Dialog.Close render={<Button variant="ghost" />}>Cancel</Dialog.Close>
                <Button onClick={onConfirm}>Import sources</Button>
            </Frame>
        </Frame>
    );
}

function ImportResult({ outcome, onRetry }: { outcome: ImportOutcome; onRetry: () => void }) {
    return (
        <Frame gap="m">
            <Text size="sm" tone="muted">
                {outcome.added} added, {outcome.updated} updated, {outcome.unchanged} already up to date.
            </Text>
            {outcome.error && (
                <Text size="sm" tone="destructive">
                    {outcome.error}
                </Text>
            )}
            <ImportFailures failures={outcome.failures} />
            {outcome.imported.length > 0 && <ImportCollection sources={outcome.imported} />}
            {outcome.remaining.length > 0 && (
                <>
                    <Text size="sm" tone="muted">
                        Sources still to check: {outcome.remaining.length}. Successful imports are kept; retrying will
                        not duplicate them.
                    </Text>
                    <Frame direction="row">
                        <Button variant="secondary" onClick={onRetry}>
                            Retry remaining sources
                        </Button>
                    </Frame>
                </>
            )}
        </Frame>
    );
}

function ImportCollection({ sources }: { sources: ImportSource[] }) {
    const state = useSources<FeedSource[]>('/api/subscriptions', 2_000);
    if (state.status !== 'loaded') {
        const messages = {
            loading: 'Checking posts…',
            error: 'Couldn’t check post availability. Your subscriptions are saved.',
        };
        return (
            <Text role="status" size="sm" tone="muted">
                {messages[state.status]}
            </Text>
        );
    }
    const byURL = new Map(state.data.map((source) => [source.feedUrl, source]));
    const unavailable = sources.filter((source) => byURL.get(source.feedUrl)?.fetchStatus === 'unavailable');
    if (unavailable.length === 0) return null;
    return (
        <Accordion tone="muted">
            <AccordionItem value="collection">
                <AccordionTrigger>
                    Posts not fetching for {unavailable.length} {unavailable.length === 1 ? 'source' : 'sources'}
                </AccordionTrigger>
                <AccordionContent>
                    <BulletList>
                        {unavailable.map((source) => (
                            <BulletItem key={source.feedUrl}>{source.title || source.feedUrl}</BulletItem>
                        ))}
                    </BulletList>
                </AccordionContent>
            </AccordionItem>
        </Accordion>
    );
}

function ImportFailures({ failures }: { failures: ImportOutcome['failures'] }) {
    if (failures.length === 0) return null;
    return (
        <Frame as="ul" gap="s">
            {failures.map((failure) => (
                <Frame as="li" key={failure.feedUrl}>
                    <Text size="sm" tone="destructive" break="words">
                        {failure.feedUrl}: {failure.message}
                    </Text>
                </Frame>
            ))}
        </Frame>
    );
}
