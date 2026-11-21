import { createElement, type ComponentPropsWithRef } from 'react';

import { styleClasses, type Responsive, type StyleProps } from '@/design/style-props';
import { frame } from '@/design/tokens';

type FrameElement =
    | 'div'
    | 'span'
    | 'section'
    | 'nav'
    | 'header'
    | 'footer'
    | 'main'
    | 'aside'
    | 'article'
    | 'ul'
    | 'ol'
    | 'li'
    | 'form'
    | 'label'
    | 'fieldset';

type LayoutProps = StyleProps<typeof frame>;

export type FrameProps<E extends FrameElement = 'div'> = { as?: E } & LayoutProps &
    Omit<ComponentPropsWithRef<E>, 'className' | 'style' | keyof LayoutProps>;

// A per-breakpoint value without a base keeps the default below its first breakpoint, so display={{ md: 'none' }} is still a flex
// column on phones.
function withBase<T extends string>(value: Responsive<T> | undefined, fallback: T): Responsive<T> {
    if (typeof value === 'object' && value !== null) return { ...value, base: value.base ?? fallback };
    return value ?? fallback;
}

// The only container: every div, section, list or form is a Frame, and its look comes from tokens, never class strings.
// A flex column by default, so a Frame with a gap is a stack.
export function Frame<E extends FrameElement = 'div'>({ as, display, direction, ...props }: FrameProps<E>) {
    const { classes, rest } = styleClasses(frame, {
        display: withBase(display, 'flex'),
        direction: withBase(direction, 'column'),
        ...props,
    });
    return createElement(as ?? 'div', { ...rest, className: classes.join(' ') });
}
