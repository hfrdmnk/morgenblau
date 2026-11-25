import { useId, type ReactNode } from 'react';
import { toast } from 'sonner';

import { ArrowRightIcon, CheckIcon, ExternalLinkIcon, LoadingIcon } from '@/components/icons';
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
import { StateSymbol } from '@/components/ui/state-symbol';
import { SpecimenAside, SpecimenStrip, StateSpecimen, Swatch, ThemeScope } from '@/components/ui/specimen';
import { Textarea } from '@/components/ui/textarea';
import { Frame } from '@/design/frame';
import { Text } from '@/design/text';
import { frame, spacing, text, type Spacing } from '@/design/tokens';

const sections = [
    ['foundations', 'Foundations'],
    ['typography', 'Typography'],
    ['color', 'Color'],
    ['spacing', 'Spacing'],
    ['shape', 'Radius and shadow'],
    ['controls', 'Controls'],
    ['forms', 'Forms'],
    ['surfaces', 'Surfaces'],
    ['states', 'States'],
    ['motion', 'Motion'],
    ['primitives', 'Frame and Text'],
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
] as const;

type TextSize = keyof typeof text.size;

const typeSamples: Record<TextSize, string> = {
    '3xl': 'A calm daily newspaper',
    '2xl': 'Today’s digest',
    xl: 'Long-form reading',
    lg: 'A considered introduction',
    base: 'Comfortable body text for sustained reading.',
    sm: 'Supporting information and controls',
    xs: 'Metadata and quiet labels',
};

const typeScale = (Object.keys(text.size) as TextSize[]).reverse();

const weightValues: Record<keyof typeof text.weight, number> = { normal: 400, book: 450, medium: 500 };

const keys = <T extends object>(map: T) => Object.keys(map) as (keyof T)[];

type SectionProps = {
    id: string;
    title: string;
    description: string;
    children: ReactNode;
};

function Section({ children, description, id, title }: SectionProps) {
    return (
        <Frame as="section" id={id} gap="2xl">
            <Frame as="header" maxWidth="2xl" gap="s">
                <Text as="h2" size="2xl" weight="medium">
                    {title}
                </Text>
                <Text size="sm" tone="muted">
                    {description}
                </Text>
            </Frame>
            {children}
        </Frame>
    );
}

function Eyebrow({ children }: { children: ReactNode }) {
    return (
        <Text size="xs" weight="medium" tone="subtle" caps>
            {children}
        </Text>
    );
}

function ThemePanel({ children, theme }: { theme: 'light' | 'dark'; children: ReactNode }) {
    return (
        <ThemeScope theme={theme}>
            <Frame
                minHeight="full"
                radius="xl"
                background="background"
                shadow="hairline"
                padding={{ base: 'xl', sm: '2xl' }}
                gap="xl"
            >
                <Eyebrow>{theme === 'light' ? 'Light' : 'Dark'}</Eyebrow>
                {children}
            </Frame>
        </ThemeScope>
    );
}

function ThemePair({ children }: { children: ReactNode }) {
    return (
        <Frame display="grid" columns={{ lg: 2 }} gap="l">
            <ThemePanel theme="light">{children}</ThemePanel>
            <ThemePanel theme="dark">{children}</ThemePanel>
        </Frame>
    );
}

function Tinted({ children }: { children: ReactNode }) {
    return (
        <Frame radius="xl" background="muted" padding={{ base: 'xl', sm: '2xl' }} gap="l">
            {children}
        </Frame>
    );
}

type Token = string | { label: string; variable: string };

function TokenGrid({ tokens }: { tokens: readonly Token[] }) {
    return (
        <Frame display="grid" columns={{ base: 2, sm: 3 }} gapX="l" gapY="m">
            {tokens.map((token) => {
                const label = typeof token === 'string' ? token : token.label;
                return (
                    <Frame key={label} minWidth={0} gap="xs">
                        <Swatch variable={typeof token === 'string' ? token : token.variable} />
                        <Text as="code" size="xs" tone="muted" break="words">
                            {label}
                        </Text>
                    </Frame>
                );
            })}
        </Frame>
    );
}

