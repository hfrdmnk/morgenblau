import { useId, type ReactNode } from 'react';

import {
    ArrowRightIcon,
    CheckIcon,
    ExternalLinkIcon,
    LoadingIcon,
} from '@/components/icons';
import { EmptyState, ErrorState, LoadingState } from '@/components/status-state';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import {
    Card,
    CardAction,
    CardContent,
    CardDescription,
    CardFooter,
    CardHeader,
    CardTitle,
} from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { Link } from '@/components/ui/link';
import {
    Popover,
    PopoverContent,
    PopoverDescription,
    PopoverHeader,
    PopoverTitle,
    PopoverTrigger,
} from '@/components/ui/popover';
import { Textarea } from '@/components/ui/textarea';

const sections = [
    ['foundations', 'Foundations'],
    ['typography', 'Typography'],
    ['color', 'Color'],
    ['controls', 'Controls'],
    ['forms', 'Forms'],
    ['surfaces', 'Surfaces'],
    ['states', 'States'],
    ['motion', 'Motion'],
] as const;

const semanticTokens = [
    'background',
    'foreground',
    'card',
    'card-foreground',
    'popover',
    'popover-foreground',
    'primary',
    'primary-foreground',
    'secondary',
    'secondary-foreground',
    'muted',
    'muted-foreground',
    'subtle-foreground',
    'accent',
    'accent-foreground',
    'destructive',
    'destructive-foreground',
    'success',
    'success-foreground',
    'border',
    'input',
    'ring',
] as const;

const supportTokens = [
    'chart-1',
    'chart-2',
    'chart-3',
    'chart-4',
    'chart-5',
    'sidebar',
    'sidebar-foreground',
    'sidebar-primary',
    'sidebar-primary-foreground',
    'sidebar-accent',
    'sidebar-accent-foreground',
    'sidebar-border',
    'sidebar-ring',
] as const;

const productColors = [
    { label: 'atmosphere-blue', variable: 'color-atmosphere-blue' },
    { label: 'atmosphere-blue-off', variable: 'atmosphere-blue-off' },
    { label: 'sunrise-orange', variable: 'color-sunrise-orange' },
    { label: 'sunrise-orange-off', variable: 'sunrise-orange-off' },
    { label: 'aurora-violet', variable: 'color-aurora-violet' },
    { label: 'aurora-violet-off', variable: 'aurora-violet-off' },
] as const;

type SectionProps = {
    id: string;
    title: string;
    description: string;
    children: ReactNode;
};

function Section({ children, description, id, title }: SectionProps) {
    return (
        <section id={id} className="scroll-mt-8 space-y-6">
            <header className="max-w-2xl space-y-2">
                <h2 className="text-2xl font-medium">{title}</h2>
                <p className="text-sm text-muted-foreground">{description}</p>
            </header>
            {children}
        </section>
    );
}

type ThemePanelProps = {
    label: string;
    dark?: boolean;
    children: ReactNode;
};

function ThemePanel({ children, dark = false, label }: ThemePanelProps) {
    return (
        <div className={dark ? 'dark' : ''}>
            <div className="min-h-full rounded-xl bg-background p-5 text-foreground ring-1 ring-border sm:p-6">
                <p className="mb-5 text-xs font-medium tracking-wide text-subtle-foreground uppercase">
                    {label}
                </p>
                {children}
            </div>
        </div>
    );
}

function ThemePair({ children }: { children: ReactNode }) {
    return (
        <div className="grid gap-4 lg:grid-cols-2">
            <ThemePanel label="Light">{children}</ThemePanel>
            <ThemePanel label="Dark" dark>
                {children}
            </ThemePanel>
        </div>
    );
}

type Token = string | { label: string; variable: string };

function TokenGrid({ tokens }: { tokens: readonly Token[] }) {
    return (
        <div className="grid grid-cols-2 gap-x-4 gap-y-3 sm:grid-cols-3">
            {tokens.map((token) => (
                <div
                    key={typeof token === 'string' ? token : token.label}
                    className="min-w-0"
                >
                    <div
                        className="mb-1.5 h-10 rounded-md ring-1 ring-foreground/10"
                        style={{
                            backgroundColor: `var(--${typeof token === 'string' ? token : token.variable})`,
                        }}
                    />
                    <code className="block break-words text-xs leading-tight text-muted-foreground">
                        {typeof token === 'string' ? token : token.label}
                    </code>
                </div>
            ))}
        </div>
    );
}

