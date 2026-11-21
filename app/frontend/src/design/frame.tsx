import { createElement, type ComponentPropsWithRef } from 'react';

import { styleClasses, type StyleProps } from '@/design/style-props';
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

// The only container: every div, section, list or form is a Frame, and its look comes from tokens, never class strings.
// A flex column by default, so a Frame with a gap is a stack.
export function Frame<E extends FrameElement = 'div'>({ as, ...props }: FrameProps<E>) {
    const { classes, rest } = styleClasses(frame, props);
    if (props.display === undefined) classes.unshift('flex');
    if (props.direction === undefined) classes.unshift('flex-col');
    return createElement(as ?? 'div', { ...rest, className: classes.join(' ') });
}
