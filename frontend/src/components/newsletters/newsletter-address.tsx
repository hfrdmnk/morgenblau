import { CopyIcon, SpinnerIcon } from '@proicons/react';
import { useEffect, useState } from 'react';
import { Link } from 'wouter';

import {
    InputGroup,
    InputGroupAddon,
    InputGroupButton,
    InputGroupInput,
    InputGroupText,
} from '@/components/ui/input-group';
import { fetchNewsletterAddress } from '@/lib/newsletters';
import { PATHS } from '@/lib/paths';

type AddressState =
    | { kind: 'loading' }
    | { kind: 'ready'; address: string }
    | { kind: 'unset' }
    | { kind: 'error' };

const loadingState: AddressState = { kind: 'loading' };

async function loadAddress(signal: AbortSignal): Promise<AddressState> {
    try {
        const { address } = await fetchNewsletterAddress(signal);
        const trimmed = address?.trim();
        return trimmed
            ? { kind: 'ready', address: trimmed }
            : { kind: 'unset' };
    } catch {
        return { kind: 'error' };
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

export function NewsletterAddressField({ address }: { address: string }) {
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
        <InputGroup>
            <InputGroupInput
                aria-label="Newsletter address"
                readOnly
                value={address}
                onFocus={(event) => event.currentTarget.select()}
            />
            <InputGroupAddon align="inline-end">
                <InputGroupButton onClick={copy}>
                    <CopyIcon />
                    {copied ? 'Copied' : 'Copy'}
                </InputGroupButton>
            </InputGroupAddon>
        </InputGroup>
    );
}

function AddressContent({
    state,
    onSetup,
}: {
    state: AddressState;
    onSetup: () => void;
}) {
    if (state.kind === 'loading') {
        return (
            <InputGroup>
                <InputGroupAddon>
                    <SpinnerIcon className="motion-safe:animate-spin" />
                </InputGroupAddon>
                <InputGroupText>Loading address…</InputGroupText>
            </InputGroup>
        );
    }
    if (state.kind === 'ready') {
        return <NewsletterAddressField address={state.address} />;
    }
    if (state.kind === 'error') {
        return (
            <p className="flex min-h-10 items-center text-label text-muted-foreground">
                Newsletter delivery is unavailable right now.
            </p>
        );
    }
    return (
        <Link
            href={PATHS.settings}
            onClick={onSetup}
            className="inline-flex min-h-10 items-center text-body text-muted-foreground underline decoration-border underline-offset-4 transition-colors hover:text-foreground focus-visible:outline-1 focus-visible:outline-offset-2 focus-visible:outline-ring focus-visible:outline-solid"
        >
            click here to set an address
        </Link>
    );
}

export function NewsletterAddress({
    open,
    onSetup,
}: {
    open: boolean;
    onSetup: () => void;
}) {
    const state = useNewsletterAddress(open);

    return (
        <section
            className="space-y-2"
            aria-labelledby="newsletter-address-heading"
        >
            <h2
                id="newsletter-address-heading"
                className="text-base font-medium tracking-tight"
            >
                Newsletter
            </h2>
            <AddressContent state={state} onSetup={onSetup} />
        </section>
    );
}
