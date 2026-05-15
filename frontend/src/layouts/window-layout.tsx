import type { ReactNode } from 'react';

import { Window } from '@/components/window';
import { WindowChrome } from '@/components/window-chrome';
import { WindowFooter } from '@/components/window-footer';

export default function WindowLayout({ children }: { children: ReactNode }) {
    return (
        <div className="flex h-dvh flex-col">
            <WindowChrome />
            <main className="min-h-0 flex-1 px-4 pb-4">
                <Window variant="plain" className="h-full">
                    <div className="h-full overflow-y-auto">
                        <div className="flex min-h-full flex-col">
                            <div className="flex-1">{children}</div>
                            <WindowFooter />
                        </div>
                    </div>
                </Window>
            </main>
        </div>
    );
}
