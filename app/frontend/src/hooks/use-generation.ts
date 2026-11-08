import { useEffect, useRef } from 'react';

import {
    canRetryGeneration,
    generateSource,
    generationError,
    type GeneratedSource,
} from '@/lib/add-source';

export type GenerationStep =
    | { step: 'pending' }
    | { step: 'failed'; message: string; retry: boolean }
    | { step: 'preview'; source: GeneratedSource; anchored: boolean };

// The steps live with the screen that shows them, so the sheet's header can name the feed once it exists.
export function useGeneration(onStep: (step: GenerationStep) => void) {
    const request = useRef<AbortController | null>(null);
    useEffect(() => () => request.current?.abort(), []);

    function begin() {
        request.current?.abort();
        const controller = new AbortController();
        request.current = controller;
        return controller;
    }

    async function generate(url: string) {
        const controller = begin();
        onStep({ step: 'pending' });
        const next = await generateSource(url, undefined, controller.signal).then(
            (source): GenerationStep => ({
                step: 'preview',
                source,
                anchored: false,
            }),
            (error): GenerationStep => ({
                step: 'failed',
                message: generationError(error),
                retry: canRetryGeneration(error),
            }),
        );
        if (!controller.signal.aborted) onStep(next);
    }

    // Rejects so the anchor form can keep the current preview and show why.
    async function pointAt(url: string, anchor: string) {
        const controller = begin();
        const source = await generateSource(url, anchor, controller.signal);
        if (!controller.signal.aborted)
            onStep({ step: 'preview', source, anchored: true });
    }

    return { generate, pointAt };
}
