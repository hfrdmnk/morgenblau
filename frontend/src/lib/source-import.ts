import { subscriptionChanges } from '@/lib/add-source';
import { api, describeMutationError } from '@/lib/api';

export type ImportSource = {
    feedUrl: string;
    title?: string;
    siteUrl?: string;
    tags?: string[];
};

export type ImportPlan = { sources: ImportSource[]; warnings: string[] };
type ImportBatch = {
    added: number;
    updated: number;
    unchanged: number;
    failures: { feedUrl: string; message: string }[];
};
export type ImportOutcome = ImportBatch & {
    remaining: ImportSource[];
    imported: ImportSource[];
    error?: string;
};

export async function importSources(
    sources: ImportSource[],
    signal: AbortSignal,
    onProgress: (completed: number) => void,
): Promise<ImportOutcome> {
    const outcome: ImportOutcome = {
        added: 0,
        updated: 0,
        unchanged: 0,
        failures: [],
        remaining: [],
        imported: [],
    };
    for (let offset = 0; offset < sources.length; offset += 5) {
        const batch = sources.slice(offset, offset + 5);
        try {
            const result = await api<ImportBatch>('/api/subscriptions/import', {
                method: 'POST',
                body: { sources: batch },
                signal,
            });
            outcome.added += result.added;
            outcome.updated += result.updated;
            outcome.unchanged += result.unchanged;
            outcome.failures.push(...result.failures);
            const failed = new Set(result.failures.map((failure) => failure.feedUrl));
            outcome.imported.push(...batch.filter((source) => !failed.has(source.feedUrl)));
            if (result.added + result.updated + result.unchanged > 0) {
                subscriptionChanges.dispatchEvent(new Event('change'));
            }
            onProgress(outcome.added + outcome.updated + outcome.unchanged);
            if (result.failures.length) {
                outcome.remaining = [
                    ...batch.filter((source) => failed.has(source.feedUrl)),
                    ...sources.slice(offset + 5),
                ];
                return outcome;
            }
        } catch (error) {
            outcome.error = describeMutationError(
                error,
                'The connection was interrupted. You can retry safely.',
            );
            outcome.remaining = sources.slice(offset);
            return outcome;
        }
    }
    return outcome;
}
