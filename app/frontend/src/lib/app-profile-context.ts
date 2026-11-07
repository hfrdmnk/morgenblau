import { createContext } from 'react';

import { api, ApiError } from '@/lib/api';
import { PATHS } from '@/lib/paths';

export type AppProfile = {
    did: string;
    handle: string;
    displayName: string | null;
    avatar: string | null;
    isAdmin?: boolean;
};

export type AppProfileState =
    | { kind: 'loading' }
    | { kind: 'ready'; profile: AppProfile }
    | { kind: 'error' };

export const AppProfileContext = createContext<AppProfileState | null>(null);

function isAbortError(error: unknown): boolean {
    return error instanceof DOMException && error.name === 'AbortError';
}

function isUnauthorized(error: unknown): boolean {
    return error instanceof ApiError && error.status === 401;
}

export async function loadProfile(signal: AbortSignal): Promise<AppProfileState | null> {
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
