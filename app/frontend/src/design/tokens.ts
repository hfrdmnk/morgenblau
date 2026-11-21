// The design decisions Frame and Text accept, each mapped to the literal Tailwind class that carries it.
// Values live in src/index.css; breakpoint variants of every class here come from its @source inline lines,
// which tokens.test.ts keeps in step with these maps.

export const spacing = {
    none: '0',
    '3xs': '0.5',
    '2xs': '1',
    xs: '1.5',
    s: '2',
    m: '3',
    l: '4',
    xl: '5',
    '2xl': '6',
    '3xl': '8',
    '4xl': '10',
    '5xl': '12',
    '6xl': '16',
    '7xl': '20',
    '8xl': '24',
} as const;

export type Spacing = keyof typeof spacing;

const scale = <P extends string>(prefix: P) =>
    Object.fromEntries(Object.entries(spacing).map(([name, step]) => [name, `${prefix}-${step}`])) as {
        [K in Spacing]: `${P}-${(typeof spacing)[K]}`;
    };

export const frame = {
    display: {
        flex: 'flex',
        grid: 'grid',
        block: 'block',
        'inline-flex': 'inline-flex',
        contents: 'contents',
        none: 'hidden',
    },
    direction: { column: 'flex-col', row: 'flex-row' },
    align: {
        start: 'items-start',
        center: 'items-center',
        end: 'items-end',
        baseline: 'items-baseline',
        stretch: 'items-stretch',
    },
    justify: {
        start: 'justify-start',
        center: 'justify-center',
        end: 'justify-end',
        between: 'justify-between',
        around: 'justify-around',
    },
    alignSelf: { start: 'self-start', center: 'self-center', end: 'self-end', stretch: 'self-stretch' },
    justifySelf: { start: 'justify-self-start', center: 'justify-self-center', end: 'justify-self-end' },
    wrap: { true: 'flex-wrap' },
    grow: { true: 'flex-1' },
    shrink: { false: 'shrink-0' },
    gap: scale('gap'),
    gapX: scale('gap-x'),
    gapY: scale('gap-y'),
    padding: scale('p'),
    paddingX: scale('px'),
    paddingY: scale('py'),
    paddingTop: scale('pt'),
    paddingBottom: scale('pb'),
    paddingStart: scale('ps'),
    paddingEnd: scale('pe'),
    width: { full: 'w-full', fit: 'w-fit' },
    // status: the room a sheet's pending state holds, so the result that replaces it does not jump.
    height: { full: 'h-full', status: 'h-36' },
    minWidth: { 0: 'min-w-0' },
    // control: a default Button's height, so a row of text lines up with one; touch: the 44px touch target.
    // state: the room a page's loading, empty or error state centres in, so it reads as the page and not a stray line.
    minHeight: {
        0: 'min-h-0',
        screen: 'min-h-dvh',
        full: 'min-h-full',
        control: 'min-h-10',
        touch: 'min-h-11',
        state: 'min-h-52',
    },
    maxWidth: {
        sm: 'max-w-sm',
        md: 'max-w-md',
        lg: 'max-w-lg',
        xl: 'max-w-xl',
        '2xl': 'max-w-2xl',
        '3xl': 'max-w-3xl',
        '7xl': 'max-w-7xl',
        prose: 'max-w-prose',
        popover: 'max-w-72',
        full: 'max-w-full',
    },
    columns: {
        1: 'grid-cols-1',
        2: 'grid-cols-2',
        3: 'grid-cols-3',
        4: 'grid-cols-4',
        7: 'grid-cols-7',
        sidebar: 'grid-cols-(--grid-sidebar)',
        specimen: 'grid-cols-(--grid-specimen)',
    },
    colSpan: { 1: 'col-span-1', 2: 'col-span-2', full: 'col-span-full' },
    colStart: { 1: 'col-start-1', 2: 'col-start-2', 3: 'col-start-3' },
    rowStart: { 1: 'row-start-1', 2: 'row-start-2', 3: 'row-start-3' },
    rowSpan: { 2: 'row-span-2' },
    order: { first: 'order-first', none: 'order-none' },
    overflow: { hidden: 'overflow-hidden', auto: 'overflow-auto' },
    position: { relative: 'relative' },
    // A focus target that is the whole screen or panel needs no ring: what changed already shows where focus went.
    outline: { none: 'outline-none' },
    background: {
        background: 'bg-background',
        muted: 'bg-muted',
        'muted-soft': 'bg-muted-soft',
        secondary: 'bg-secondary',
        popover: 'bg-popover',
        accent: 'bg-accent',
        primary: 'bg-primary',
        destructive: 'bg-destructive',
        warning: 'bg-warning',
        neutral: 'bg-neutral',
    },
    radius: { sm: 'rounded-sm', md: 'rounded-md', lg: 'rounded-lg', xl: 'rounded-xl', full: 'rounded-full' },
    border: { all: 'border', top: 'border-t', bottom: 'border-b', start: 'border-s' },
    shadow: { popover: 'shadow-popover', hairline: 'shadow-hairline' },
} as const;

export const text = {
    size: {
        xs: 'text-xs',
        sm: 'text-sm',
        base: 'text-base',
        lg: 'text-lg',
        xl: 'text-xl',
        '2xl': 'text-2xl',
        '3xl': 'text-3xl',
    },
    weight: { normal: 'font-normal', book: 'font-book', medium: 'font-medium' },
    tone: {
        foreground: 'text-foreground',
        muted: 'text-muted-foreground',
        subtle: 'text-subtle-foreground',
        destructive: 'text-destructive',
        success: 'text-success',
    },
    family: { sans: 'font-sans', serif: 'font-serif', duo: 'font-duo' },
    align: { start: 'text-start', center: 'text-center', end: 'text-end' },
    wrap: { pretty: 'text-pretty', balance: 'text-balance', nowrap: 'whitespace-nowrap' },
    break: { words: 'break-words', all: 'break-all' },
    truncate: { true: 'truncate' },
    lineClamp: { 2: 'line-clamp-2', 5: 'line-clamp-5' },
    numeric: { tabular: 'tabular-nums' },
    caps: { true: 'uppercase tracking-wide' },
    tracking: { tight: 'tracking-tight' },
    // A heading that script focuses to mark where a new result begins; the result itself shows where focus went.
    outline: { none: 'outline-none' },
    display: { block: 'block', inline: 'inline', 'inline-flex': 'inline-flex', none: 'hidden' },
    visuallyHidden: { true: 'sr-only' },
} as const;

export const breakpoints = ['sm', 'md', 'lg'] as const;
