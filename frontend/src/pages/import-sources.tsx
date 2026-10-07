import { Dialog } from '@base-ui/react/dialog';
import { useRef } from 'react';

import { UploadIcon } from '@/components/icons';
import {
    Accordion,
    AccordionContent,
    AccordionItem,
    AccordionTrigger,
} from '@/components/ui/accordion';
import { Button } from '@/components/ui/button';
import { useSourceImport, type ImportState } from '@/hooks/use-source-import';
import { DataSettings } from '@/layouts/data-settings';
import type { ImportOutcome, ImportPlan } from '@/lib/source-import';

export function ImportSources() {
    const { state, prepare, startImport, cancel } = useSourceImport();
    const input = useRef<HTMLInputElement>(null);
    const busy = state.kind === 'preparing' || state.kind === 'importing';
    const preparingProvider =
        state.kind === 'preparing' ? state.provider : null;

    return (
        <DataSettings page="import">
            <h1 className="text-2xl font-medium">Import sources</h1>
            <p className="mt-3 max-w-xl text-muted-foreground">
                Bring your feeds with you. Folders become tags, and sources you
                already follow are kept.
            </p>

            <div className="mt-12 space-y-10">
                <section aria-labelledby="opml-title">
                    <h2 id="opml-title" className="text-lg font-medium">
                        From an OPML file
                    </h2>
                    <p className="mt-2 max-w-lg text-sm text-muted-foreground">
                        Upload an export from Reeder, NetNewsWire, or another
                        RSS reader. A feed in several folders gets each folder’s
                        tag.
                    </p>
                    <input
                        ref={input}
                        type="file"
                        accept=".opml,.xml,text/xml,application/xml,text/x-opml"
                        className="hidden"
                        aria-label="OPML file"
                        onChange={(event) => {
                            const file = event.target.files?.[0];
                            event.target.value = '';
                            if (file) void prepare('opml', file);
                        }}
                    />
                    <Button
                        variant="secondary"
                        className="mt-4"
                        disabled={busy}
                        onClick={() => input.current?.click()}
                    >
                        <UploadIcon />
                        {preparingProvider === 'opml'
                            ? 'Loading'
                            : 'Choose OPML file'}
                    </Button>
                </section>

                <YouTubeImport
                    busy={busy}
                    loading={preparingProvider === 'youtube'}
                    onFile={(file) => void prepare('youtube', file)}
                />

                <section aria-labelledby="apps-title">
                    <h2 id="apps-title" className="text-lg font-medium">
                        From another app
                    </h2>
                    <p className="mt-2 max-w-lg text-sm text-muted-foreground">
                        Read the subscriptions on your current ATProto account.
                        Your sources in the other app stay unchanged.
                    </p>
                    <div className="mt-5 space-y-5">
                        {(
                            [
                                {
                                    provider: 'skyreader',
                                    name: 'Skyreader',
                                    description:
                                        'RSS feeds, categories, and tags',
                                },
                                {
                                    provider: 'glean',
                                    name: 'Glean',
                                    description: 'RSS feeds and categories',
                                },
                            ] as const
                        ).map(({ provider, name, description }) => (
                            <div
                                key={provider}
                                className="flex flex-col items-start gap-3 sm:flex-row sm:items-center sm:justify-between"
                            >
                                <div>
                                    <h3 className="text-sm font-medium">
                                        {name}
                                    </h3>
                                    <p className="mt-1 text-sm text-subtle-foreground">
                                        {description}
                                    </p>
                                </div>
                                <Button
                                    variant="secondary"
                                    disabled={busy}
                                    onClick={() => void prepare(provider)}
                                >
                                    {preparingProvider === provider
                                        ? 'Loading'
                                        : `Import from ${name}`}
                                </Button>
                            </div>
                        ))}
                    </div>
                </section>
            </div>
            <p className="mt-12 max-w-lg text-xs text-subtle-foreground">
                Imported feed subscriptions are public records on your PDS, like
                sources you add in Morgenblau. Newsletters and native
                Standardfeed subscriptions are not imported here.
            </p>

            <ImportDialog
                state={state}
                onCancel={cancel}
                onConfirm={(plan) => void startImport(plan)}
                onRetry={(outcome) =>
                    void startImport(
                        { sources: outcome.remaining, warnings: [] },
                        outcome,
                    )
                }
            />
        </DataSettings>
    );
}