function Field({ children, label }: { children: ReactNode; label: string }) {
    return (
        <Frame as="label" gap="s">
            <Text as="span" size="sm" weight="medium">
                {label}
            </Text>
            {children}
        </Frame>
    );
}

function Caption({ children }: { children: ReactNode }) {
    return (
        <Text size="xs" tone="subtle">
            {children}
        </Text>
    );
}

function TypeSpecimens() {
    return (
        <Frame gap="xl">
            {typeScale.map((size) => (
                <Frame key={size} display="grid" columns="specimen" align="baseline" gap="l">
                    <Text as="code" size="xs" tone="subtle">
                        {size}
                    </Text>
                    <Text size={size}>{typeSamples[size]}</Text>
                </Frame>
            ))}
            <Frame display="grid" columns={{ sm: 3 }} gap="s" border="top" paddingTop="xl">
                {keys(text.weight).map((weight) => (
                    <Text key={weight} weight={weight}>
                        {weight[0].toUpperCase() + weight.slice(1)} {weightValues[weight]}
                    </Text>
                ))}
            </Frame>
            <Frame direction="row" wrap gapX="xl" gapY="s" border="top" paddingTop="xl">
                {keys(text.tone).map((tone) => (
                    <Text key={tone} size="sm" tone={tone}>
                        {tone}
                    </Text>
                ))}
            </Frame>
        </Frame>
    );
}

function SpacingSpecimens() {
    return (
        <Frame gap="m">
            {keys(spacing).map((name: Spacing) => (
                <Frame key={name} display="grid" columns="specimen" align="center" gap="l">
                    <Text as="code" size="xs" tone="subtle">
                        {name}
                    </Text>
                    <Frame direction="row" align="center" gap="m">
                        <Frame background="primary" radius="sm" paddingStart={name} paddingY="xs" shrink={false} />
                        <Text as="code" size="xs" tone="muted">
                            {Number(spacing[name]) * 4}px
                        </Text>
                    </Frame>
                </Frame>
            ))}
        </Frame>
    );
}

function ShapeSpecimens() {
    return (
        <Frame gap="2xl">
            <Frame direction="row" wrap gap="2xl">
                {keys(frame.radius).map((radius) => (
                    <Frame key={radius} align="center" gap="s">
                        <Frame background="muted" radius={radius} padding="3xl" />
                        <Text as="code" size="xs" tone="muted">
                            {radius}
                        </Text>
                    </Frame>
                ))}
            </Frame>
            <Frame direction="row" wrap gap="2xl" border="top" paddingTop="xl">
                {keys(frame.shadow).map((shadow) => (
                    <Frame key={shadow} align="center" gap="s">
                        <Frame background="popover" radius="lg" padding="3xl" shadow={shadow} />
                        <Text as="code" size="xs" tone="muted">
                            {shadow}
                        </Text>
                    </Frame>
                ))}
            </Frame>
        </Frame>
    );
}

function ButtonSpecimens() {
    return (
        <Frame gap="2xl">
            <Frame direction="row" wrap align="center" gap="m">
                <Button>Primary</Button>
                <Button variant="secondary">Secondary</Button>
                <Button variant="ghost">Ghost</Button>
                <Button variant="destructive">Destructive</Button>
            </Frame>
            <Frame display="grid" columns={{ sm: 2 }} gap="l">
                {(['default', 'secondary'] as const).map((variant) => (
                    <Frame key={variant} gap="m">
                        <Caption>{variant === 'default' ? 'Primary' : 'Secondary'} states</Caption>
                        <Frame direction="row" wrap gap="m">
                            <Button size="sm" variant={variant}>
                                Rest
                            </Button>
                            <StateSpecimen state="hover">
                                <Button size="sm" variant={variant}>
                                    Hover
                                </Button>
                            </StateSpecimen>
                            <StateSpecimen state="focus">
                                <Button size="sm" variant={variant}>
                                    Focus
                                </Button>
                            </StateSpecimen>
                            <Button size="sm" variant={variant} disabled>
                                Disabled
                            </Button>
                        </Frame>
                    </Frame>
                ))}
            </Frame>
        </Frame>
    );
}

