import type { ReactNode } from 'react';
import AppLogoIcon from '@/components/app-logo-icon';
import { Window } from '@/components/window';

export default function AuthLoginLayout({ children }: { children: ReactNode }) {
    return (
        <div className="flex min-h-svh gap-6 bg-background p-6">
            <section className="flex flex-[1.618] items-center">
                <div className="w-full max-w-md pl-6 lg:pl-12">{children}</div>
            </section>
            <Window
                variant="sunrise"
                className="flex flex-1 items-center justify-center text-white"
            >
                <AppLogoIcon
                    className="size-16"
                    style={{ viewTransitionName: 'brand-logo' }}
                />
            </Window>
        </div>
    );
}
