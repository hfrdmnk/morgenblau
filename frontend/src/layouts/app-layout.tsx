import type { ReactNode } from 'react';
import { useState } from 'react';

import { AddSourceDialog } from '@/components/sources/add-dialog';
import { AppChrome } from '@/layouts/app/app-chrome';
import { AppFooter } from '@/layouts/app/app-footer';
import { ChromeActionsProvider } from '@/layouts/app/chrome-actions';

export function AppLayout({ children }: { children: ReactNode }) {
    const [addSourceOpen, setAddSourceOpen] = useState(false);

    return (
        <ChromeActionsProvider>
            <div className="flex h-dvh flex-col">
                <AppChrome onAddSourceClick={() => setAddSourceOpen(true)} />
                <main className="min-h-0 flex-1 overflow-y-auto">
                    <div className="flex min-h-full flex-col">
                        <div className="flex-1">{children}</div>
                        <AppFooter />
                    </div>
                </main>
                <AddSourceDialog
                    open={addSourceOpen}
                    onOpenChange={setAddSourceOpen}
                />
            </div>
        </ChromeActionsProvider>
    );
}
