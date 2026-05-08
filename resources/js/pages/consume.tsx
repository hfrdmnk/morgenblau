import { RefreshIcon } from '@hugeicons/core-free-icons';
import { HugeiconsIcon } from '@hugeicons/react';
import { Form, Head } from '@inertiajs/react';
import TextLink from '@/components/text-link';
import { Button } from '@/components/ui/button';
import { discover } from '@/routes';
import { refresh } from '@/routes/feeds';

type FeedEntry = App.Data.Feeds.FeedEntryViewData;

type ConsumeProps = {
    entries: FeedEntry[];
    has_subscriptions: boolean;
};

export default function Consume({ entries, has_subscriptions }: ConsumeProps) {
    const isEmpty = entries.length === 0;

    return (
        <>
            <Head title="Consume" />
            <div className="mx-auto w-full max-w-2xl px-6 py-10">
                {has_subscriptions ? (
                    <div className="mb-8 flex items-center justify-end">
                        <RefreshButton />
                    </div>
                ) : null}
                {isEmpty ? (
                    <EmptyState hasSubscriptions={has_subscriptions} />
                ) : (
                    <ul className="flex flex-col gap-8">
                        {entries.map((entry) => (
                            <EntryRow key={entry.id} entry={entry} />
                        ))}
                    </ul>
                )}
            </div>
        </>
    );
}

function RefreshButton() {
    return (
        <Form {...refresh.form()} options={{ preserveScroll: true }}>
            {({ processing }) => (
                <Button
                    type="submit"
                    variant="secondary"
                    size="sm"
                    disabled={processing}
                >
                    <HugeiconsIcon icon={RefreshIcon} />
                    Refresh now
                </Button>
            )}
        </Form>
    );
}

function EmptyState({ hasSubscriptions }: { hasSubscriptions: boolean }) {
    if (!hasSubscriptions) {
        return (
            <div className="flex flex-col gap-2">
                <p>No sources yet.</p>
                <p className="font-handwritten text-sm text-muted-foreground">
                    Pick one over at{' '}
                    <TextLink href={discover().url}>/discover</TextLink>.
                </p>
            </div>
        );
    }

    return (
        <div className="flex flex-col gap-2">
            <p>Nothing new this morning.</p>
            <p className="font-handwritten text-sm text-muted-foreground">
                Enjoy your coffee.
            </p>
        </div>
    );
}

function EntryRow({ entry }: { entry: FeedEntry }) {
    const meta = formatMeta(entry);

    return (
        <li className="flex flex-col gap-2">
            <p className="font-handwritten text-sm text-muted-foreground">
                {entry.display_title ?? 'Untitled source'}
            </p>
            {entry.entry_title ? (
                entry.link ? (
                    <a
                        href={entry.link}
                        target="_blank"
                        rel="noreferrer noopener"
                        className="text-lg font-medium tracking-tight text-foreground hover:underline"
                    >
                        {entry.entry_title}
                    </a>
                ) : (
                    <h3 className="text-foreground">{entry.entry_title}</h3>
                )
            ) : null}
            {entry.summary ? (
                <p className="text-sm text-muted-foreground">{entry.summary}</p>
            ) : null}
            {meta ? (
                <p className="font-handwritten text-sm text-muted-foreground">
                    {meta}
                </p>
            ) : null}
        </li>
    );
}

function formatMeta(entry: FeedEntry): string | null {
    const bits: string[] = [];

    if (entry.author) {
        bits.push(entry.author);
    }

    const when = entry.published_at ?? entry.first_seen_at;

    if (when) {
        bits.push(formatRelative(when));
    }

    return bits.length > 0 ? bits.join(' · ') : null;
}

function formatRelative(iso: string): string {
    const then = new Date(iso);
    const now = new Date();
    const diffSeconds = Math.round((now.getTime() - then.getTime()) / 1000);

    if (diffSeconds < 60) {
        return 'just now';
    }

    if (diffSeconds < 3600) {
        const m = Math.round(diffSeconds / 60);

        return `${m} min ago`;
    }

    if (diffSeconds < 86400) {
        const h = Math.round(diffSeconds / 3600);

        return `${h} hr ago`;
    }

    const d = Math.round(diffSeconds / 86400);

    return `${d} day${d === 1 ? '' : 's'} ago`;
}
