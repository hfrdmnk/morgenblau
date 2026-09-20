import { CopyIcon, MailIcon, SpinnerIcon } from '@proicons/react';
import { useEffect, useState } from 'react';

import { Button } from '@/components/ui/button';
import { api } from '@/lib/api';

type AddressState =
    | { kind: 'loading' }
    | { kind: 'ready'; address: string }
    | { kind: 'unavailable' };

const loadingState: AddressState = { kind: 'loading' };

async function loadAddress(signal: AbortSignal): Promise<AddressState> {
    try {
        const { address } = await api<{ address?: string }>(
            '/api/newsletters/address',
            { signal },
        );
        const trimmed = address?.trim();
        return trimmed
            ? { kind: 'ready', address: trimmed }
            : { kind: 'unavailable' };
    } catch {
        return { kind: 'unavailable' };
    }
}

function useNewsletterAddress(open: boolean) {
    const [state, setState] = useState<AddressState>(loadingState);

    if (!open && state.kind !== 'loading') setState(loadingState);

    useEffect(() => {
        if (!open) return;
        const abort = new AbortController();
        void loadAddress(abort.signal).then((nextState) => {
            if (!abort.signal.aborted) setState(nextState);
        });
        return () => abort.abort();
    }, [open]);

    return state;
}

function CopyAddress({ address }: { address: string }) {
    const [copied, setCopied] = useState(false);

    const copy = async () => {
        try {
            await navigator.clipboard.writeText(address);
            setCopied(true);
            window.setTimeout(() => setCopied(false), 1800);
        } catch {
            setCopied(false);
        }
    };

    return (
        <div className="flex items-center gap-2 rounded-xl bg-muted p-1 pl-3">
            <span className="min-w-0 flex-1 truncate text-body select-all">
                {address}
            </span>
            <Button
                type="button"
                variant="secondary"
                size="sm"
                onClick={copy}
            >
                <CopyIcon className="size-4" />
                {copied ? 'Copied' : 'Copy'}
            </Button>
        </div>
    );
}

function AddressContent({ state }: { state: AddressState }) {
    if (state.kind === 'loading') {
        return (
            <div className="flex min-h-10 items-center gap-2 rounded-xl bg-muted px-3 text-label text-muted-foreground">
                <SpinnerIcon className="size-4 motion-safe:animate-spin" />
                Preparing your address…
            </div>
        );
    }
    if (state.kind === 'ready') {
        return <CopyAddress address={state.address} />;
    }
    return (
        <p className="rounded-xl bg-muted px-3 py-2.5 text-label text-muted-foreground">
            Newsletter delivery is unavailable right now.
        </p>
    );
}

export function NewsletterAddress({ open }: { open: boolean }) {
    const state = useNewsletterAddress(open);

    return (
        <section
            className="space-y-2"
            aria-labelledby="newsletter-address-heading"
        >
            <div className="flex items-center gap-2">
                <MailIcon className="size-4 text-muted-foreground" />
                <h2 id="newsletter-address-heading" className="text-heading">
                    Newsletters
                </h2>
            </div>
            <p className="text-label text-muted-foreground">
                Use your private address when subscribing. The first email
                creates the source automatically.
            </p>
            <AddressContent state={state} />
        </section>
    );
}
