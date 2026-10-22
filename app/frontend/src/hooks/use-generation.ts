import { useEffect, useRef, useState } from 'react';

import {
    canRetryGeneration,
    generateSource,
    generationError,
    type GeneratedSource,
} from '@/lib/add-source';

export type GenerationStep =
    | { step: 'offer' }
    | { step: 'pending' }
    | { step: 'failed'; message: string; retry: boolean }
    | { step: 'preview'; source: GeneratedSource; anchored: boolean };

export function useGeneration(url: string) {
    const [state, setState] = useState<GenerationStep>({ step: 'offer' });
    const request = useRef<AbortController | null>(null);
    useEffect(() => () => request.current?.abort(), []);

    function begin() {
        request.current?.abort();
        const controller = new AbortController();
        request.current = controller;
        return controller;
    }

    async function generate() {
        const controller = begin();
        setState({ step: 'pending' });
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
        if (!controller.signal.aborted) setState(next);
    }

    // Rejects so the anchor form can keep the current preview and show why.
    async function pointAt(anchor: string) {
        const controller = begin();
        const source = await generateSource(url, anchor, controller.signal);
        if (!controller.signal.aborted)
            setState({ step: 'preview', source, anchored: true });
    }

    return { state, generate, pointAt };
}
