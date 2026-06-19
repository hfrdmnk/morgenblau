import { AppLogoIcon } from '@/components/app-logo-icon';

export function AppFooter() {
    return (
        <footer className="flex flex-col items-center gap-1.5 py-6">
            <AppLogoIcon className="size-5 text-muted-foreground" />
            <p className="text-sm font-light text-muted-foreground">
                Your calm corner of the internet.
            </p>
        </footer>
    );
}
