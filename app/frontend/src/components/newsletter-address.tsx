import { useEffect, useId, useState } from 'react';

import { IconCrossfade } from '@/components/icon-crossfade';
import { CheckIcon, CopyIcon } from '@/components/icons';
import { PdsLink } from '@/components/public-data-hint';
import { Skeleton } from '@/components/ui/skeleton';
import { Button } from '@/components/ui/button';
import { InputGroup, InputGroupButton, InputGroupInput } from '@/components/ui/input-group';
import { VeiledFieldLink } from '@/components/ui/veiled-field';
import { Frame } from '@/design/frame';
import { Text } from '@/design/text';
import { useAppProfile } from '@/hooks/use-app-profile';
import { useCopy } from '@/hooks/use-copy';
import { newsletterAddress } from '@/lib/add-source';
import { ApiError } from '@/lib/api';
import { PATHS } from '@/lib/paths';

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
        <Frame
            as="section"
            aria-labelledby={heading}
            border={settings ? undefined : 'top'}
            gap="l"
            paddingTop={settings ? undefined : '2xl'}
        >
            <Frame gap="2xs">
                <Text as="h2" id={heading} size={settings ? 'lg' : undefined} weight="medium">
                    Newsletters
                </Text>
                <Text size="sm" tone="muted">
                    Subscribe with your private email address.
                </Text>
            </Frame>
            <Frame gap="m">
                <NewsletterAddressContent
                    status={status}
                    address={address}
                    error={error}
                    settings={settings}
                    onNavigate={onNavigate}
                />
                <Text size="xs" tone="subtle" wrap="pretty">
                    {settings && 'This address is generated for you. '}
                    Newsletters are always private and not saved to your <PdsLink />.
                    They appear after their first email arrives.
                </Text>
            </Frame>
        </Frame>
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
    if (status !== 'failed')
        return (
            <Skeleton
                data={status === 'ready' ? address : null}
                label="Loading your newsletter address"
                sample="your-address@news.example.com"
            >
                {(value) => <CopyNewsletterAddress address={value} />}
            </Skeleton>
        );
    if (settings)
        return (
            <Frame align="start" gap="m">
                <Text role="alert" size="sm" tone="muted">
                    {error}
                </Text>
                <Button variant="secondary" onClick={() => window.location.reload()}>
                    Retry
                </Button>
            </Frame>
        );
    return (
        <VeiledFieldLink
            href={PATHS.general}
            icon={<CopyIcon size="md" />}
            onClick={onNavigate}
            value="your-address@news.example.com"
        >
            View your newsletter settings
        </VeiledFieldLink>
    );
}

const COPY_ADDRESS_FAILURE = {
    title: 'Couldn’t copy the address',
    description: 'Select the address and copy it manually.',
};

// The crossfade sizes its icon for a 20px slot; the copy button's icons stay at its 16px.
function CopyGlyph() {
    return <CopyIcon size="md" />;
}

function CheckGlyph() {
    return <CheckIcon size="md" />;
}

function CopyNewsletterAddress({ address }: { address: string }) {
    const { copied, copy, instant } = useCopy(address, COPY_ADDRESS_FAILURE);
    return (
        <InputGroup>
            <InputGroupInput
                aria-label="Newsletter email address"
                onFocus={(event) => event.target.select()}
                readOnly
                value={address}
            />
            <InputGroupButton
                aria-label={
                    copied
                        ? 'Newsletter address copied'
                        : 'Copy newsletter email address'
                }
                onClick={copy}
                tone={copied ? 'success' : undefined}
            >
                <IconCrossfade
                    icon={copied ? CheckGlyph : CopyGlyph}
                    instant={instant}
                    name={copied ? 'check' : 'copy'}
                />
            </InputGroupButton>
            <Text as="span" aria-live="polite" visuallyHidden>
                {copied ? 'Newsletter address copied.' : ''}
            </Text>
        </InputGroup>
    );
}