function FormSpecimens() {
    const errorId = useId();
    const successId = useId();

    return (
        <Frame display="grid" columns={{ sm: 2 }} gap="xl">
            <Field label="Default">
                <Input placeholder="Search your sources" />
            </Field>
            <Field label="Focus demonstration">
                <StateSpecimen state="field-focus">
                    <Input defaultValue="Selected field" />
                </StateSpecimen>
            </Field>
            <Field label="Disabled">
                <Input defaultValue="Unavailable" disabled />
            </Field>
            <Field label="Error">
                <Frame as="span" gap="xs">
                    <Input defaultValue="Incomplete value" aria-describedby={errorId} aria-invalid />
                    <Text as="span" id={errorId} size="xs" tone="destructive">
                        Enter a complete source address.
                    </Text>
                </Frame>
            </Field>
            <Field label="Success">
                <Frame as="span" gap="xs">
                    <StateSpecimen state="field-success">
                        <Input defaultValue="https://feed.example.com" aria-describedby={successId} />
                    </StateSpecimen>
                    <Frame as="span" id={successId} direction="row" align="center" gap="2xs">
                        <CheckIcon size="xs" tone="success" />
                        <Text as="span" size="xs" tone="success">
                            Source found
                        </Text>
                    </Frame>
                </Frame>
            </Field>
            <Field label="Note">
                <Textarea placeholder="Add a short note" />
            </Field>
        </Frame>
    );
}

function SurfaceSpecimens() {
    return (
        <Frame display="grid" columns={{ sm: 2 }} gap="xl">
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
                    <Text size="sm">A finite collection with room for the content to lead.</Text>
                </CardContent>
                <CardFooter>
                    <Link href="#states">
                        View states <ArrowRightIcon size="md" />
                    </Link>
                </CardFooter>
            </Card>
            <Frame direction="row" align="start" justify="center" radius="xl" background="muted" padding="3xl">
                <Frame
                    width="full"
                    maxWidth="popover"
                    radius="lg"
                    background="popover"
                    padding="l"
                    shadow="popover"
                    gap="2xs"
                >
                    <Text weight="medium">Popover plane</Text>
                    <Text size="sm" tone="muted">
                        A distinct elevated surface reserved for temporary context.
                    </Text>
                </Frame>
            </Frame>
        </Frame>
    );
}

function PrimitiveReference({
    name,
    description,
    maps,
}: {
    name: string;
    description: string;
    maps: Record<string, Record<string | number, string>>;
}) {
    return (
        <Frame gap="l">
            <Frame gap="2xs">
                <Text as="h3" weight="medium">
                    {name}
                </Text>
                <Text size="sm" tone="muted">
                    {description}
                </Text>
            </Frame>
            <Frame as="ul" gap="s">
                {Object.entries(maps).map(([prop, map]) => (
                    <Frame as="li" key={prop} display="grid" columns={{ sm: 'sidebar' }} gapX="l" gapY="3xs">
                        <Text as="code" size="xs">
                            {prop}
                        </Text>
                        <Text as="code" size="xs" tone="muted" break="words">
                            {Object.keys(map).join(' · ')}
                        </Text>
                    </Frame>
                ))}
            </Frame>
        </Frame>
    );
}

