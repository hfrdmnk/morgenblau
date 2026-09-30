import { useEffect, useRef, useState } from 'react';
import { toast } from 'sonner';

import { api, describeMutationError } from '@/lib/api';
import {
    importSources,
    type ImportOutcome,
    type ImportPlan,
} from '@/lib/source-import';

export type ImportState =
    | { kind: 'idle' }
    | { kind: 'preparing'; provider: string }
    | { kind: 'confirm'; plan: ImportPlan }
    | { kind: 'importing'; completed: number; total: number }
    | { kind: 'done'; outcome: ImportOutcome };

async function readOPML(file?: File): Promise<string | undefined> {
    if (file && file.size > 512 * 1024)
        throw new Error('Choose an OPML file smaller than 512 KB.');
    return file?.text();
}

function preparationError(error: unknown) {
    return describeMutationError(
        error,
        error instanceof Error
            ? error.message
            : 'Could not read these sources. Please try again.',
    );
}

function confirmPlan(plan: ImportPlan): ImportState {
    if (plan.sources.length > 0) return { kind: 'confirm', plan };
    toast('No sources to import');
    return { kind: 'idle' };
}

export function useSourceImport() {
    const [state, setState] = useState<ImportState>({ kind: 'idle' });
    const request = useRef<AbortController | null>(null);
    useEffect(() => () => request.current?.abort(), []);

    async function prepare(
        provider: 'opml' | 'skyreader' | 'glean',
        file?: File,
    ) {
        if (request.current) return;
        const controller = new AbortController();
        request.current = controller;
        setState({ kind: 'preparing', provider });
        try {
            const opml = await readOPML(file);
            const plan = await api<ImportPlan>(
                '/api/subscriptions/import/prepare',
                {
                    method: 'POST',
                    body: { provider, opml },
                    signal: controller.signal,
                },
            );
            controller.signal.throwIfAborted();
            setState(confirmPlan(plan));
        } catch (error) {
            if (controller.signal.aborted) return;
            toast.error(preparationError(error));
            setState({ kind: 'idle' });
        } finally {
            request.current = null;
        }
    }

    async function startImport(
        plan: ImportPlan,
        previous = { added: 0, updated: 0, unchanged: 0 },
    ) {
        if (request.current) return;
        const controller = new AbortController();
        request.current = controller;
        setState({
            kind: 'importing',
            completed: 0,
            total: plan.sources.length,
        });
        const outcome = await importSources(
            plan.sources,
            controller.signal,
            (completed) => {
                setState({
                    kind: 'importing',
                    completed,
                    total: plan.sources.length,
                });
            },
        );
        request.current = null;
        if (controller.signal.aborted) return;
        const combined = {
            ...outcome,
            added: previous.added + outcome.added,
            updated: previous.updated + outcome.updated,
            unchanged: previous.unchanged + outcome.unchanged,
        };
        if (combined.remaining.length > 0) {
            setState({ kind: 'done', outcome: combined });
            toast.error('Import paused', {
                description: 'Some sources could not be imported. Retry them in the dialog.',
            });
        } else {
            toast.success('Import complete', {
                description: `${combined.added} added, ${combined.updated} updated, ${combined.unchanged} already up to date.`,
            });
            setState({ kind: 'idle' });
        }
    }

    return {
        state,
        prepare,
        startImport,
        cancel: () => setState({ kind: 'idle' }),
    };
}
