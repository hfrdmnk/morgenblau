import type { ReactNode } from 'react';

import { Frame } from '@/design/frame';
import { Text } from '@/design/text';

export function PageSection({ title, children }: { title: string; children: ReactNode }) {
    return (
        <Frame as="section" aria-label={title} gap="2xl">
            <Text as="h2" size="base" tone="muted" weight="medium">
                {title}
            </Text>
            {children}
        </Frame>
    );
}
