import { useReducedMotion } from 'motion/react';
import { useEffect, useState, type MouseEvent } from 'react';
import { toast } from 'sonner';

export function useCopy(text: string, failure: { title: string; description: string }) {
    const [copied, setCopied] = useState(false);
    const [keyboard, setKeyboard] = useState(false);
    const reduced = useReducedMotion();
    useEffect(() => {
        if (!copied) return;
        const timer = window.setTimeout(() => setCopied(false), 1500);
        return () => window.clearTimeout(timer);
    }, [copied]);
    async function copy(event: MouseEvent) {
        setKeyboard(event.detail === 0);
        try {
            await navigator.clipboard.writeText(text);
            setCopied(true);
        } catch {
            toast.error(failure.title, { description: failure.description });
        }
    }
    return { copied, copy, instant: Boolean(reduced) || keyboard };
}