function Field({
    children,
    label,
}: {
    children: ReactNode;
    label: string;
}) {
    return (
        <label className="grid gap-2 text-sm font-medium">
            {label}
            {children}
        </label>
    );
}

function ButtonSpecimens() {
    return (
        <div className="space-y-6">
            <div className="flex flex-wrap items-center gap-3">
                <Button>Primary</Button>
                <Button variant="secondary">Secondary</Button>
                <Button variant="ghost">Ghost</Button>
                <Button variant="destructive">Destructive</Button>
            </div>
            <div className="grid gap-4 sm:grid-cols-2">
                <div className="space-y-3">
                    <p className="text-xs text-subtle-foreground">Primary states</p>
                    <div className="flex flex-wrap gap-3">
                        <Button size="sm">Rest</Button>
                        <Button size="sm" className="bg-primary/90">
                            Hover
                        </Button>
                        <Button
                            size="sm"
                            className="outline-2 outline-offset-2 outline-ring"
                        >
                            Focus
                        </Button>
                        <Button size="sm" disabled>
                            Disabled
                        </Button>
                    </div>
                </div>
                <div className="space-y-3">
                    <p className="text-xs text-subtle-foreground">Secondary states</p>
                    <div className="flex flex-wrap gap-3">
                        <Button size="sm" variant="secondary">
                            Rest
                        </Button>
                        <Button size="sm" variant="secondary" className="bg-accent">
                            Hover
                        </Button>
                        <Button
                            size="sm"
                            variant="secondary"
                            className="outline-2 outline-offset-2 outline-ring"
                        >
                            Focus
                        </Button>
                        <Button size="sm" variant="secondary" disabled>
                            Disabled
                        </Button>
                    </div>
                </div>
            </div>
        </div>
    );
}

function FormSpecimens() {
    const errorId = useId();
    const successId = useId();

    return (
        <div className="grid gap-5 sm:grid-cols-2">
            <Field label="Default">
                <Input placeholder="Search your sources" />
            </Field>
            <Field label="Focus demonstration">
                <Input
                    defaultValue="Selected field"
                    className="outline-2 outline-offset-2 outline-ring"
                />
            </Field>
            <Field label="Disabled">
                <Input defaultValue="Unavailable" disabled />
            </Field>
            <Field label="Error">
                <span className="grid gap-1.5">
                    <Input
                        defaultValue="Incomplete value"
                        aria-describedby={errorId}
                        aria-invalid
                    />
                    <span id={errorId} className="text-xs text-destructive">
                        Enter a complete source address.
                    </span>
                </span>
            </Field>
            <Field label="Success">
                <span className="grid gap-1.5">
                    <Input
                        defaultValue="https://feed.example.com"
                        className="shadow-[inset_0_0_0_1px_var(--success)]"
                        aria-describedby={successId}
                    />
                    <span
                        id={successId}
                        className="inline-flex items-center gap-1 text-xs text-success"
                    >
                        <CheckIcon className="size-3" /> Source found
                    </span>
                </span>
            </Field>
            <Field label="Note">
                <Textarea placeholder="Add a short note" />
            </Field>
        </div>
    );
}

function SurfaceSpecimens() {
    return (
        <div className="grid gap-5 sm:grid-cols-2">
            <Card>
                <CardHeader>
                    <CardTitle>Canvas card</CardTitle>
                    <CardDescription>
                        Cards use spacing for hierarchy and remain on the continuous canvas.
                    </CardDescription>
                    <CardAction>
                        <Badge>12 items</Badge>
                    </CardAction>
                </CardHeader>
                <CardContent>
                    <p className="text-sm">
                        A finite collection with room for the content to lead.
                    </p>
                </CardContent>
                <CardFooter>
                    <Link href="#states">
                        View states <ArrowRightIcon className="size-4" />
                    </Link>
                </CardFooter>
            </Card>
            <div className="flex items-start justify-center rounded-xl bg-muted p-8">
                <div className="w-full max-w-72 rounded-lg bg-popover p-4 text-popover-foreground shadow-popover">
                    <p className="font-medium">Popover plane</p>
                    <p className="mt-1 text-sm text-muted-foreground">
                        A distinct elevated surface reserved for temporary context.
                    </p>
                </div>
            </div>
        </div>
    );
}

