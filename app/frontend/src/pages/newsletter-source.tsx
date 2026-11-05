import { Skeleton } from 'skullet/react';
import { useLocation, useParams } from 'wouter';

import { EditNewsletterSheet } from '@/components/add-source-dialog';
import { SourceDetailPage, SourceEntries, SourceHeader, SourceUnavailable } from '@/components/source-detail';
import { useDocumentTitle } from '@/hooks/use-document-title';
import { useSources, type NewsletterSourceDetail, type SourceEntry } from '@/hooks/use-sources';
import { newsletterSourceHref, PATHS } from '@/lib/paths';
import { newsletterFacts } from '@/lib/source-display';

export function NewsletterSource() {
    const { id = '' } = useParams<{ id: string }>();
    const [, navigate] = useLocation();
    const path = `/api/newsletters/${encodeURIComponent(id)}`;
    const source = useSources<NewsletterSourceDetail>(path);
    const entries = useSources<SourceEntry[]>(`${path}/entries`);
    useDocumentTitle(source.status === 'loaded' ? newsletterTitle(source.data) : 'Source');

    return (
        <SourceDetailPage>
            <Skeleton
                data={source.status === 'loaded' ? source.data : null}
                error={source.status === 'error' ? <SourceUnavailable label="this newsletter" state={source} /> : undefined}
                id="newsletter-source"
                label="Loading this newsletter"
                sample={SAMPLE_NEWSLETTER}
            >
                {(data) => (
                    <>
                        <NewsletterHeader onChanged={() => {
                            source.refresh();
                            entries.refresh();
                        }} onRemoved={() => navigate(PATHS.sources)} source={data} />
                        <SourceEntries empty={data.status === 'stopped' ? 'Unsaved issues were removed when this newsletter stopped.' : 'No issues have arrived yet.'} from={newsletterSourceHref(id)} state={entries} title="Issues" />
                    </>
                )}
            </Skeleton>
        </SourceDetailPage>
    );
}

const SAMPLE_NEWSLETTER: NewsletterSourceDetail = {
    id: 'sample', title: 'Example Newsletter', senderAddress: 'hello@example.com', senderName: null, primary: false, canUnsubscribe: false, tags: [],
    status: 'active', lastReceivedAt: '2026-01-01T00:00:00Z', frequency: 'weekly', issueCount: 12,
};

function newsletterTitle(source: NewsletterSourceDetail): string {
    return source.title || source.senderAddress;
}

function NewsletterHeader({ source, onChanged, onRemoved }: { source: NewsletterSourceDetail; onChanged: () => void; onRemoved: () => void }) {
    const title = newsletterTitle(source);
    return (
        <SourceHeader
            facts={newsletterFacts(source)}
            menu={<EditNewsletterSheet onChanged={onChanged} onRemoved={onRemoved} source={source} stopped={source.status === 'stopped'} />}
            identity={title === source.senderAddress ? null : <span className="break-all">{source.senderAddress}</span>}
            tags={source.tags}
            title={title}
        />
    );
}
