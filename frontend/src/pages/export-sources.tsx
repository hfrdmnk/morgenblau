import { useState } from 'react';
import { toast } from 'sonner';

import { DownloadIcon } from '@/components/icons';
import { Button } from '@/components/ui/button';
import { DataSettings } from '@/layouts/data-settings';
import { api, describeMutationError } from '@/lib/api';

export function ExportSources() {
    const [busy, setBusy] = useState(false);

    async function download() {
        setBusy(true);
        try {
            const { opml } = await api<{ opml: string }>(
                '/api/subscriptions/export',
                { method: 'POST' },
            );
            const url = URL.createObjectURL(
                new Blob([opml], { type: 'text/x-opml;charset=utf-8' }),
            );
            const link = document.createElement('a');
            link.href = url;
            link.download = 'morgenblau.opml';
            document.body.append(link);
            link.click();
            link.remove();
            setTimeout(() => URL.revokeObjectURL(url), 1000);
            toast.success('Your OPML download is ready.');
        } catch (error) {
            toast.error(
                describeMutationError(
                    error,
                    'Could not export your sources. Please try again.',
                ),
            );
        } finally {
            setBusy(false);
        }
    }

    return (
        <DataSettings page="export">
            <h1 className="text-2xl font-medium">Export sources</h1>
            <p className="mt-3 max-w-xl text-muted-foreground">
                Take your feeds to another reader, or keep a copy for yourself.
            </p>
            <section className="mt-12" aria-labelledby="export-opml-title">
                <h2 id="export-opml-title" className="text-lg font-medium">
                    OPML file
                </h2>
                <p className="mt-2 max-w-lg text-sm text-muted-foreground">
                    Your RSS and Atom feeds, including YouTube feeds. Tags
                    become folders; a source with several tags appears in each
                    folder.
                </p>
                <Button
                    className="mt-5"
                    variant="secondary"
                    disabled={busy}
                    onClick={() => void download()}
                >
                    <DownloadIcon />{' '}
                    {busy ? 'Preparing export…' : 'Download OPML'}
                </Button>
            </section>
            <p className="mt-12 max-w-lg text-xs text-subtle-foreground">
                Newsletters and native Standardfeed subscriptions are not
                included. Your Standardfeed subscriptions remain on your PDS.
                Exporting does not change your sources.
            </p>
        </DataSettings>
    );
}
