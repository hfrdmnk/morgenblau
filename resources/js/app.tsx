import { createInertiaApp } from '@inertiajs/react';
import { Retune } from 'retune';
import { Toaster } from '@/components/ui/sonner';
import { TooltipProvider } from '@/components/ui/tooltip';
import { initializeTheme } from '@/hooks/use-appearance';
import AppLayout from '@/layouts/app-layout';
import AuthGoldenLayout from '@/layouts/auth/auth-golden-layout';
import AuthLayout from '@/layouts/auth-layout';
import SettingsLayout from '@/layouts/settings/layout';
import WindowLayout from '@/layouts/window-layout';

const WINDOW_PAGES = new Set(['discover', 'consume', 'create']);

const appName = import.meta.env.VITE_APP_NAME || 'Morgenblau';

createInertiaApp({
    title: (title) => (title ? `${title} | ${appName}` : appName),
    layout: (name) => {
        switch (true) {
            case name === 'welcome':
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
                {app}
                <Toaster />
                <Retune />
            </TooltipProvider>
        );
    },
    progress: {
        color: '#4B5563',
    },
});

initializeTheme();
