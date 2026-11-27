import type { ReactNode } from 'react';

import { Button } from '@/components/ui/button';
import { Link } from '@/components/ui/link';
import { PostRow, PostRowActions } from '@/components/ui/post-list';
import { GutterMark } from '@/components/ui/source-mark';
import { SourceIcon } from '@/components/ui/source-icon';
import { Frame } from '@/design/frame';
import { Text } from '@/design/text';

export function ItemList({
    children,
    label,
    ordered = false,
}: {
    children: ReactNode;
    label?: string;
    ordered?: boolean;
}) {
    if (ordered)
        return (
            <Frame aria-label={label} as="ol" gap="3xl">
                {children}
            </Frame>
        );
    return (
        <Frame aria-label={label} as="ul" gap="3xl">
            {children}
        </Frame>
    );
}

// mark hangs in the gutter left of the title on wide screens, so marked and unmarked titles share one edge.
export function ListItem({
    title,
    href,
    mark,
    icon,
    meta,
    status,
    note,
    actions,
    rowActions,
}: {
    title: string;
    href: string;
    mark?: ReactNode;
    icon?: string | null;
    meta?: ReactNode;
    status?: ReactNode;
    note?: ReactNode;
    actions?: ReactNode;
    rowActions?: ReactNode;
}) {
    return (
        <Frame align="center" as="li" direction="row" gap="m" minWidth={0}>
            <Frame grow minWidth={0}>
                <PostRow>
                    <Frame gap="s">
                        <Frame
                            align="center"
                            direction="row"
                            display={{ base: 'flex', sm: 'block' }}
                            gap="s"
                            minWidth={0}
                            position={{ sm: 'relative' }}
                        >
                            {mark ? <GutterMark offset="row">{mark}</GutterMark> : null}
                            <Text as="span" display="block" size="lg" truncate weight="medium">
                                <Link data-post-link href={href} title={title} variant="title">
                                    {title}
                                </Link>
                            </Text>
                        </Frame>
                        {icon || meta ? (
                            // The status inherits the line's size and tone, as the meta does.
                            <Text as="span" size="sm" tone="subtle">
                                <Frame align="center" as="span" direction="row" gap="s" minWidth={0}>
                                    <SourceIcon src={icon} />
                                    <Text as="span" truncate>
                                        {meta}
                                    </Text>
                                    {status}
                                </Frame>
                            </Text>
                        ) : null}
                    </Frame>
                    {rowActions ? <PostRowActions>{rowActions}</PostRowActions> : null}
                </PostRow>
                {note}
            </Frame>
            {actions}
        </Frame>
    );
}

export function ListError({ label, retry }: { label: string; retry: () => void }) {
    return (
        <Frame align="center" direction="row" gap="l" role="alert" wrap>
            <Text size="sm" tone="muted">
                Couldn’t load {label}.
            </Text>
            <Button onClick={retry} size="sm" variant="secondary">
                Try again
            </Button>
        </Frame>
    );
}
