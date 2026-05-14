import { createInertiaApp } from '@inertiajs/react';
import { Toaster } from '@/components/ui/sonner';
import { TooltipProvider } from '@/components/ui/tooltip';
import { initializeTheme } from '@/hooks/use-appearance';
import { useFlashToast } from '@/hooks/use-flash-toast';
import AppLayout from '@/layouts/app-layout';
import AuthGoldenLayout from '@/layouts/auth/auth-golden-layout';
import AuthLayout from '@/layouts/auth-layout';
import SettingsLayout from '@/layouts/settings/layout';
import WindowLayout from '@/layouts/window-layout';

const WINDOW_PAGES = new Set(['discover', 'consume', 'create']);

const appName = import.meta.env.VITE_APP_NAME || 'Morgenblau';

function FlashListener() {
    useFlashToast();

    return null;
}

createInertiaApp({
    title: (title) => (title ? `${title} | ${appName}` : appName),
    layout: (name) => {
        switch (true) {
            case name === 'welcome':
                return null;
            case name === 'entry':
            case name === 'watch':
                return null;
            case name === 'auth/login':
                return AuthGoldenLayout;
            case name.startsWith('auth/'):
                return AuthLayout;
            case name.startsWith('settings/'):
                return [AppLayout, SettingsLayout];
            case WINDOW_PAGES.has(name):
                return WindowLayout;
            default:
                return AppLayout;
        }
    },
    strictMode: true,
    withApp(app) {
        return (
            <TooltipProvider delay={0}>
                <FlashListener />
                {app}
                <Toaster />
            </TooltipProvider>
        );
    },
    progress: {
        color: '#4B5563',
    },
});

initializeTheme();
