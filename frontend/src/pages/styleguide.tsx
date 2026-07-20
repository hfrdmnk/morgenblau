import type { ComponentProps, ReactNode } from 'react';

import { Button } from '@/components/ui/button';
import type { ButtonVariant } from '@/components/ui/button-variants';
import { Input } from '@/components/ui/input';
import {
    InputGroup,
    InputGroupAddon,
    InputGroupInput,
} from '@/components/ui/input-group';
import { Label } from '@/components/ui/label';
import { Switch } from '@/components/ui/switch';
import { Textarea } from '@/components/ui/textarea';
import { useDocumentTitle } from '@/hooks/use-document-title';
import { cn } from '@/lib/utils';

const BUTTON_VARIANTS = [
    { label: 'Primary', variant: 'default' },
    { label: 'Secondary', variant: 'secondary' },
    { label: 'Ghost', variant: 'ghost' },
    { label: 'Destructive', variant: 'destructive' },
    { label: 'Link', variant: 'link' },
] satisfies { label: string; variant: ButtonVariant }[];

const CHECKABLE_CLASS =
    'peer size-5 shrink-0 cursor-pointer appearance-none bg-overlay-1 transition-colors outline-none focus-visible:outline-solid focus-visible:outline-1 focus-visible:outline-offset-2 focus-visible:outline-ring disabled:cursor-not-allowed disabled:opacity-50';

type SectionProps = {
    title: string;
    description: string;
    children: ReactNode;
};

function StyleguideSection({ title, description, children }: SectionProps) {
    return (
        <section className="overflow-hidden rounded-xl bg-card shadow-card">
            <header className="space-y-1 px-6 py-5">
                <h2>{title}</h2>
                <p className="max-w-2xl text-sm font-light text-muted-foreground">
                    {description}
                </p>
            </header>
            <div aria-hidden className="border-t border-border" />
            <div className="p-6">{children}</div>
        </section>
    );
}

type FieldProps = {
    id: string;
    label: string;
    hint?: string;
    children: ReactNode;
};

function Field({ id, label, hint, children }: FieldProps) {
    return (
        <div className="space-y-2">
            <div className="space-y-1">
                <Label htmlFor={id}>{label}</Label>
                {hint ? (
                    <p
                        id={`${id}-hint`}
                        className="text-xs font-light text-muted-foreground"
                    >
                        {hint}
                    </p>
                ) : null}
            </div>
            {children}
        </div>
    );
}

type ChoiceProps = Omit<ComponentProps<'input'>, 'id' | 'type'> & {
    id: string;
    label: string;
};

function Checkbox({ id, label, disabled, ...props }: ChoiceProps) {
    return (
        <label
            htmlFor={id}
            className={cn(
                'inline-flex items-center gap-2.5 text-sm',
                disabled
                    ? 'cursor-not-allowed text-muted-foreground'
                    : 'cursor-pointer',
            )}
        >
            <span className="relative inline-flex">
                <input
                    id={id}
                    type="checkbox"
                    disabled={disabled}
                    className={cn(CHECKABLE_CLASS, 'rounded-sm checked:bg-primary')}
                    {...props}
                />
                <span className="pointer-events-none absolute top-[0.3rem] left-[0.45rem] h-2 w-1 rotate-45 border-r-2 border-b-2 border-primary-foreground opacity-0 peer-checked:opacity-100" />
            </span>
            {label}
        </label>
    );
}

function Radio({ id, label, disabled, ...props }: ChoiceProps) {
    return (
        <label
            htmlFor={id}
            className={cn(
                'inline-flex items-center gap-2.5 text-sm',
                disabled
                    ? 'cursor-not-allowed text-muted-foreground'
                    : 'cursor-pointer',
            )}
        >
            <span className="relative inline-flex">
                <input
                    id={id}
                    type="radio"
                    disabled={disabled}
                    className={cn(CHECKABLE_CLASS, 'rounded-full checked:bg-primary')}
                    {...props}
                />
                <span className="pointer-events-none absolute inset-0 m-auto size-1.5 rounded-full bg-primary-foreground opacity-0 peer-checked:opacity-100" />
            </span>
            {label}
        </label>
    );
}

type NativeSelectProps = ComponentProps<'select'>;

