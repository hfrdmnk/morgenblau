import type { ReactNode } from 'react';
import { useState } from 'react';

import { AddSourceDialog } from '@/components/sources/add-dialog';
import { Window } from '@/components/window';
import { ChromeActionsProvider } from '@/layouts/window/chrome-actions';
import { WindowChrome } from '@/layouts/window/window-chrome';
import { WindowFooter } from '@/layouts/window/window-footer';

export function WindowLayout({ children }: { children: ReactNode }) {
    const [addSourceOpen, setAddSourceOpen] = useState(false);

    return (
        <ChromeActionsProvider>
            <div className="flex h-dvh flex-col">
                <WindowChrome onAddSourceClick={() => setAddSourceOpen(true)} />
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
                <AddSourceDialog
                    open={addSourceOpen}
                    onOpenChange={setAddSourceOpen}
                />
            </div>
        </ChromeActionsProvider>
    );
}
