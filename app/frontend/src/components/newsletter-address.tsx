import { useEffect, useId, useState } from 'react';
import { toast } from 'sonner';
import { Link as RouterLink } from 'wouter';

import { CheckIcon, CopyIcon, LoadingIcon } from '@/components/icons';
import { Button } from '@/components/ui/button';
import { useAppProfile } from '@/hooks/use-app-profile';
import { newsletterAddress } from '@/lib/add-source';
import { ApiError } from '@/lib/api';
import { PATHS } from '@/lib/paths';
import { cn } from '@/lib/utils';

type AddressStatus = 'loading' | 'ready' | 'failed';

export function NewsletterAddress({
    settings = false,
    onNavigate,
}: {
    settings?: boolean;
    onNavigate?: () => void;
}) {
    const heading = useId();
    const profile = useAppProfile();
    const profileReady = profile.kind === 'ready';
    const [address, setAddress] = useState('');
    const [status, setStatus] = useState<AddressStatus>('loading');
    const [error, setError] = useState('');
    useEffect(() => {
        if (!profileReady) return;
        const controller = new AbortController();
        void newsletterAddress(controller.signal)
            .then((address) => {
                if (!address)
                    throw new Error('Newsletter address not provisioned');
                if (!controller.signal.aborted) {
                    setAddress(address);
                    setStatus('ready');
                }
            })
            .catch((error) => {
                if (!controller.signal.aborted) {
                    setStatus('failed');
                    setError(newsletterErrorMessage(error));
                }
            });
        return () => controller.abort();
    }, [profileReady]);

    return (
        <section
            aria-labelledby={heading}
            className={settings ? '' : 'border-t border-border/60 pt-6'}
        >
            <h2
                className={settings ? 'text-lg font-medium' : 'font-medium'}
                id={heading}
            >
                Newsletters
            </h2>
            <p className="mt-1 text-sm text-muted-foreground">
                Subscribe with your private email address.
            </p>
            <NewsletterAddressContent
                status={status}
                address={address}
                error={error}
                settings={settings}
                onNavigate={onNavigate}
            />
            <p className="mt-3 text-xs text-muted-foreground">
                {settings &&
                    'This address is generated for you and is never published on your PDS. '}
                Newsletters appear after their first email arrives.
            </p>
        </section>
    );
}

function newsletterErrorMessage(error: unknown) {
    return error instanceof ApiError &&
        (error.code === 'unavailable' || [404, 405].includes(error.status))
        ? 'Newsletter delivery isn’t enabled on this server yet.'
        : 'Your newsletter address isn’t available right now. Try again in a moment.';
}

function NewsletterAddressContent({
    status,
    address,
    error,
    settings,
    onNavigate,
}: {
    status: AddressStatus;
    address: string;
    error: string;
    settings: boolean;
    onNavigate?: () => void;
}) {
    if (status === 'ready') return <CopyNewsletterAddress address={address} />;
    if (status === 'loading')
        return (
            <div
                className="mt-4 flex min-h-14 items-center justify-center rounded-lg bg-input text-muted-foreground"
                role="status"
            >
                <LoadingIcon className="size-5 motion-safe:animate-spin" />
                <span className="sr-only">Loading your newsletter address</span>
            </div>
        );
    if (settings)
        return (
            <div className="mt-4">
                <p className="text-sm text-muted-foreground" role="alert">
                    {error}
                </p>
                <Button
                    className="mt-3"
                    variant="secondary"
                    onClick={() => window.location.reload()}
                >
                    Retry
                </Button>
            </div>
        );
    return (
        <RouterLink
            href={PATHS.general}
            onClick={onNavigate}
            className="relative mt-4 block min-h-14 overflow-hidden rounded-lg bg-input text-sm"
        >
            <span
                aria-hidden="true"
                className="flex min-h-14 items-center justify-between gap-2 px-3 text-subtle-foreground blur-[3px] select-none"
            >
                your-address@news.example.com
                <CopyIcon className="size-4 shrink-0" />
            </span>
            <span className="absolute inset-0 flex items-center justify-center bg-input/50 px-3 text-center font-medium">
                View your newsletter settings
            </span>
        </RouterLink>
    );
}

function CopyNewsletterAddress({ address }: { address: string }) {
    const [copied, setCopied] = useState(false);
    async function copy() {
        try {
            await navigator.clipboard.writeText(address);
            setCopied(true);
        } catch {
            toast.error('Couldn’t copy the address', {
                description: 'Select the address and copy it manually.',
            });
        }
    }
    return (
        <div className="mt-4 flex items-center gap-2 rounded-lg bg-input py-1 pr-1 pl-3">
            <input
                aria-label="Newsletter email address"
                className="min-w-0 flex-1 bg-transparent text-sm"
                onFocus={(event) => event.target.select()}
                readOnly
                value={address}
            />
            <Button
                aria-label={
                    copied
                        ? 'Newsletter address copied'
                        : 'Copy newsletter email address'
                }
                className={cn({ 'text-success': copied })}
                onClick={copy}
                size="icon-lg"
                variant="ghost"
            >
                {copied ? <CheckIcon /> : <CopyIcon />}
            </Button>
            <span aria-live="polite" className="sr-only">
                {copied ? 'Newsletter address copied.' : ''}
            </span>
        </div>
    );
}