export function Styleguide() {
    return (
        <main className="min-h-dvh bg-background text-foreground">
            <div className="mx-auto grid max-w-7xl grid-cols-[minmax(0,1fr)] gap-12 px-5 py-10 sm:px-8 lg:grid-cols-[11rem_minmax(0,1fr)] lg:gap-16 lg:py-16">
                <aside className="min-w-0 lg:sticky lg:top-8 lg:h-fit">
                    <p className="text-xs font-medium tracking-wide text-subtle-foreground uppercase">
                        Development only
                    </p>
                    <nav className="mt-5 flex gap-x-4 gap-y-2 overflow-x-auto pb-2 text-sm lg:flex-col lg:overflow-visible">
                        {sections.map(([id, label]) => (
                            <a
                                key={id}
                                href={`#${id}`}
                                className="shrink-0 text-muted-foreground hover:text-foreground"
                            >
                                {label}
                            </a>
                        ))}
                    </nav>
                </aside>

                <div className="min-w-0 space-y-20">
                    <header id="foundations" className="scroll-mt-8 space-y-4">
                        <Badge>Phase two</Badge>
                        <div className="max-w-3xl space-y-3">
                            <h1 className="text-3xl font-medium">Morgenblau style guide</h1>
                            <p className="max-w-2xl text-lg text-muted-foreground">
                                A working index of the visual foundations and the first reusable
                                primitives. Product pages should compose these directly.
                            </p>
                        </div>
                        <div className="flex flex-wrap gap-3 pt-2">
                            <Button>
                                Start reading <ArrowRightIcon />
                            </Button>
                            <Button variant="secondary">Manage sources</Button>
                        </div>
                    </header>

                    <Section
                        id="typography"
                        title="Typography"
                        description="Geist uses a major-second size scale. Size and weight remain independent choices."
                    >
                        <ThemePair>
                            <div className="space-y-5">
                                {[
                                    ['text-3xl', '3xl', 'A calm daily newspaper'],
                                    ['text-2xl', '2xl', 'Today’s digest'],
                                    ['text-xl', 'xl', 'Long-form reading'],
                                    ['text-lg', 'lg', 'A considered introduction'],
                                    ['text-base', 'base', 'Comfortable body text for sustained reading.'],
                                    ['text-sm', 'sm', 'Supporting information and controls'],
                                    ['text-xs', 'xs', 'Metadata and quiet labels'],
                                ].map(([className, name, sample]) => (
                                    <div
                                        key={name}
                                        className="grid grid-cols-[3rem_minmax(0,1fr)] items-baseline gap-4"
                                    >
                                        <code className="text-xs text-subtle-foreground">{name}</code>
                                        <p className={className}>{sample}</p>
                                    </div>
                                ))}
                                <div className="grid gap-2 border-t border-border pt-5 sm:grid-cols-2">
                                    <p className="font-normal">Normal 400</p>
                                    <p className="font-medium">Medium 500</p>
                                </div>
                            </div>
                        </ThemePair>
                    </Section>

                    <Section
                        id="color"
                        title="Semantic color"
                        description="Stone neutrals carry the interface. Product colors identify Digest, Sources, and Library rather than ordinary actions."
                    >
                        <ThemePair>
                            <div className="space-y-6">
                                <div className="space-y-3">
                                    <p className="text-sm font-medium">Core</p>
                                    <TokenGrid tokens={semanticTokens} />
                                </div>
                                <div className="space-y-3 border-t border-border pt-6">
                                    <p className="text-sm font-medium">Charts and shell</p>
                                    <TokenGrid tokens={supportTokens} />
                                </div>
                            </div>
                        </ThemePair>
                        <div className="rounded-xl bg-muted p-5 sm:p-6">
                            <p className="mb-4 text-sm font-medium">Product modes</p>
                            <TokenGrid tokens={productColors} />
                        </div>
                    </Section>

                    <Section
                        id="controls"
                        title="Buttons, icon buttons, links, and badges"
                        description="Primary controls invert the canvas. Secondary controls use a soft neutral fill. All focus rings remain neutral and external."
                    >
                        <ThemePair>
                            <ButtonSpecimens />
                        </ThemePair>
                        <div className="grid gap-6 rounded-xl bg-muted p-5 sm:grid-cols-3 sm:p-6">
                            <div className="space-y-3">
                                <p className="text-xs text-subtle-foreground">Icon buttons</p>
                                <div className="flex gap-3">
                                    <Button size="icon" aria-label="Continue">
                                        <ArrowRightIcon />
                                    </Button>
                                    <Button size="icon" variant="secondary" aria-label="Open source">
                                        <ExternalLinkIcon />
                                    </Button>
                                    <Button size="icon" variant="ghost" aria-label="Loading" disabled>
                                        <LoadingIcon className="motion-safe:animate-spin" />
                                    </Button>
                                </div>
                            </div>
                            <div className="space-y-3">
                                <p className="text-xs text-subtle-foreground">Links</p>
                                <div className="flex flex-col items-start gap-2 text-sm">
                                    <Link href="#forms">Internal link</Link>
                                    <Link href="https://example.com">
                                        External link <ExternalLinkIcon className="size-4" />
                                    </Link>
                                </div>
                            </div>
                            <div className="space-y-3">
                                <p className="text-xs text-subtle-foreground">Badges</p>
                                <div className="flex flex-wrap gap-2">
                                    <Badge>Neutral</Badge>
                                    <Badge variant="success">
                                        <CheckIcon /> Healthy
                                    </Badge>
                                    <Badge variant="destructive">Needs attention</Badge>
                                </div>
                            </div>
                        </div>
                    </Section>

                    <Section
                        id="forms"
                        title="Inputs and textareas"
                        description="Fields are borderless tonal areas. Validation uses semantic color without replacing the external focus ring."
                    >
                        <ThemePair>
                            <FormSpecimens />
                        </ThemePair>
                    </Section>

                    <Section
                        id="surfaces"
                        title="Cards and popovers"
                        description="Cards remain part of the canvas. Popovers use their own surface and shadow because they temporarily sit above it."
                    >
                        <ThemePair>
                            <SurfaceSpecimens />
                        </ThemePair>
                        <div className="flex items-center gap-4 rounded-xl bg-muted p-5 sm:p-6">
                            <Popover>
                                <PopoverTrigger render={<Button variant="secondary" />}>
                                    Open interactive popover
                                </PopoverTrigger>
                                <PopoverContent align="start">
                                    <PopoverHeader>
                                        <PopoverTitle>Digest settings</PopoverTitle>
                                        <PopoverDescription>
                                            Temporary context stays close to its trigger.
                                        </PopoverDescription>
                                    </PopoverHeader>
                                    <Button size="sm">Done</Button>
                                </PopoverContent>
                            </Popover>
                            <p className="text-sm text-muted-foreground">
                                Uses Base UI positioning and focus management.
                            </p>
                        </div>
                    </Section>

                    <Section
                        id="states"
                        title="Loading, empty, and error"
                        description="Page states use concise copy, one clear recovery action where useful, and no decorative containers."
                    >
                        <div className="grid divide-y divide-border rounded-xl bg-muted lg:grid-cols-3 lg:divide-x lg:divide-y-0">
                            <LoadingState
                                title="Gathering today’s entries"
                                description="This should only take a moment."
                            />
                            <EmptyState
                                title="No entries for this day"
                                description="Choose another day or add a source."
                                action={<Button variant="secondary">Add a source</Button>}
                            />
                            <ErrorState
                                title="Couldn’t load this digest"
                                description="Your saved sources are unchanged."
                                action={<Button variant="secondary">Try again</Button>}
                            />
                        </div>
                    </Section>

                    <Section
                        id="motion"
                        title="Motion and reduced motion"
                        description="Motion is reserved for feedback and temporary context. Reduced-motion preferences remove the loading rotation and collapse interface durations."
                    >
                        <div className="grid gap-4 sm:grid-cols-2">
                            <div className="rounded-xl bg-muted p-6">
                                <p className="text-xs text-subtle-foreground">Default</p>
                                <div className="mt-5 flex items-center gap-3">
                                    <LoadingIcon className="size-6 motion-safe:animate-spin" />
                                    <span className="text-sm">Loading feedback</span>
                                </div>
                                <code className="mt-5 block text-xs text-muted-foreground">
                                    120ms · 160ms · 240ms
                                </code>
                            </div>
                            <div className="rounded-xl bg-muted p-6">
                                <p className="text-xs text-subtle-foreground">Reduced motion</p>
                                <div className="mt-5 flex items-center gap-3">
                                    <LoadingIcon className="size-6" />
                                    <span className="text-sm">Static status indicator</span>
                                </div>
                                <code className="mt-5 block text-xs text-muted-foreground">
                                    1ms interface durations · no rotation
                                </code>
                            </div>
                        </div>
                    </Section>
                </div>
            </div>
        </main>
    );
}
