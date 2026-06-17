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
            <div className="flex min-h-dvh flex-col">
                <AppChrome onAddSourceClick={() => setAddSourceOpen(true)} />
                <main className="flex flex-1 flex-col">
                    <div className="flex-1">{children}</div>
                    <AppFooter />
                </main>
                <AddSourceDialog
                    open={addSourceOpen}
                    onOpenChange={setAddSourceOpen}
                />
            </div>
        </ChromeActionsProvider>
    );
}