export function Styleguide() {
    return (
        <Frame as="main" minHeight="screen" background="background" align="center">
            <Frame
                display="grid"
                columns={{ base: 1, lg: 'sidebar' }}
                width="full"
                maxWidth="7xl"
                gap={{ base: '5xl', lg: '6xl' }}
                paddingX={{ base: 'xl', sm: '3xl' }}
                paddingY={{ base: '4xl', lg: '6xl' }}
            >
                <SpecimenAside>
                    <Eyebrow>Development only</Eyebrow>
                    <Frame
                        as="nav"
                        direction={{ base: 'row', lg: 'column' }}
                        gapX="l"
                        gapY="s"
                        overflow="auto"
                        paddingBottom="s"
                    >
                        {sections.map(([id, label]) => (
                            <Link key={id} href={`#${id}`} variant="muted">
                                <Text as="span" size="sm" wrap="nowrap">
                                    {label}
                                </Text>
                            </Link>
                        ))}
                    </Frame>
                </SpecimenAside>

                <Frame minWidth={0} gap="7xl">
                    <Frame as="header" id="foundations" gap="l">
                        <Badge>Phase two</Badge>
                        <Frame maxWidth="3xl" gap="m">
                            <Text as="h1" size="3xl" weight="medium">
                                Morgenblau style guide
                            </Text>
                            <Frame maxWidth="2xl">
                                <Text size="lg" tone="muted">
                                    A working index of the design tokens and the primitives built on them. Product pages
                                    compose Frame, Text and these components; lint rejects raw elements and class
                                    strings.
                                </Text>
                            </Frame>
                        </Frame>
                        <Frame direction="row" wrap gap="m" paddingTop="s">
                            <Button>
                                Start reading <ArrowRightIcon />
                            </Button>
                            <Button variant="secondary">Manage sources</Button>
                        </Frame>
                    </Frame>

                    <Section
                        id="typography"
                        title="Typography"
                        description="Geist uses a major-second size scale. Size, weight and tone remain independent choices."
                    >
                        <ThemePair>
                            <TypeSpecimens />
                        </ThemePair>
                    </Section>

                    <Section
                        id="color"
                        title="Semantic color"
                        description="Stone neutrals carry the interface. Product colors identify Digest, Sources, and Library rather than ordinary actions."
                    >
                        <ThemePair>
                            <Frame gap="2xl">
                                <Frame gap="m">
                                    <Text size="sm" weight="medium">
                                        Core
                                    </Text>
                                    <TokenGrid tokens={semanticTokens} />
                                </Frame>
                                <Frame gap="m" border="top" paddingTop="2xl">
                                    <Text size="sm" weight="medium">
                                        Charts and shell
                                    </Text>
                                    <TokenGrid tokens={supportTokens} />
                                </Frame>
                            </Frame>
                        </ThemePair>
                        <Tinted>
                            <Text size="sm" weight="medium">
                                Product modes
                            </Text>
                            <TokenGrid tokens={productColors} />
                        </Tinted>
                    </Section>

                    <Section
                        id="spacing"
                        title="Spacing"
                        description="Gap and padding take a step on this scale. There are no margins: siblings are spaced by their parent's gap."
                    >
                        <Tinted>
                            <SpacingSpecimens />
                        </Tinted>
                    </Section>

                    <Section
                        id="shape"
                        title="Radius and shadow"
                        description="Corners soften surfaces; the popover shadow is reserved for temporary planes, and the hairline outlines a surface without raising it."
                    >
                        <ThemePair>
                            <ShapeSpecimens />
                        </ThemePair>
                    </Section>

                    <Section
                        id="controls"
                        title="Buttons, icon buttons, links, and badges"
                        description="Primary controls invert the canvas. Secondary controls use a soft neutral fill. Focus rings remain neutral."
                    >
                        <ThemePair>
                            <ButtonSpecimens />
                        </ThemePair>
                        <Frame
                            display="grid"
                            columns={{ sm: 3 }}
                            gap="2xl"
                            radius="xl"
                            background="muted"
                            padding={{ base: 'xl', sm: '2xl' }}
                        >
                            <Frame gap="m">
                                <Caption>Icon buttons</Caption>
                                <Frame direction="row" gap="m">
                                    <Button size="icon" aria-label="Continue">
                                        <ArrowRightIcon />
                                    </Button>
                                    <Button size="icon" variant="secondary" aria-label="Open source">
                                        <ExternalLinkIcon />
                                    </Button>
                                    <Button size="icon" variant="ghost" aria-label="Loading" disabled>
                                        <LoadingIcon spin />
                                    </Button>
                                </Frame>
                            </Frame>
                            <Frame gap="m">
                                <Caption>Links</Caption>
                                <Text as="span" display="block" size="sm">
                                    <Frame as="span" align="start" gap="s">
                                        <Link href="#forms">Internal link</Link>
                                        <Link href="https://example.com">
                                            External link <ExternalLinkIcon size="md" />
                                        </Link>
                                    </Frame>
                                </Text>
                            </Frame>
                            <Frame gap="m">
                                <Caption>Badges</Caption>
                                <Frame direction="row" wrap gap="s">
                                    <Badge>Neutral</Badge>
                                    <Badge variant="success">
                                        <CheckIcon /> Healthy
                                    </Badge>
                                    <Badge variant="destructive">Needs attention</Badge>
                                </Frame>
                            </Frame>
                        </Frame>
                    </Section>

                    <Section
                        id="forms"
                        title="Inputs and textareas"
                        description="Fields are borderless tonal areas. Focus and validation use a one-pixel border treatment."
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
                        <Frame
                            direction="row"
                            align="center"
                            gap="l"
                            radius="xl"
                            background="muted"
                            padding={{ base: 'xl', sm: '2xl' }}
                        >
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
                            <Text size="sm" tone="muted">
                                Uses Base UI positioning and focus management.
                            </Text>
                        </Frame>
                        <Frame direction="row" wrap gap="m">
                            <Button
                                variant="secondary"
                                onClick={() =>
                                    toast.success('Import complete', {
                                        description: '16 added, 0 updated, 0 already up to date.',
                                    })
                                }
                            >
                                Show success toast
                            </Button>
                            <Button
                                variant="secondary"
                                onClick={() =>
                                    toast.error('Could not import sources', { description: 'Please try again.' })
                                }
                            >
                                Show error toast
                            </Button>
                            <Button variant="secondary" onClick={() => toast.info('No next article in this digest')}>
                                Show info toast
                            </Button>
                        </Frame>
                    </Section>

                    <Section
                        id="states"
                        title="Loading, empty, and error"
                        description="Page states use concise copy, one clear recovery action where useful, and no decorative containers. Empty and error states carry their mode’s symbol, painted with its art."
                    >
                        <SpecimenStrip>
                            <LoadingState
                                title="Gathering today’s entries"
                                description="This should only take a moment."
                            />
                            <EmptyState
                                symbol="calm"
                                title="No entries for this day"
                                description="Choose another day or add a source."
                                action={<Button variant="secondary">Add a source</Button>}
                            />
                            <ErrorState
                                title="Couldn’t load this digest"
                                description="Your saved sources are unchanged."
                                action={<Button variant="secondary">Try again</Button>}
                            />
                        </SpecimenStrip>
                        <Frame direction="row" wrap align="end" gap="3xl">
                            {(['calm', 'streams', 'shelf', 'loop'] as const).map((shape) => (
                                <StateSymbol key={shape} shape={shape} />
                            ))}
                        </Frame>
                    </Section>

                    <Section
                        id="motion"
                        title="Motion and reduced motion"
                        description="Motion is reserved for feedback and temporary context. Reduced-motion preferences remove the loading rotation and collapse interface durations."
                    >
                        <Frame display="grid" columns={{ sm: 2 }} gap="l">
                            {[
                                {
                                    label: 'Default',
                                    spin: true,
                                    sample: 'Loading feedback',
                                    note: '120ms · 160ms · 240ms',
                                },
                                {
                                    label: 'Reduced motion',
                                    spin: false,
                                    sample: 'Static status indicator',
                                    note: '1ms interface durations · no rotation',
                                },
                            ].map(({ label, spin, sample, note }) => (
                                <Frame key={label} radius="xl" background="muted" padding="2xl" gap="xl">
                                    <Caption>{label}</Caption>
                                    <Frame direction="row" align="center" gap="m">
                                        <LoadingIcon size="2xl" spin={spin} />
                                        <Text as="span" size="sm">
                                            {sample}
                                        </Text>
                                    </Frame>
                                    <Text as="code" size="xs" tone="muted">
                                        {note}
                                    </Text>
                                </Frame>
                            ))}
                        </Frame>
                    </Section>

                    <Section
                        id="primitives"
                        title="Frame and Text"
                        description="Every container is a Frame and every run of text is a Text. Each prop takes one of these tokens, or one per breakpoint as { base, sm, md, lg }."
                    >
                        <Tinted>
                            <Frame display="grid" columns={{ lg: 2 }} gap="3xl">
                                <PrimitiveReference
                                    name="Frame"
                                    description="A flex column unless display or direction says otherwise."
                                    maps={frame}
                                />
                                <PrimitiveReference
                                    name="Text"
                                    description="A paragraph by default; anything left out inherits."
                                    maps={text}
                                />
                            </Frame>
                        </Tinted>
                    </Section>
                </Frame>
            </Frame>
        </Frame>
    );
}