function YouTubeImport({
    busy,
    loading,
    onFile,
}: {
    busy: boolean;
    loading: boolean;
    onFile: (file: File) => void;
}) {
    const input = useRef<HTMLInputElement>(null);
    return (
        <section aria-labelledby="youtube-title">
            <h2 id="youtube-title" className="text-lg font-medium">
                From YouTube
            </h2>
            <input
                ref={input}
                type="file"
                accept=".csv,text/csv"
                className="hidden"
                aria-label="YouTube subscriptions CSV"
                onChange={(event) => {
                    const file = event.target.files?.[0];
                    event.target.value = '';
                    if (file) onFile(file);
                }}
            />
            <Button
                variant="secondary"
                className="mt-4"
                disabled={busy}
                aria-describedby="youtube-help"
                onClick={() => input.current?.click()}
            >
                <UploadIcon />
                {loading ? 'Loading' : 'Import YouTube subscriptions'}
            </Button>
            <p
                id="youtube-help"
                className="mt-2 max-w-lg text-sm text-muted-foreground"
            >
                Export your YouTube subscriptions from{' '}
                <a
                    href="https://takeout.google.com/takeout/custom/youtube"
                    target="_blank"
                    rel="noopener noreferrer"
                    className="underline underline-offset-4"
                >
                    Google Takeout
                </a>
                , then extract and upload the subscriptions CSV file.
            </p>
        </section>
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
    if (state.kind === 'idle' || state.kind === 'preparing') return null;
    return (
        <Dialog.Root
            open
            onOpenChange={(open, details) => {
                if (state.kind === 'importing') details.cancel();
                else if (!open) onCancel();
            }}
        >
            <Dialog.Portal>
                <Dialog.Backdrop className="fixed inset-0 z-40 bg-black/20" />
                <Dialog.Popup className="fixed left-1/2 top-1/2 z-50 max-h-[85dvh] w-[calc(100%-2rem)] max-w-md -translate-x-1/2 -translate-y-1/2 overflow-y-auto rounded-xl bg-popover p-6 text-popover-foreground shadow-popover">
                    <ImportDialogContent
                        state={state}
                        onConfirm={onConfirm}
                        onRetry={onRetry}
                    />
                </Dialog.Popup>
            </Dialog.Portal>
        </Dialog.Root>
    );
}

function ImportDialogContent({
    state,
    onConfirm,
    onRetry,
}: {
    state: Exclude<ImportState, { kind: 'idle' | 'preparing' }>;
    onConfirm: (plan: ImportPlan) => void;
    onRetry: (outcome: ImportOutcome) => void;
}) {
    return (
        <>
            {state.kind === 'confirm' && (
                <ImportConfirmation
                    plan={state.plan}
                    onConfirm={() => onConfirm(state.plan)}
                />
            )}
            {state.kind === 'importing' && (
                <>
                    <Dialog.Title className="text-xl font-medium">
                        Importing sources
                    </Dialog.Title>
                    <Dialog.Description
                        className="mt-3 text-sm text-muted-foreground"
                        role="status"
                    >
                        {state.completed} of {state.total} processed. Keep this
                        page open.
                    </Dialog.Description>
                    <div
                        className="mt-6 h-1.5 overflow-hidden rounded-sm bg-secondary/50"
                        role="progressbar"
                        aria-valuenow={state.completed}
                        aria-valuemin={0}
                        aria-valuemax={state.total}
                        aria-label="Sources processed"
                    >
                        <div
                            className="h-full w-full rounded-sm bg-neutral transition-transform duration-(--motion-duration-overlay) ease-in-out motion-reduce:transition-none"
                            style={{ transform: `translateX(${(state.completed / state.total - 1) * 100}%)` }}
                        />
                    </div>
                </>
            )}
            {state.kind === 'done' && (
                <>
                    <Dialog.Title className="mb-3 text-xl font-medium">
                        Import paused
                    </Dialog.Title>
                    <ImportResult
                        outcome={state.outcome}
                        onRetry={() => onRetry(state.outcome)}
                    />
                    <Dialog.Close
                        render={<Button variant="ghost" className="mt-6" />}
                    >
                        Close
                    </Dialog.Close>
                </>
            )}
        </>
    );
}

function ImportConfirmation({
    plan,
    onConfirm,
}: {
    plan: ImportPlan;
    onConfirm: () => void;
}) {
    const count = plan.sources.length;
    return (
        <>
            <Dialog.Title className="text-xl font-medium">
                Import {count} {count === 1 ? 'source' : 'sources'}?
            </Dialog.Title>
            <Dialog.Description className="mt-3 text-sm text-muted-foreground">
                New sources will be added. Existing sources keep their titles
                and primary status; incoming tags are merged with yours.
            </Dialog.Description>
            {plan.warnings.length > 0 && (
                <Accordion className="mt-4 text-muted-foreground">
                    <AccordionItem value="warnings">
                        <AccordionTrigger>
                            Import warnings ({plan.warnings.length})
                        </AccordionTrigger>
                        <AccordionContent>
                            <ul className="list-disc space-y-2 pl-5">
                                {plan.warnings.map((warning, index) => (
                                    <li key={index} className="break-words">
                                        {warning}
                                    </li>
                                ))}
                            </ul>
                        </AccordionContent>
                    </AccordionItem>
                </Accordion>
            )}
            <div className="mt-6 flex justify-end gap-2">
                <Dialog.Close render={<Button variant="ghost" />}>
                    Cancel
                </Dialog.Close>
                <Button onClick={onConfirm}>Import sources</Button>
            </div>
        </>
    );
}

function ImportResult({
    outcome,
    onRetry,
}: {
    outcome: ImportOutcome;
    onRetry: () => void;
}) {
    return (
        <div className="space-y-3 text-sm">
            <p className="text-muted-foreground">
                {outcome.added} added, {outcome.updated} updated,{' '}
                {outcome.unchanged} already up to date.
            </p>
            {outcome.error && (
                <p className="text-destructive">{outcome.error}</p>
            )}
            <ImportFailures failures={outcome.failures} />
            {outcome.remaining.length > 0 && (
                <>
                    <p className="text-muted-foreground">
                        Sources still to check: {outcome.remaining.length}.
                        Successful imports are kept; retrying will not duplicate
                        them.
                    </p>
                    <Button variant="secondary" onClick={onRetry}>
                        Retry remaining sources
                    </Button>
                </>
            )}
        </div>
    );
}

function ImportFailures({ failures }: { failures: ImportOutcome['failures'] }) {
    if (failures.length === 0) return null;
    return (
        <ul className="space-y-2 text-destructive">
            {failures.map((failure) => (
                <li key={failure.feedUrl} className="break-words">
                    {failure.feedUrl}: {failure.message}
                </li>
            ))}
        </ul>
    );
}