function NativeSelect({ className, children, ...props }: NativeSelectProps) {
    return (
        <select
            className={cn(
                'h-10 w-full cursor-pointer rounded-xl bg-overlay-1 px-2.5 text-base transition-colors outline-none focus-visible:bg-overlay-2 disabled:cursor-not-allowed disabled:opacity-50 aria-invalid:ring-1 aria-invalid:ring-destructive md:text-sm',
                className,
            )}
            {...props}
        >
            {children}
        </select>
    );
}

export function Styleguide() {
    useDocumentTitle('Styleguide');

    return (
        <main className="min-h-svh bg-background">
            <div className="mx-auto max-w-6xl space-y-8 px-6 py-10 sm:py-14">
                <header className="max-w-2xl space-y-2">
                    <p className="text-xs font-light tracking-wide text-muted-foreground uppercase">
                        Development only
                    </p>
                    <h1>Styleguide</h1>
                    <p className="text-sm font-light text-muted-foreground">
                        Morgenblau’s form controls and button treatments in
                        their interactive states.
                    </p>
                </header>

                <StyleguideSection
                    title="Text fields"
                    description="Borderless overlay fields deepen their fill on focus. Invalid fields earn a visible destructive edge."
                >
                    <div className="grid gap-x-6 gap-y-8 md:grid-cols-2">
                        <Field id="styleguide-text" label="Text">
                            <Input
                                id="styleguide-text"
                                type="text"
                                placeholder="Edition title"
                            />
                        </Field>

                        <Field
                            id="styleguide-email"
                            label="Email"
                            hint="Hints sit above the field so browser popovers cannot cover them."
                        >
                            <Input
                                id="styleguide-email"
                                type="email"
                                defaultValue="reader@example.test"
                                autoComplete="email"
                                aria-describedby="styleguide-email-hint"
                            />
                        </Field>

                        <Field id="styleguide-password" label="Password">
                            <Input
                                id="styleguide-password"
                                type="password"
                                defaultValue="morgenblau"
                                autoComplete="current-password"
                            />
                        </Field>

                        <Field id="styleguide-search" label="Input group">
                            <InputGroup>
                                <InputGroupAddon align="inline-start">
                                    @
                                </InputGroupAddon>
                                <InputGroupInput
                                    id="styleguide-search"
                                    type="search"
                                    placeholder="handle.example"
                                    spellCheck={false}
                                />
                            </InputGroup>
                        </Field>

                        <Field id="styleguide-disabled" label="Disabled">
                            <Input
                                id="styleguide-disabled"
                                defaultValue="Unavailable"
                                disabled
                            />
                        </Field>

                        <Field
                            id="styleguide-invalid"
                            label="Invalid"
                            hint="Use a specific message that helps someone recover."
                        >
                            <Input
                                id="styleguide-invalid"
                                type="email"
                                defaultValue="reader@"
                                aria-invalid
                                aria-describedby="styleguide-invalid-hint styleguide-invalid-error"
                            />
                            <p
                                id="styleguide-invalid-error"
                                className="mt-1.5 text-xs font-light text-destructive"
                            >
                                Enter a complete email address.
                            </p>
                        </Field>

                        <Field id="styleguide-textarea" label="Textarea">
                            <Textarea
                                id="styleguide-textarea"
                                placeholder="Add a short note…"
                                defaultValue="A finite edition for a calmer morning."
                            />
                        </Field>

                        <Field id="styleguide-file" label="File">
                            <Input id="styleguide-file" type="file" />
                        </Field>
                    </div>
                </StyleguideSection>

                <StyleguideSection
                    title="Choice controls"
                    description="Native semantics stay intact while fills, focus, and selection use the same surface tokens as the rest of the product."
                >
                    <div className="grid gap-x-8 gap-y-10 md:grid-cols-2">
                        <Field id="styleguide-select" label="Select">
                            <NativeSelect
                                id="styleguide-select"
                                defaultValue="morning"
                            >
                                <option value="morning">Morning edition</option>
                                <option value="lunch">Lunch edition</option>
                                <option value="evening">Evening edition</option>
                            </NativeSelect>
                        </Field>

                        <Field id="styleguide-date" label="Date">
                            <Input
                                id="styleguide-date"
                                type="date"
                                defaultValue="2026-05-14"
                            />
                        </Field>

                        <fieldset className="space-y-3">
                            <legend className="text-sm leading-none font-medium">
                                Checkboxes
                            </legend>
                            <div className="flex flex-col items-start gap-3">
                                <Checkbox
                                    id="styleguide-checkbox-off"
                                    label="Unchecked"
                                />
                                <Checkbox
                                    id="styleguide-checkbox-on"
                                    label="Checked"
                                    defaultChecked
                                />
                                <Checkbox
                                    id="styleguide-checkbox-disabled"
                                    label="Disabled"
                                    disabled
                                />
                            </div>
                        </fieldset>

                        <fieldset className="space-y-3">
                            <legend className="text-sm leading-none font-medium">
                                Radio group
                            </legend>
                            <div className="flex flex-col items-start gap-3">
                                <Radio
                                    id="styleguide-radio-morning"
                                    name="styleguide-edition"
                                    label="Morning"
                                    defaultChecked
                                />
                                <Radio
                                    id="styleguide-radio-lunch"
                                    name="styleguide-edition"
                                    label="Lunch"
                                />
                                <Radio
                                    id="styleguide-radio-evening"
                                    name="styleguide-edition"
                                    label="Evening"
                                />
                            </div>
                        </fieldset>

                        <fieldset className="space-y-3">
                            <legend className="text-sm leading-none font-medium">
                                Switches
                            </legend>
                            <div className="flex max-w-sm flex-col gap-4">
                                <SwitchRow
                                    id="styleguide-switch-off"
                                    label="Muted source"
                                />
                                <SwitchRow
                                    id="styleguide-switch-on"
                                    label="Primary source"
                                    defaultChecked
                                />
                                <SwitchRow
                                    id="styleguide-switch-disabled"
                                    label="Disabled"
                                    disabled
                                />
                            </div>
                        </fieldset>

                        <Field
                            id="styleguide-range"
                            label="Range"
                            hint="Native range controls use atmosphere blue as their accent."
                        >
                            <input
                                id="styleguide-range"
                                type="range"
                                min="0"
                                max="100"
                                defaultValue="68"
                                aria-describedby="styleguide-range-hint"
                                className="h-10 w-full cursor-pointer accent-primary outline-none focus-visible:outline-solid focus-visible:outline-1 focus-visible:outline-offset-2 focus-visible:outline-ring"
                            />
                        </Field>
                    </div>
                </StyleguideSection>

                <StyleguideSection
                    title="Button variants"
                    description="Primary and secondary carry the action hierarchy. The remaining exported variants handle quieter or context-specific controls."
                >
                    <div className="space-y-8">
                        <div className="flex flex-wrap items-center gap-3">
                            {BUTTON_VARIANTS.map(({ label, variant }) => (
                                <Button key={variant} variant={variant}>
                                    {label}
                                </Button>
                            ))}
                            <div className="rounded-xl bg-sunrise p-2">
                                <Button variant="ghost-on-gradient">
                                    On gradient
                                </Button>
                            </div>
                        </div>

                        <ButtonSubsection title="States">
                            <Button>Default</Button>
                            <Button aria-pressed="true">Pressed</Button>
                            <Button disabled>Disabled</Button>
                            <Button disabled>Saving…</Button>
                        </ButtonSubsection>

                        <ButtonSubsection title="Sizes">
                            <Button size="xs">Extra small</Button>
                            <Button size="sm">Small</Button>
                            <Button>Default</Button>
                            <Button size="lg">Large</Button>
                        </ButtonSubsection>
                    </div>
                </StyleguideSection>
            </div>
        </main>
    );
}

type SwitchRowProps = ComponentProps<typeof Switch> & {
    id: string;
    label: string;
};

function SwitchRow({ id, label, disabled, ...props }: SwitchRowProps) {
    return (
        <div className="flex items-center justify-between gap-4">
            <Label
                htmlFor={id}
                className={cn(
                    'cursor-pointer',
                    disabled && 'cursor-not-allowed text-muted-foreground',
                )}
            >
                {label}
            </Label>
            <Switch id={id} disabled={disabled} {...props} />
        </div>
    );
}

function ButtonSubsection({
    title,
    children,
}: {
    title: string;
    children: ReactNode;
}) {
    return (
        <div className="space-y-3">
            <h3>{title}</h3>
            <div className="flex flex-wrap items-center gap-3">{children}</div>
        </div>
    );
}
