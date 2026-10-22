import { useContext } from 'react';

import { AppProfileContext } from '@/lib/app-profile-context';

export function useAppProfile() {
    const state = useContext(AppProfileContext);
    if (!state) {
        throw new Error('useAppProfile must be used within AppShell');
    }
    return state;
}
