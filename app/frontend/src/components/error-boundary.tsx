import { Component, type ReactNode } from 'react';

import { ErrorState } from '@/components/status-state';
import { Button } from '@/components/ui/button';
import { Frame } from '@/design/frame';

export class ErrorBoundary extends Component<{ children: ReactNode }, { failed: boolean }> {
    state = { failed: false };

    static getDerivedStateFromError() {
        return { failed: true };
    }

    render() {
        if (!this.state.failed) return this.props.children;
        return (
            <Frame align="center" as="main" id="main" justify="center" minHeight="screen">
                <ErrorState
                    title="This page couldn’t be shown"
                    description="Something on this page broke while loading. Reloading usually fixes it."
                    action={<Button variant="secondary" onClick={() => window.location.reload()}>Reload page</Button>}
                />
            </Frame>
        );
    }
}
