import { ListError } from '@/components/item-list';
import { PersonAvatar } from '@/components/person-avatar';
import { Skeleton } from '@/components/ui/skeleton';
import { UsageHeader, UsageRow, UsageStats } from '@/components/ui/usage-table';
import { PageSection } from '@/components/page-section';
import { Frame } from '@/design/frame';
import { Text } from '@/design/text';
import { useDocumentTitle } from '@/hooks/use-document-title';
import { useSources } from '@/hooks/use-sources';
import { formatDate } from '@/lib/source-display';

type SourceCounts = { regular: number; generated: number; newsletter: number };

type Person = SourceCounts & {
    did: string;
    handle: string;
    displayName: string | null;
    avatar: string | null;
    lastActiveAt: string | null;
};

type Usage = { totals: SourceCounts & { people: number }; people: Person[] };

const SAMPLE_USAGE: Usage = {
    totals: { people: 3, regular: 24, generated: 2, newsletter: 5 },
    people: ['Example Person', 'Another Example', 'Sample Reader'].map((displayName, index) => ({
        did: `did:plc:sample${index}`, handle: `reader${index}.example`, displayName, avatar: null,
        lastActiveAt: '2026-01-01T00:00:00Z', regular: 8, generated: 1, newsletter: 2,
    })),
};

const COUNT_COLUMNS = [
    { key: 'regular', label: 'Regular' },
    { key: 'generated', label: 'Generated' },
    { key: 'newsletter', label: 'Newsletter' },
] as const;

export function Admin() {
    const usage = useSources<Usage>('/api/admin/usage');
    useDocumentTitle('Usage');
    return (
        <Frame
            as="main"
            aria-label="Admin"
            gap="5xl"
            id="main"
            maxWidth="3xl"
            paddingX={{ base: 'xl', sm: '3xl' }}
            width="full"
        >
            <Text as="h1" visuallyHidden>Usage</Text>
            <Skeleton
                data={usage.status === 'loaded' ? usage.data : null}
                error={usage.status === 'error' ? (
                    usage.notFound ? (
                        <Frame gap="m" role="alert">
                            <Text as="h2" size="2xl" weight="medium">Not found</Text>
                            <Text tone="muted">This page is only available to admins.</Text>
                        </Frame>
                    ) : <ListError label="usage" retry={usage.retry} />
                ) : undefined}
                label="Loading usage"
                sample={SAMPLE_USAGE}
            >
                {(data) => (
                    <Frame gap="5xl">
                        <PageSection title="Overview"><Totals totals={data.totals} /></PageSection>
                        <PageSection title="People"><People people={data.people} /></PageSection>
                    </Frame>
                )}
            </Skeleton>
        </Frame>
    );
}

function Totals({ totals }: { totals: Usage['totals'] }) {
    const stats = [
        { label: 'People', value: totals.people },
        { label: 'Regular feeds', value: totals.regular },
        { label: 'Generated feeds', value: totals.generated },
        { label: 'Newsletters', value: totals.newsletter },
    ];
    return (
        <UsageStats>
            {stats.map(({ label, value }) => (
                <Frame key={label} gap="2xs">
                    <Text as="dt" size="sm" tone="muted">{label}</Text>
                    <Text as="dd" size="3xl" weight="medium" numeric="tabular">{value}</Text>
                </Frame>
            ))}
        </UsageStats>
    );
}

function People({ people }: { people: Person[] }) {
    if (!people.length) return <Text size="sm" tone="muted">Nobody has signed in yet.</Text>;
    return (
        <Frame gap="m">
            <UsageHeader aria-hidden="true">
                <Text as="span" size="xs" tone="subtle">Person</Text>
                {COUNT_COLUMNS.map(({ label }) => <Text as="span" align="end" key={label} size="xs" tone="subtle">{label}</Text>)}
            </UsageHeader>
            <Frame as="ul" aria-label="People" gap="2xl">
                {people.map((person) => <PersonRow key={person.did} person={person} />)}
            </Frame>
        </Frame>
    );
}

function personName(person: Person): string {
    return person.displayName?.trim() || (person.handle ? `@${person.handle}` : person.did);
}

function personMeta(person: Person, name: string): string {
    const handle = person.handle && name !== `@${person.handle}` ? `@${person.handle}` : null;
    const lastSeen = formatDate(person.lastActiveAt);
    return [handle, lastSeen ? `Last seen ${lastSeen}` : 'No active session'].filter(Boolean).join(' · ');
}

function countLabel(person: Person, key: keyof SourceCounts, label: string): string {
    return `${person[key]} ${label.toLowerCase()}`;
}

function PersonRow({ person }: { person: Person }) {
    const name = personName(person);
    return (
        <UsageRow>
            <PersonAvatar seed={person.did} lazy src={person.avatar} />
            <Frame minWidth={0}>
                <Text
                    family={person.handle ? undefined : 'duo'}
                    size={person.handle ? undefined : 'sm'}
                    title={person.did}
                    truncate
                    weight="medium"
                >
                    {name}
                </Text>
                <Text size="sm" tone="subtle" truncate>{personMeta(person, name)}</Text>
            </Frame>
            <Frame colStart={2} display={{ base: 'flex', sm: 'none' }}>
                <Text size="sm" tone="muted">
                    {COUNT_COLUMNS.map(({ key, label }) => countLabel(person, key, label)).join(' · ')}
                </Text>
            </Frame>
            {COUNT_COLUMNS.map(({ key, label }) => (
                <Text
                    align="end"
                    display={{ base: 'none', sm: 'block' }}
                    key={key}
                    numeric="tabular"
                    tone={person[key] === 0 ? 'subtle' : undefined}
                >
                    <Text as="span" aria-hidden="true">{person[key]}</Text>
                    <Text as="span" visuallyHidden>{countLabel(person, key, label)}</Text>
                </Text>
            ))}
        </UsageRow>
    );
}
