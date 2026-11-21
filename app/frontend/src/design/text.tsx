import { createElement, type ComponentPropsWithRef } from 'react';

import { styleClasses, type StyleProps } from '@/design/style-props';
import { text } from '@/design/tokens';

type TextElement =
    | 'p'
    | 'span'
    | 'h1'
    | 'h2'
    | 'h3'
    | 'h4'
    | 'label'
    | 'code'
    | 'strong'
    | 'em'
    | 'time'
    | 'small'
    | 'dt'
    | 'dd';

type TypeProps = StyleProps<typeof text>;

export type TextProps<E extends TextElement = 'p'> = { as?: E } & TypeProps &
    Omit<ComponentPropsWithRef<E>, 'className' | 'style' | keyof TypeProps>;

// Every run of text: size, weight and tone are separate decisions, and each one left out inherits from the parent.
export function Text<E extends TextElement = 'p'>({ as, ...props }: TextProps<E>) {
    const { classes, rest } = styleClasses(text, props);
    return createElement(as ?? 'p', { ...rest, className: classes.join(' ') || undefined });
}
