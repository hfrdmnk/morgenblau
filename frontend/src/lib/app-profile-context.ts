import { createContext } from 'react';

export type AppProfile = {
    did: string;
    handle: string;
    displayName: string | null;
    avatar: string | null;
};

export type AppProfileState =
    | { kind: 'loading' }
    | { kind: 'ready'; profile: AppProfile }
    | { kind: 'error' };

export const AppProfileContext = createContext<AppProfileState | null>(null);
