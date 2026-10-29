import { Component, type ReactNode } from 'react';

import { ErrorState } from '@/components/status-state';
import { Button } from '@/components/ui/button';

export class ErrorBoundary extends Component<{ children: ReactNode }, { failed: boolean }> {
    state = { failed: false };

    static getDerivedStateFromError() {
        return { failed: true };
    }

    render() {
        if (!this.state.failed) return this.props.children;
        return (
            <main className="grid min-h-dvh place-items-center" id="main">
                <ErrorState
                    title="This page couldn’t be shown"
                    description="Something on this page broke while loading. Reloading usually fixes it."
                    action={<Button variant="secondary" onClick={() => window.location.reload()}>Reload page</Button>}
                />
            </main>
        );
    }
}
