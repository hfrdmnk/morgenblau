import { breakpoints } from '@/design/tokens';

type Breakpoint = (typeof breakpoints)[number];

type Choice<K> = K extends 'true' ? true : K extends 'false' ? false : K;

export type Responsive<T> = T | ({ base?: T } & { [B in Breakpoint]?: T });

type ClassMaps = Record<string, Record<string | number, string>>;

export type StyleProps<M extends ClassMaps> = {
    [P in keyof M]?: Responsive<Choice<keyof M[P]>>;
};

function tokenClasses(map: Record<string, string>, value: unknown): string[] {
    const perBreakpoint: [string, unknown][] =
        typeof value === 'object' && value !== null ? Object.entries(value) : [['base', value]];
    return perBreakpoint
        .filter(([, token]) => token !== undefined)
        .flatMap(([breakpoint, token]) =>
            map[String(token)].split(' ').map((cls) => (breakpoint === 'base' ? cls : `${breakpoint}:${cls}`)),
        );
}

export function styleClasses<M extends ClassMaps>(maps: M, props: Record<string, unknown>) {
    const classes: string[] = [];
    const rest: Record<string, unknown> = {};
    for (const [name, value] of Object.entries(props)) {
        if (name in maps) classes.push(...tokenClasses(maps[name], value));
        else rest[name] = value;
    }
    return { classes, rest };
}
