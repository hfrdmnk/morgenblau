import { useState } from 'react';
import { toast } from 'sonner';

import { DownloadIcon } from '@/components/icons';
import { Button } from '@/components/ui/button';
import { Link } from '@/components/ui/link';
import { Frame } from '@/design/frame';
import { Text } from '@/design/text';
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
            toast.error(describeMutationError(error, 'Couldn’t export your sources. Try again.'));
        } finally {
            setBusy(false);
        }
    }

    return (
        <DataSettings>
            <Frame gap="5xl">
                <Frame gap="m">
                    <Text as="h1" size="2xl" weight="medium">
                        Export sources
                    </Text>
                    <Frame maxWidth="xl">
                        <Text tone="muted">Take your feeds to another reader, or keep a copy for yourself.</Text>
                    </Frame>
                </Frame>
                <Frame gap="m">
                    <Frame as="section" aria-labelledby="export-opml-title" gap="l">
                        <Frame gap="2xs" maxWidth="lg">
                            <Text as="h2" id="export-opml-title" size="lg" weight="medium">
                                OPML file
                            </Text>
                            <Text size="sm" tone="muted">
                                The file has your RSS and Atom feeds, YouTube channels included. Each tag becomes a
                                folder, and a source with several tags shows up in each of them.
                            </Text>
                        </Frame>
                        <Frame direction="row">
                            <Button variant="secondary" disabled={busy} onClick={() => void download()}>
                                <DownloadIcon /> {busy ? 'Preparing export…' : 'Download OPML'}
                            </Button>
                        </Frame>
                    </Frame>
                    <Frame maxWidth="lg">
                        <Text size="xs" tone="subtle">
                            The file leaves out newsletters and native Standardfeed subscriptions. Your Standardfeed
                            subscriptions stay in your PDS.
                        </Text>
                    </Frame>
                </Frame>
                <PublicFollowing />
            </Frame>
        </DataSettings>
    );
}

function PublicFollowing() {
    const { enabled, busy, toggle } = usePublicFollowing();
    return (
        <Frame as="section" aria-labelledby="public-following-title" gap="l">
            <Frame gap="2xs" maxWidth="lg">
                <Text as="h2" id="public-following-title" size="lg" weight="medium">
                    Public list
                </Text>
                <Text size="sm" tone="muted">
                    Publish the feeds you follow at a public address, for a blogroll on your site. Anyone with the
                    address sees each feed’s title, link and tags. The list never includes newsletters.
                </Text>
            </Frame>
            {enabled && <PublishedList />}
            <Frame direction="row">
                <Button variant="secondary" disabled={busy || enabled === undefined} onClick={() => void toggle()}>
                    {enabled ? 'Stop publishing' : 'Publish list'}
                </Button>
            </Frame>
        </Frame>
    );
}

function PublishedList() {
    const profile = useAppProfile();
    if (profile.kind !== 'ready') return null;
    const base = `${window.location.origin}/user/${profile.profile.did}/following`;
    return (
        <Frame gap="m">
            <Frame as="ul" gap="s">
                {(['opml', 'json'] as const).map((format) => (
                    <Frame as="li" key={format}>
                        <Text size="sm" break="all">
                            <Link
                                href={`${base}.${format}`}
                                rel="noopener noreferrer"
                                target="_blank"
                                variant="underline"
                            >
                                {`${base}.${format}`}
                            </Link>
                        </Text>
                    </Frame>
                ))}
            </Frame>
            <Frame maxWidth="lg">
                <Text size="xs" tone="subtle">
                    Narrow the list with <Text as="code">?tag=</Text>, <Text as="code">excludeTag=</Text>,{' '}
                    <Text as="code">kind=</Text> or <Text as="code">excludeKind=</Text> (rss, youtube, standardfeed),{' '}
                    <Text as="code">primary=true</Text>, and <Text as="code">folders=none</Text> for a flat OPML file.
                </Text>
            </Frame>
        </Frame>
    );
}
