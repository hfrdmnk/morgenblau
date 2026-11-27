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

function withBase<T extends string>(value: Responsive<T> | undefined, fallback: T): Responsive<T> {
    if (typeof value === 'object' && value !== null) return { ...value, base: value.base ?? fallback };
    return value ?? fallback;
}

export function Frame<E extends FrameElement = 'div'>({ as, display, direction, ...props }: FrameProps<E>) {
    const { classes, rest } = styleClasses(frame, {
        display: withBase(display, 'flex'),
        direction: withBase(direction, 'column'),
        ...props,
    });
    return createElement(as ?? 'div', { ...rest, className: classes.join(' ') });
}
