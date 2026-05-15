import { AppLogoIcon } from '@/components/app-logo-icon';

export function WindowFooter() {
    return (
        <footer className="flex flex-col items-center gap-1.5 py-6">
            <AppLogoIcon className="size-5 text-muted-foreground" />
            <p className="text-sm font-light text-muted-foreground">
                Your calm window into the Atmosphere.
            </p>
        </footer>
    );
}
