import { useState } from 'react';

import { ReaderNotice } from '@/components/reader-notice';
import { Button } from '@/components/ui/button';
import { api } from '@/lib/api';
import type { ReaderEntry } from '@/lib/reader';

export function NewsletterImages({ entry, newsletter, onChange }: {
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
            const result = await api<{ body: string; remoteImagesAllowed: boolean; hasBlockedRemoteImages: boolean }>(
                `/api/newsletters/messages/${encodeURIComponent(newsletter.messageId)}/images`, { method: 'POST' },
            );
            onChange({ ...entry, body: result.body, newsletter: { ...newsletter, ...result } });
        } catch {
            setNotice('Images could not be loaded. Please try again.');
        } finally {
            setPending(false);
        }
    }

    return (
        <>
            {newsletter.hasBlockedRemoteImages && !newsletter.remoteImagesAllowed ? (
                <aside className="mb-8 flex flex-wrap items-center justify-between gap-4 rounded-lg bg-muted/60 p-4 text-sm">
                    <div className="max-w-md space-y-1"><p>Remote images are blocked</p><p className="text-muted-foreground">Loading them may tell the sender you opened this message. This choice applies only to this message.</p></div>
                    <Button disabled={pending} onClick={() => void allowImages()} variant="secondary">{pending ? 'Loading images…' : 'Load images'}</Button>
                </aside>
            ) : null}

            <ReaderNotice notice={notice} />
        </>
    );
}
