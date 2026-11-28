import { useState } from 'react';

import { ReaderNotice } from '@/components/reader-notice';
import { Button } from '@/components/ui/button';
import { Frame } from '@/design/frame';
import { Text } from '@/design/text';
import { allowRemoteImages, type ReaderEntry } from '@/lib/reader';

export function NewsletterImages({
    entry,
    newsletter,
    onChange,
}: {
    entry: ReaderEntry;
    newsletter: NonNullable<ReaderEntry['newsletter']>;
    onChange: (entry: ReaderEntry) => void;
}) {
    const [pending, setPending] = useState(false);
    const [notice, setNotice] = useState('');

    async function allowImages() {
        setPending(true);
        setNotice('');
        try {
            onChange(await allowRemoteImages(entry, newsletter));
        } catch {
            setNotice('Couldn’t load the images. Try again.');
        } finally {
            setPending(false);
        }
    }

    const blocked = newsletter.hasBlockedRemoteImages && !newsletter.remoteImagesAllowed;
    return (
        <Frame gap="xl" paddingBottom={blocked || notice ? '4xl' : 'none'}>
            {blocked ? (
                <Frame
                    align="center"
                    as="aside"
                    background="muted-soft"
                    direction="row"
                    gap="l"
                    justify="between"
                    padding="l"
                    radius="lg"
                    wrap
                >
                    <Frame gap="2xs" maxWidth="md">
                        <Text size="sm">Remote images are blocked</Text>
                        <Text size="sm" tone="muted">
                            Loading them may tell the sender you opened this message. This choice applies only to this
                            message.
                        </Text>
                    </Frame>
                    <Button disabled={pending} onClick={() => void allowImages()} variant="secondary">
                        {pending ? 'Loading images…' : 'Load images'}
                    </Button>
                </Frame>
            ) : null}
            <ReaderNotice notice={notice} />
        </Frame>
    );
}
