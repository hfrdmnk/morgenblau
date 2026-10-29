import type { ReactNode } from 'react';

import { ListError } from '@/components/item-list';
import { PersonAvatar } from '@/components/person-avatar';
import { useDocumentTitle } from '@/hooks/use-document-title';
import { useSources } from '@/hooks/use-sources';
import { formatDate } from '@/lib/source-display';
import { cn } from '@/lib/utils';

type SourceCounts = { regular: number; generated: number; newsletter: number };

type Person = SourceCounts & {
    did: string;
    handle: string;
    displayName: string | null;
    avatar: string | null;
    lastActiveAt: string | null;
};

type Usage = { totals: SourceCounts & { people: number }; people: Person[] };

const COUNT_COLUMNS = [
    { key: 'regular', label: 'Regular' },
    { key: 'generated', label: 'Generated' },
    { key: 'newsletter', label: 'Newsletter' },
] as const;

export function Admin() {
    const usage = useSources<Usage>('/api/admin/usage');
    useDocumentTitle('Usage');
    return (
        <main aria-label="Admin" className="mx-auto w-full max-w-3xl space-y-14 px-5 pb-20 pt-20 sm:px-8 md:pt-24" id="main">
            <h1 className="sr-only">Usage</h1>
            {usage.status === 'loaded' ? (
                <>
                    <Section title="Overview"><Totals totals={usage.data.totals} /></Section>
                    <Section title="People"><People people={usage.data.people} /></Section>
                </>
            ) : usage.status === 'error' ? (
                usage.notFound ? (
                    <div className="space-y-3" role="alert">
                        <h2 className="text-2xl font-medium">Not found</h2>
                        <p className="text-muted-foreground">This page is only available to admins.</p>
                    </div>
                ) : <ListError label="usage" retry={usage.retry} />
            ) : <AdminSkeleton />}
        </main>
    );
}

function Section({ title, children }: { title: string; children: ReactNode }) {
    return (
        <section aria-label={title}>
            <h2 className="text-base font-medium text-muted-foreground">{title}</h2>
            <div className="mt-7">{children}</div>
        </section>
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
        <dl className="grid grid-cols-2 gap-x-6 gap-y-8 sm:grid-cols-4">
            {stats.map(({ label, value }) => (
                <div key={label}>
                    <dt className="text-sm text-muted-foreground">{label}</dt>
                    <dd className="mt-1 text-3xl font-medium tabular-nums">{value}</dd>
                </div>
            ))}
        </dl>
    );
}

function People({ people }: { people: Person[] }) {
    if (!people.length) return <p className="text-sm text-muted-foreground">Nobody has signed in yet.</p>;
    return (
        <div>
            <div aria-hidden="true" className="hidden grid-cols-[1fr_repeat(3,5.5rem)] gap-4 pb-3 text-xs text-subtle-foreground sm:grid">
                <span>Person</span>
                {COUNT_COLUMNS.map(({ label }) => <span className="text-right" key={label}>{label}</span>)}
            </div>
            <ul aria-label="People" className="space-y-6">
                {people.map((person) => <PersonRow key={person.did} person={person} />)}
            </ul>
        </div>
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
        <li className="grid min-w-0 grid-cols-[2rem_1fr] items-center gap-x-3 gap-y-2 sm:grid-cols-[2rem_1fr_repeat(3,5.5rem)] sm:gap-x-4">
            <PersonAvatar did={person.did} lazy src={person.avatar} />
            <div className="min-w-0">
                <p className={cn('truncate font-medium', !person.handle && 'font-mono text-sm')} title={person.did}>{name}</p>
                <p className="truncate text-sm text-subtle-foreground">{personMeta(person, name)}</p>
            </div>
            <p className="col-start-2 text-sm text-muted-foreground sm:hidden">
                {COUNT_COLUMNS.map(({ key, label }) => countLabel(person, key, label)).join(' · ')}
            </p>
            {COUNT_COLUMNS.map(({ key, label }) => (
                <p className={cn('hidden text-right tabular-nums sm:block', person[key] === 0 && 'text-subtle-foreground')} key={key}>
                    <span aria-hidden="true">{person[key]}</span>
                    <span className="sr-only">{countLabel(person, key, label)}</span>
                </p>
            ))}
        </li>
    );
}

function AdminSkeleton() {
    return (
        <div aria-label="Loading usage" className="space-y-14" role="status">
            <div className="grid grid-cols-2 gap-x-6 gap-y-8 sm:grid-cols-4">
                {[0, 1, 2, 3].map((index) => (
                    <div key={index}>
                        <div className="h-5 w-24 animate-pulse rounded bg-muted motion-reduce:animate-none" />
                        <div className="mt-2 h-9 w-12 animate-pulse rounded bg-muted motion-reduce:animate-none" />
                    </div>
                ))}
            </div>
            <div className="space-y-6">
                {[64, 48, 56].map((width) => (
                    <div className="flex items-center gap-3" key={width}>
                        <div className="size-8 animate-pulse rounded-full bg-muted motion-reduce:animate-none" />
                        <div className="h-5 animate-pulse rounded bg-muted motion-reduce:animate-none" style={{ width: `${width}%` }} />
                    </div>
                ))}
            </div>
        </div>
    );
}
