import { SpinnerIcon } from '@proicons/react';
import { useCallback, useEffect, useState } from 'react';

import { NewsletterAddressField } from '@/components/newsletters/newsletter-address';
import { Button } from '@/components/ui/button';
import { useDocumentTitle } from '@/hooks/use-document-title';
import {
    createNewsletterAddress,
    fetchNewsletterAddress,
} from '@/lib/newsletters';

type AddressState =
    | { kind: 'loading' }
    | { kind: 'unset' }
    | { kind: 'ready'; address: string }
    | { kind: 'error' };

export function Settings() {
    useDocumentTitle('Settings');
    const [state, setState] = useState<AddressState>({ kind: 'loading' });
    const [creating, setCreating] = useState(false);

    const loadAddress = useCallback((signal?: AbortSignal) => {
        void fetchNewsletterAddress(signal)
            .then(({ address }) => {
                const trimmed = address?.trim();
                setState(
                    trimmed
                        ? { kind: 'ready', address: trimmed }
                        : { kind: 'unset' },
                );
            })
            .catch((error: unknown) => {
                if ((error as Error).name !== 'AbortError') {
                    setState({ kind: 'error' });
                }
            });
    }, []);

    const retryAddress = () => {
        setState({ kind: 'loading' });
        loadAddress();
    };

    useEffect(() => {
        const abort = new AbortController();
        loadAddress(abort.signal);
        return () => abort.abort();
    }, [loadAddress]);

    const createAddress = async () => {
        setCreating(true);
        try {
            const { address } = await createNewsletterAddress();
            const trimmed = address?.trim();
            setState(
                trimmed
                    ? { kind: 'ready', address: trimmed }
                    : { kind: 'error' },
            );
        } catch {
            setState({ kind: 'error' });
        } finally {
            setCreating(false);
        }
    };

    return (
        <div className="mx-auto w-full max-w-2xl px-4 pt-10 pb-12 sm:px-6">
            <h1>Settings</h1>

            <section
                className="mt-10 space-y-4"
                aria-labelledby="newsletter-settings-heading"
            >
                <div className="space-y-1">
                    <h2
                        id="newsletter-settings-heading"
                        className="text-heading"
                    >
                        Newsletter
                    </h2>
                    <p className="text-label text-muted-foreground">
                        Use your private address when subscribing. The first
                        email creates the source automatically.
                    </p>
                </div>

                <NewsletterAddressSetting
                    state={state}
                    creating={creating}
                    onCreate={createAddress}
                    onRetry={retryAddress}
                />
            </section>
        </div>
    );
}

function NewsletterAddressSetting({
    state,
    creating,
    onCreate,
    onRetry,
}: {
    state: AddressState;
    creating: boolean;
    onCreate: () => void;
    onRetry: () => void;
}) {
    if (state.kind === 'ready') {
        return <NewsletterAddressField address={state.address} />;
    }
    if (state.kind === 'loading') {
        return (
            <div className="flex min-h-10 items-center gap-2 text-label text-muted-foreground">
                <SpinnerIcon className="size-4 motion-safe:animate-spin" />
                Loading address…
            </div>
        );
    }
    if (state.kind === 'error') {
        return (
            <div className="flex items-center gap-3">
                <p className="text-label text-muted-foreground">
                    Couldn’t load your newsletter address.
                </p>
                <Button type="button" variant="secondary" onClick={onRetry}>
                    Try again
                </Button>
            </div>
        );
    }
    return (
        <CreateNewsletterAddressButton
            creating={creating}
            onCreate={onCreate}
        />
    );
}

function CreateNewsletterAddressButton({
    creating,
    onCreate,
}: {
    creating: boolean;
    onCreate: () => void;
}) {
    return (
        <Button type="button" onClick={onCreate} disabled={creating}>
            {creating ? (
                <>
                    <SpinnerIcon className="motion-safe:animate-spin" />
                    Creating…
                </>
            ) : (
                'Create address'
            )}
        </Button>
    );
}
