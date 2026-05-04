import type { Auth } from '@/types/auth';

declare module '@inertiajs/core' {
    export interface InertiaConfig {
        sharedPageProps: {
            name: string;
            auth: Auth;
            sidebarOpen: boolean;
            // Laravel session flash, surfaced for the login banner. Toast
            // notifications come through the Inertia v3 flash channel
            // (router.on('flash')) and are typed via flashDataType below.
            flash: { message?: string } | null;
            [key: string]: unknown;
        };
        flashDataType: App.Data.Shared.FlashData;
    }
}
