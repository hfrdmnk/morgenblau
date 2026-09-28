import type { ReactNode } from 'react';
import { useEffect, useState } from 'react';

import { ErrorState } from '@/components/status-state';
import { Button } from '@/components/ui/button';
import { api, ApiError } from '@/lib/api';
import {
    AppProfileContext,
    type AppProfile,
    type AppProfileState,
} from '@/lib/app-profile-context';
import { PATHS } from '@/lib/paths';

function isAbortError(error: unknown): boolean {
    return error instanceof DOMException && error.name === 'AbortError';
}

function isUnauthorized(error: unknown): boolean {
    return error instanceof ApiError && error.status === 401;
}

async function loadProfile(signal: AbortSignal): Promise<AppProfileState | null> {
    try {
        const profile = await api<AppProfile>('/api/profiles/me', { signal });
        return { kind: 'ready', profile };
    } catch (error) {
        if (isAbortError(error)) {
            return null;
        }
        if (isUnauthorized(error)) {
            window.location.replace(PATHS.login);
            return null;
        }
        return { kind: 'error' };
    }
}

export function AppShell({ children }: { children: ReactNode }) {
    const [attempt, setAttempt] = useState(0);
    const [state, setState] = useState<AppProfileState>({ kind: 'loading' });

    useEffect(() => {
        const controller = new AbortController();

        void loadProfile(controller.signal).then((nextState) => {
            if (nextState) {
                setState(nextState);
            }
        });

        return () => controller.abort();
    }, [attempt]);

    if (state.kind === 'loading') {
        return (
            <AppProfileContext.Provider value={state}>
                {children}
            </AppProfileContext.Provider>
        );
    }

    if (state.kind === 'error') {
        return (
            <main className="grid min-h-dvh place-items-center">
                <ErrorState
                    title="Couldn’t load your account"
                    description="Try again in a moment."
                    action={
                        <Button
                            variant="secondary"
                            onClick={() => {
                                setState({ kind: 'loading' });
                                setAttempt((current) => current + 1);
                            }}
                        >
                            Try again
                        </Button>
                    }
                />
            </main>
        );
    }

    return (
        <AppProfileContext.Provider value={state}>
            {children}
        </AppProfileContext.Provider>
    );
}
