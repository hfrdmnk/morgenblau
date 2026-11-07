import { useState } from 'react';
import { toast } from 'sonner';

import { DownloadIcon } from '@/components/icons';
import { Button } from '@/components/ui/button';
import { useAppProfile } from '@/hooks/use-app-profile';
import { useDocumentTitle } from '@/hooks/use-document-title';
import { usePublicFollowing } from '@/hooks/use-public-following';
import { DataSettings } from '@/layouts/data-settings';
import { describeMutationError } from '@/lib/api';
import { downloadOpml } from '@/lib/opml';

export function ExportSources() {
    const [busy, setBusy] = useState(false);
    useDocumentTitle('Export sources');

    async function download() {
        setBusy(true);
        try {
            toast.success(await downloadOpml());
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
        <DataSettings>
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
            <p className="mt-3 max-w-lg text-xs text-subtle-foreground">
                Newsletters and native Standardfeed subscriptions are not
                included. Your Standardfeed subscriptions remain on your PDS.
                Exporting does not change your sources.
            </p>
            <PublicFollowing />
        </DataSettings>
    );
}

function PublicFollowing() {
    const { enabled, busy, toggle } = usePublicFollowing();
    return (
        <section className="mt-12" aria-labelledby="public-following-title">
            <h2 id="public-following-title" className="text-lg font-medium">
                Public list
            </h2>
            <p className="mt-2 max-w-lg text-sm text-muted-foreground">
                Publish the feeds you follow at a public address, for a
                blogroll on your site. Anyone with the address sees each
                feed’s title, link and tags. Newsletters are never included.
            </p>
            {enabled && <PublishedList />}
            <Button
                className="mt-5"
                variant="secondary"
                disabled={busy || enabled === undefined}
                onClick={() => void toggle()}
            >
                {enabled ? 'Stop publishing' : 'Publish list'}
            </Button>
        </section>
    );
}

function PublishedList() {
    const profile = useAppProfile();
    if (profile.kind !== 'ready') return null;
    const base = `${window.location.origin}/user/${profile.profile.did}/following`;
    return (
        <>
            <ul className="mt-5 space-y-2 text-sm">
                {(['opml', 'json'] as const).map((format) => (
                    <li key={format}>
                        <a
                            className="break-all underline underline-offset-4"
                            href={`${base}.${format}`}
                            rel="noopener noreferrer"
                            target="_blank"
                        >
                            {`${base}.${format}`}
                        </a>
                    </li>
                ))}
            </ul>
            <p className="mt-3 max-w-lg text-xs text-subtle-foreground">
                Narrow the list with <code>?tag=</code>,{' '}
                <code>excludeTag=</code>, <code>kind=</code> or{' '}
                <code>excludeKind=</code> (rss, youtube, standardfeed),{' '}
                <code>primary=true</code>, and{' '}
                <code>folders=none</code> for a flat OPML file.
            </p>
        </>
    );
}
