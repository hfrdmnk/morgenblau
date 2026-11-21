import type { ReactNode } from 'react';

import { Frame } from '@/design/frame';

export function DataSettings({ children }: { children: ReactNode }) {
    return (
        <Frame as="main" id="main" maxWidth="3xl" paddingX={{ base: 'xl', sm: '3xl' }} width="full">
            {children}
        </Frame>
    );
}
