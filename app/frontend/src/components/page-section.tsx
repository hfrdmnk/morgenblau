import type { ReactNode } from 'react';

import { Frame } from '@/design/frame';
import { Text } from '@/design/text';

// A titled part of a page, such as Feeds on Sources or People on Admin; the title names the region for assistive tech too.
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
